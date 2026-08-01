package oidc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
)

// JWK is a JSON Web Key entry from a provider JWKS response. Only the
// parameters needed for ID-token signature verification are modelled.
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

// JWKS is the JWKS response body.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// ErrUnsupportedKey indicates a JWK cannot be used for ID-token verification
// because its type, algorithm or parameters are unsupported or malformed.
var ErrUnsupportedKey = fmt.Errorf("oidc: unsupported or malformed signing key")

// publicKey converts a JWK into a crypto.PublicKey suitable for the configured
// algorithm. It mirrors the structural validation in
// identityreadiness.usableSigningKey but returns a usable key rather than a
// boolean, so M36C can verify ID-token signatures.
func (k JWK) publicKey(allowedAlgorithms map[string]struct{}) (cryptoPublicKey, string, error) {
	if k.KID == "" {
		return nil, "", ErrUnsupportedKey
	}
	if k.Use != "" && k.Use != "sig" {
		return nil, "", ErrUnsupportedKey
	}
	if len(k.KeyOps) > 0 && !containsString(k.KeyOps, "verify") {
		return nil, "", ErrUnsupportedKey
	}
	if _, ok := allowedAlgorithms[k.Alg]; !ok {
		return nil, "", ErrUnsupportedKey
	}
	switch k.Alg {
	case "RS256", "PS256":
		return k.rsaPublicKey()
	case "ES256":
		return k.ecdsaPublicKey()
	case "EdDSA":
		return k.ed25519PublicKey()
	default:
		return nil, "", ErrUnsupportedKey
	}
}

// cryptoPublicKey is an alias so the keyset file does not import crypto directly
// in type signatures while still returning the standard library key types.
type cryptoPublicKey = any

func (k JWK) rsaPublicKey() (cryptoPublicKey, string, error) {
	if k.KTY != "RSA" {
		return nil, "", ErrUnsupportedKey
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, "", ErrUnsupportedKey
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, "", ErrUnsupportedKey
	}
	modulus := new(big.Int).SetBytes(nBytes)
	if modulus.BitLen() < 2048 {
		return nil, "", ErrUnsupportedKey
	}
	exponent := new(big.Int).SetBytes(eBytes)
	if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Bit(0) != 1 {
		return nil, "", ErrUnsupportedKey
	}
	return &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}, k.Alg, nil
}

func (k JWK) ecdsaPublicKey() (cryptoPublicKey, string, error) {
	if k.KTY != "EC" || k.Crv != "P-256" {
		return nil, "", ErrUnsupportedKey
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil || len(xBytes) != 32 {
		return nil, "", ErrUnsupportedKey
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil || len(yBytes) != 32 {
		return nil, "", ErrUnsupportedKey
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, "", ErrUnsupportedKey
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, k.Alg, nil
}

func (k JWK) ed25519PublicKey() (cryptoPublicKey, string, error) {
	if k.KTY != "OKP" || k.Crv != "Ed25519" {
		return nil, "", ErrUnsupportedKey
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil || len(xBytes) != 32 {
		return nil, "", ErrUnsupportedKey
	}
	key := ed25519.PublicKey(xBytes)
	if key.Equal(ed25519.PublicKey{}) {
		return nil, "", ErrUnsupportedKey
	}
	return key, k.Alg, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
