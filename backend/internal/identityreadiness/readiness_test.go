package identityreadiness

import (
	"path/filepath"
	"testing"
)

func TestEvaluateAcceptsCompleteProviderContract(t *testing.T) {
	policy, discovery, keys := loadFixtures(t)
	report := Evaluate(policy, discovery, keys)
	if !report.Ready || report.Failed != 0 || report.Passed != 14 {
		t.Fatalf("Evaluate() = %#v, want ready report with 14 passed checks", report)
	}
}

func TestEvaluateFailsClosedForSecurityDowngrades(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Policy, *Discovery, *JWKS)
		code   string
	}{
		{"issuer mismatch", func(_ *Policy, d *Discovery, _ *JWKS) { d.Issuer = "https://other.example.test" }, "oidc.issuer"},
		{"missing S256", func(_ *Policy, d *Discovery, _ *JWKS) { d.CodeChallengeMethodsSupported = []string{"plain"} }, "oidc.authorization_flow"},
		{"email linking", func(p *Policy, _ *Discovery, _ *JWKS) { p.AccountLinking.AutoLinkByEmail = true }, "identity.account_linking"},
		{"MFA not required", func(p *Policy, _ *Discovery, _ *JWKS) { p.MFA.Required = false }, "identity.mfa"},
		{"invalid signing point", func(_ *Policy, _ *Discovery, k *JWKS) { k.Keys[0].X = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" }, "oidc.jwks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, discovery, keys := loadFixtures(t)
			tt.mutate(&policy, &discovery, &keys)
			report := Evaluate(policy, discovery, keys)
			if report.Ready || !failedCheck(report, tt.code) {
				t.Fatalf("Evaluate() = %#v, want failed %s check", report, tt.code)
			}
		})
	}
}

func TestLoadPolicyRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	contents := []byte(`{"format":"aiops.identity-readiness-policy/v1","client_secret":"must-not-be-here"}`)
	if err := writeTestFile(path, contents); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(path); err == nil {
		t.Fatal("LoadPolicy() error = nil, want unknown secret field rejection")
	}
}

func TestLoadPolicyRejectsTrailingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := writeTestFile(path, []byte(`{} trailing`)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(path); err == nil {
		t.Fatal("LoadPolicy() error = nil, want trailing-data rejection")
	}
}

func loadFixtures(t *testing.T) (Policy, Discovery, JWKS) {
	t.Helper()
	root := filepath.Join("testdata")
	policy, err := LoadPolicy(filepath.Join(root, "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := LoadDiscovery(filepath.Join(root, "discovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := LoadJWKS(filepath.Join(root, "jwks.json"))
	if err != nil {
		t.Fatal(err)
	}
	return policy, discovery, keys
}

func failedCheck(report Report, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code {
			return !check.Passed
		}
	}
	return false
}
