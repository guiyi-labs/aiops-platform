package identityreadiness

import (
	"bytes"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"slices"
	"strings"
)

const (
	PolicyFormat = "aiops.identity-readiness-policy/v1"
	ReportFormat = "aiops.identity-readiness-report/v1"
	maxFileSize  = 1 << 20
)

type Policy struct {
	Format         string               `json:"format"`
	ProviderName   string               `json:"provider_name"`
	Issuer         string               `json:"issuer"`
	ClientID       string               `json:"client_id"`
	RedirectURIs   []string             `json:"redirect_uris"`
	ClaimMapping   ClaimMapping         `json:"claim_mapping"`
	Authorization  AuthorizationPolicy  `json:"authorization"`
	MFA            MFAPolicy            `json:"mfa"`
	AccountLinking AccountLinkingPolicy `json:"account_linking"`
	Sessions       SessionPolicy        `json:"sessions"`
	BreakGlass     BreakGlassPolicy     `json:"break_glass"`
	Approvals      ApprovalPolicy       `json:"approvals"`
}

type ClaimMapping struct {
	Subject     string `json:"subject"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Groups      string `json:"groups"`
}

type AuthorizationPolicy struct {
	RequiredScopes           []string `json:"required_scopes"`
	AllowedSigningAlgorithms []string `json:"allowed_signing_algorithms"`
	PKCERequired             bool     `json:"pkce_required"`
}

type MFAPolicy struct {
	Required         bool     `json:"required"`
	EnforcementOwner string   `json:"enforcement_owner"`
	EvidenceClaim    string   `json:"evidence_claim"`
	AcceptedValues   []string `json:"accepted_values"`
}

type AccountLinkingPolicy struct {
	Mode            string `json:"mode"`
	AutoLinkByEmail bool   `json:"auto_link_by_email"`
}

type SessionPolicy struct {
	MaxAgeMinutes                        int    `json:"max_age_minutes"`
	ReauthenticationMinutes              int    `json:"reauthentication_minutes"`
	RevokeLocalSessionsOnIdentityDisable bool   `json:"revoke_local_sessions_on_identity_disable"`
	LogoutMode                           string `json:"logout_mode"`
}

type BreakGlassPolicy struct {
	Enabled           bool   `json:"enabled"`
	OwnerGroup        string `json:"owner_group"`
	CredentialStorage string `json:"credential_storage"`
	TestIntervalDays  int    `json:"test_interval_days"`
	MaxAccounts       int    `json:"max_accounts"`
}

type ApprovalPolicy struct {
	IdentityOwner    string `json:"identity_owner"`
	SecurityOwner    string `json:"security_owner"`
	ApplicationOwner string `json:"application_owner"`
}

type Discovery struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	JWKSURI                          string   `json:"jwks_uri"`
	EndSessionEndpoint               string   `json:"end_session_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

type JWK struct {
	KTY    string   `json:"kty"`
	Use    string   `json:"use"`
	KeyOps []string `json:"key_ops"`
	Alg    string   `json:"alg"`
	KID    string   `json:"kid"`
	N      string   `json:"n"`
	E      string   `json:"e"`
	Crv    string   `json:"crv"`
	X      string   `json:"x"`
	Y      string   `json:"y"`
}

type Check struct {
	Code   string `json:"code"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type Report struct {
	Format       string  `json:"format"`
	Ready        bool    `json:"ready"`
	ProviderName string  `json:"provider_name"`
	Issuer       string  `json:"issuer"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Checks       []Check `json:"checks"`
}

func LoadPolicy(path string) (Policy, error) {
	var value Policy
	return value, decodeStrictFile(path, &value)
}

func LoadDiscovery(path string) (Discovery, error) {
	var value Discovery
	return value, decodeStrictFile(path, &value)
}

func LoadJWKS(path string) (JWKS, error) {
	var value JWKS
	return value, decodeStrictFile(path, &value)
}

func Evaluate(policy Policy, discovery Discovery, keys JWKS) Report {
	report := Report{Format: ReportFormat, ProviderName: policy.ProviderName, Issuer: policy.Issuer}
	add := func(code string, passed bool, success, failure string) {
		detail := failure
		if passed {
			detail = success
			report.Passed++
		} else {
			report.Failed++
		}
		report.Checks = append(report.Checks, Check{Code: code, Passed: passed, Detail: detail})
	}

	add("policy.format", policy.Format == PolicyFormat, "supported policy format", "format must be "+PolicyFormat)
	add("policy.owners", allNonEmpty(policy.ProviderName, policy.ClientID, policy.Approvals.IdentityOwner, policy.Approvals.SecurityOwner, policy.Approvals.ApplicationOwner), "provider, client and accountable owners are assigned", "provider_name, client_id and all three accountable owners are required")
	issuerOK := validHTTPSURL(policy.Issuer) && strings.TrimSuffix(policy.Issuer, "/") == policy.Issuer
	add("oidc.issuer", issuerOK && discovery.Issuer == policy.Issuer, "discovery issuer exactly matches the approved HTTPS issuer", "approved issuer must be canonical HTTPS and exactly match discovery")
	endpointsOK := validHTTPSURL(discovery.AuthorizationEndpoint) && validHTTPSURL(discovery.TokenEndpoint) && validHTTPSURL(discovery.JWKSURI)
	add("oidc.endpoints", endpointsOK, "authorization, token and JWKS endpoints use HTTPS", "authorization, token and JWKS endpoints must be absolute HTTPS URLs")
	redirectsOK := len(policy.RedirectURIs) > 0
	for _, redirect := range policy.RedirectURIs {
		redirectsOK = redirectsOK && validHTTPSURL(redirect) && !strings.Contains(redirect, "#")
	}
	add("oidc.redirects", redirectsOK, "redirect URIs are explicit HTTPS values", "at least one explicit fragment-free HTTPS redirect URI is required")
	flowOK := slices.Contains(discovery.ResponseTypesSupported, "code") && policy.Authorization.PKCERequired && slices.Contains(discovery.CodeChallengeMethodsSupported, "S256")
	add("oidc.authorization_flow", flowOK, "authorization code flow requires PKCE S256", "authorization code response type and required PKCE S256 support are mandatory")
	requiredScopes := []string{"openid", "profile"}
	scopesOK := containsAll(policy.Authorization.RequiredScopes, requiredScopes) && containsAll(discovery.ScopesSupported, policy.Authorization.RequiredScopes)
	add("oidc.scopes", scopesOK, "required scopes are approved and advertised", "policy must require openid/profile and discovery must advertise every required scope")
	algorithmsOK := len(policy.Authorization.AllowedSigningAlgorithms) > 0
	for _, algorithm := range policy.Authorization.AllowedSigningAlgorithms {
		algorithmsOK = algorithmsOK && slices.Contains([]string{"RS256", "PS256", "ES256", "EdDSA"}, algorithm) && slices.Contains(discovery.IDTokenSigningAlgValuesSupported, algorithm)
	}
	add("oidc.signing_algorithms", algorithmsOK, "approved asymmetric ID-token algorithms are advertised", "allowed algorithms must be a non-empty subset of RS256/PS256/ES256/EdDSA and discovery support")
	usableKeys := 0
	seenKIDs := map[string]bool{}
	uniqueKIDs := true
	for _, key := range keys.Keys {
		if key.KID == "" || seenKIDs[key.KID] {
			uniqueKIDs = false
		}
		seenKIDs[key.KID] = true
		if slices.Contains(policy.Authorization.AllowedSigningAlgorithms, key.Alg) && usableSigningKey(key) {
			usableKeys++
		}
	}
	add("oidc.jwks", uniqueKIDs && usableKeys > 0, fmt.Sprintf("%d usable signing key(s) have unique key IDs", usableKeys), "JWKS must contain a unique kid and at least one structurally valid approved signing key")
	claimsOK := policy.ClaimMapping.Subject == "sub" && allNonEmpty(policy.ClaimMapping.Username, policy.ClaimMapping.DisplayName, policy.ClaimMapping.Groups)
	add("identity.claim_mapping", claimsOK, "immutable sub and explicit username/display/group claims are mapped", "subject must map to sub and all display/group mappings must be explicit")
	mfaOK := policy.MFA.Required && policy.MFA.EnforcementOwner == "identity_provider" && slices.Contains([]string{"acr", "amr"}, policy.MFA.EvidenceClaim) && len(policy.MFA.AcceptedValues) > 0
	for _, value := range policy.MFA.AcceptedValues {
		mfaOK = mfaOK && strings.TrimSpace(value) != ""
	}
	add("identity.mfa", mfaOK, "MFA is identity-provider enforced with explicit token evidence", "MFA must be required by the identity provider with accepted acr or amr evidence values")
	linkingOK := policy.AccountLinking.Mode == "admin_prelinked_subject" && !policy.AccountLinking.AutoLinkByEmail
	add("identity.account_linking", linkingOK, "accounts are admin-prelinked by immutable subject", "automatic email linking is forbidden; admin_prelinked_subject is required")
	sessionsOK := policy.Sessions.MaxAgeMinutes >= 5 && policy.Sessions.MaxAgeMinutes <= 1440 && policy.Sessions.ReauthenticationMinutes >= 1 && policy.Sessions.ReauthenticationMinutes <= policy.Sessions.MaxAgeMinutes && policy.Sessions.RevokeLocalSessionsOnIdentityDisable && policy.Sessions.LogoutMode == "local_and_provider" && validHTTPSURL(discovery.EndSessionEndpoint)
	add("identity.sessions", sessionsOK, "bounded reauthentication, disable revocation and provider logout are defined", "session bounds, disable revocation, local_and_provider logout and an HTTPS end-session endpoint are required")
	breakGlassOK := policy.BreakGlass.Enabled && allNonEmpty(policy.BreakGlass.OwnerGroup) && policy.BreakGlass.CredentialStorage == "offline_secret_manager" && policy.BreakGlass.TestIntervalDays >= 1 && policy.BreakGlass.TestIntervalDays <= 90 && policy.BreakGlass.MaxAccounts >= 1 && policy.BreakGlass.MaxAccounts <= 2
	add("identity.break_glass", breakGlassOK, "one or two offline break-glass accounts have ownership and a bounded drill interval", "break-glass requires an owner, offline secret manager, 1..2 accounts and a 1..90 day drill interval")

	report.Ready = report.Failed == 0
	return report
}

func decodeStrictFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return err
	}
	if len(contents) == 0 || len(contents) > maxFileSize {
		return fmt.Errorf("file must contain 1..%d bytes", maxFileSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("file must contain one JSON value")
	}
	return nil
}

func validHTTPSURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func allNonEmpty(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func containsAll(values, required []string) bool {
	for _, value := range required {
		if !slices.Contains(values, value) {
			return false
		}
	}
	return true
}

func usableSigningKey(key JWK) bool {
	if key.KID == "" || (key.Use != "" && key.Use != "sig") || (len(key.KeyOps) > 0 && !slices.Contains(key.KeyOps, "verify")) {
		return false
	}
	switch key.Alg {
	case "ES256":
		if key.KTY != "EC" || key.Crv != "P-256" {
			return false
		}
		xBytes, xErr := base64.RawURLEncoding.DecodeString(key.X)
		yBytes, yErr := base64.RawURLEncoding.DecodeString(key.Y)
		return xErr == nil && yErr == nil && len(xBytes) == 32 && len(yBytes) == 32 && elliptic.P256().IsOnCurve(new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes))
	case "RS256", "PS256":
		n, nErr := base64.RawURLEncoding.DecodeString(key.N)
		e, eErr := base64.RawURLEncoding.DecodeString(key.E)
		if key.KTY != "RSA" || nErr != nil || eErr != nil {
			return false
		}
		exponent := new(big.Int).SetBytes(e)
		return new(big.Int).SetBytes(n).BitLen() >= 2048 && exponent.IsInt64() && exponent.Int64() >= 3 && exponent.Bit(0) == 1
	case "EdDSA":
		x, err := base64.RawURLEncoding.DecodeString(key.X)
		return key.KTY == "OKP" && key.Crv == "Ed25519" && err == nil && len(x) == 32 && new(big.Int).SetBytes(x).Sign() != 0
	default:
		return false
	}
}
