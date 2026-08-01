package oidc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"
)

func rsaJWK(t *testing.T, kid, alg string) (JWK, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return JWK{
		KTY: "RSA", Use: "sig", Alg: alg, KID: kid,
		N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}, key
}

func ecJWK(t *testing.T, kid, alg string) (JWK, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}
	return JWK{
		KTY: "EC", Use: "sig", Alg: alg, KID: kid, Crv: "P-256",
		X: base64.RawURLEncoding.EncodeToString(key.X.Bytes()),
		Y: base64.RawURLEncoding.EncodeToString(key.Y.Bytes()),
	}, key
}

func ed25519JWK(t *testing.T, kid, alg string) (JWK, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return JWK{
		KTY: "OKP", Use: "sig", Alg: alg, KID: kid, Crv: "Ed25519",
		X: base64.RawURLEncoding.EncodeToString(pub),
	}, priv
}

func allowedAlgoSet(algos ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(algos))
	for _, a := range algos {
		out[a] = struct{}{}
	}
	return out
}

func TestJWKRSAPublicKey(t *testing.T) {
	jwk, key := rsaJWK(t, "rsa-1", "RS256")
	pub, algo, err := jwk.publicKey(allowedAlgoSet("RS256", "PS256"))
	if err != nil {
		t.Fatalf("publicKey error = %v", err)
	}
	if algo != "RS256" {
		t.Fatalf("algo = %q, want RS256", algo)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key type = %T, want *rsa.PublicKey", pub)
	}
	if rsaPub.N.Cmp(key.N) != 0 || rsaPub.E != key.E {
		t.Fatal("rsa public key does not match source key")
	}
}

func TestJWKECPublicKey(t *testing.T) {
	jwk, key := ecJWK(t, "ec-1", "ES256")
	pub, algo, err := jwk.publicKey(allowedAlgoSet("ES256"))
	if err != nil {
		t.Fatalf("publicKey error = %v", err)
	}
	if algo != "ES256" {
		t.Fatalf("algo = %q, want ES256", algo)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("public key type = %T, want *ecdsa.PublicKey", pub)
	}
	if ecPub.X.Cmp(key.X) != 0 || ecPub.Y.Cmp(key.Y) != 0 {
		t.Fatal("ec public key does not match source key")
	}
}

func TestJWKEd25519PublicKey(t *testing.T) {
	jwk, priv := ed25519JWK(t, "ed-1", "EdDSA")
	pub, algo, err := jwk.publicKey(allowedAlgoSet("EdDSA"))
	if err != nil {
		t.Fatalf("publicKey error = %v", err)
	}
	if algo != "EdDSA" {
		t.Fatalf("algo = %q, want EdDSA", algo)
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("public key type = %T, want ed25519.PublicKey", pub)
	}
	if !edPub.Equal(priv.Public()) {
		t.Fatal("ed25519 public key does not match source key")
	}
}

func TestJWKRejectsUnsupportedKeys(t *testing.T) {
	rsaKey, _ := rsaJWK(t, "rsa-1", "RS256")
	ecKey, _ := ecJWK(t, "ec-1", "ES256")
	edKey, _ := ed25519JWK(t, "ed-1", "EdDSA")

	cases := map[string]JWK{
		"missing kid":            {KTY: "RSA", Alg: "RS256", N: rsaKey.N, E: rsaKey.E},
		"disallowed algorithm":   {KTY: "RSA", Use: "sig", Alg: "HS256", KID: "k1", N: rsaKey.N, E: rsaKey.E},
		"use=enc":                {KTY: "RSA", Use: "enc", Alg: "RS256", KID: "k1", N: rsaKey.N, E: rsaKey.E},
		"key_ops missing verify": {KTY: "RSA", Use: "sig", KeyOps: []string{"sign"}, Alg: "RS256", KID: "k1", N: rsaKey.N, E: rsaKey.E},
		"rsa wrong kty":          {KTY: "EC", Use: "sig", Alg: "RS256", KID: "k1", N: rsaKey.N, E: rsaKey.E},
		"rsa short modulus":      {KTY: "RSA", Use: "sig", Alg: "RS256", KID: "k1", N: base64.RawURLEncoding.EncodeToString([]byte("short")), E: rsaKey.E},
		"ec wrong curve":         {KTY: "EC", Use: "sig", Alg: "ES256", KID: "k1", Crv: "P-384", X: ecKey.X, Y: ecKey.Y},
		"ec off curve":           {KTY: "EC", Use: "sig", Alg: "ES256", KID: "k1", Crv: "P-256", X: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), Y: base64.RawURLEncoding.EncodeToString(make([]byte, 32))},
		"ed25519 wrong kty":      {KTY: "RSA", Use: "sig", Alg: "EdDSA", KID: "k1", Crv: "Ed25519", X: edKey.X},
		"ed25519 wrong curve":    {KTY: "OKP", Use: "sig", Alg: "EdDSA", KID: "k1", Crv: "Ed448", X: edKey.X},
		"ed25519 short x":        {KTY: "OKP", Use: "sig", Alg: "EdDSA", KID: "k1", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString([]byte("short"))},
	}
	allowed := allowedAlgoSet("RS256", "PS256", "ES256", "EdDSA")
	for name, jwk := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := jwk.publicKey(allowed); err == nil {
				t.Fatal("publicKey error = nil, want ErrUnsupportedKey")
			}
		})
	}
}
