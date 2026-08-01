package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func jwksHandler(keys ...JWK) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JWKS{Keys: keys})
	})
}

func newTestJWKSCache(t *testing.T, server *httptest.Server, allowedAlgos []string) *JWKSCache {
	t.Helper()
	allowed := make(map[string]struct{}, len(allowedAlgos))
	for _, a := range allowedAlgos {
		allowed[a] = struct{}{}
	}
	return &JWKSCache{
		client:            testHTTPClient(t, server),
		jwksURI:           server.URL + "/jwks",
		allowedAlgorithms: allowed,
		cacheTTL:          time.Hour,
		now:               time.Now,
	}
}

func TestJWKSCacheFetchAndKeyByID(t *testing.T) {
	jwk, key := rsaJWK(t, "rsa-1", "RS256")
	server := newTestTLSServer(t, jwksHandler(jwk))
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	pub, algo, err := cache.KeyByID(context.Background(), "rsa-1")
	if err != nil {
		t.Fatalf("KeyByID error = %v", err)
	}
	if algo != "RS256" {
		t.Fatalf("algo = %q, want RS256", algo)
	}
	if pub == nil {
		t.Fatal("public key = nil")
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key type = %T, want *rsa.PublicKey", pub)
	}
	if rsaPub.N.Cmp(key.N) != 0 || rsaPub.E != key.E {
		t.Fatal("returned key does not match source public key")
	}
}

func TestJWKSCacheFastPathDoesNotRefetch(t *testing.T) {
	jwk, _ := rsaJWK(t, "rsa-1", "RS256")
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JWKS{Keys: []JWK{jwk}})
	})
	server := newTestTLSServer(t, handler)
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	for i := 0; i < 5; i++ {
		if _, _, err := cache.KeyByID(context.Background(), "rsa-1"); err != nil {
			t.Fatalf("KeyByID %d error = %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("provider called %d times, want 1 (fast path)", got)
	}
}

func TestJWKSCacheUnknownKidFailsClosed(t *testing.T) {
	jwk, _ := rsaJWK(t, "rsa-1", "RS256")
	server := newTestTLSServer(t, jwksHandler(jwk))
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	if _, _, err := cache.KeyByID(context.Background(), "missing-kid"); err == nil {
		t.Fatal("KeyByID error = nil, want ErrUnknownKey for missing kid")
	}
}

func TestJWKSCacheRotationDropsRetiredKey(t *testing.T) {
	oldJWK, _ := rsaJWK(t, "old-1", "RS256")
	newJWK, _ := rsaJWK(t, "new-1", "RS256")

	current := &atomic.Value{}
	current.Store([]JWK{oldJWK})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JWKS{Keys: current.Load().([]JWK)})
	})
	server := newTestTLSServer(t, handler)
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	// Prime the cache with the old key.
	if _, _, err := cache.KeyByID(context.Background(), "old-1"); err != nil {
		t.Fatalf("prime with old key error = %v", err)
	}

	// Provider rotates: publish only the new key.
	current.Store([]JWK{newJWK})

	// Force a refresh by requesting the new kid (cache is fresh, so a missing
	// kid triggers a refresh). After refresh, the new key validates and the old
	// kid must no longer be found.
	if _, _, err := cache.KeyByID(context.Background(), "new-1"); err != nil {
		t.Fatalf("KeyByID new-1 error = %v", err)
	}
	// Old kid should now fail closed after the rotation refresh.
	cache.mu.Lock()
	keys := cache.keys
	cache.mu.Unlock()
	if _, ok := keys["old-1"]; ok {
		t.Fatal("retired key old-1 still present after rotation")
	}
	if _, ok := keys["new-1"]; !ok {
		t.Fatal("new key new-1 missing after rotation")
	}
}

func TestJWKSCacheTTLExpiryForcesRefresh(t *testing.T) {
	jwk1, _ := rsaJWK(t, "k1", "RS256")
	jwk2, _ := rsaJWK(t, "k2", "RS256")
	current := &atomic.Value{}
	current.Store([]JWK{jwk1})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JWKS{Keys: current.Load().([]JWK)})
	})
	server := newTestTLSServer(t, handler)

	now := time.Now()
	cache := &JWKSCache{
		client:            testHTTPClient(t, server),
		jwksURI:           server.URL + "/jwks",
		allowedAlgorithms: map[string]struct{}{"RS256": {}},
		cacheTTL:          time.Hour,
		now:               func() time.Time { return now },
	}

	if _, _, err := cache.KeyByID(context.Background(), "k1"); err != nil {
		t.Fatalf("KeyByID k1 error = %v", err)
	}

	// Rotate the published key set and advance time past the TTL.
	current.Store([]JWK{jwk2})
	now = now.Add(2 * time.Hour)

	if _, _, err := cache.KeyByID(context.Background(), "k2"); err != nil {
		t.Fatalf("KeyByID k2 after TTL error = %v", err)
	}
	cache.mu.Lock()
	keys := cache.keys
	cache.mu.Unlock()
	if _, ok := keys["k1"]; ok {
		t.Fatal("retired key k1 still present after TTL refresh")
	}
	if _, ok := keys["k2"]; !ok {
		t.Fatal("new key k2 missing after TTL refresh")
	}
}

func TestJWKSCacheSingleFlightCoalescesRefresh(t *testing.T) {
	jwk, _ := rsaJWK(t, "rsa-1", "RS256")
	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // slow response to force overlap
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JWKS{Keys: []JWK{jwk}})
	})
	server := newTestTLSServer(t, handler)
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, _, err := cache.KeyByID(context.Background(), "rsa-1")
			done <- err
		}()
	}
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent KeyByID error = %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("provider called %d times, want 1 (single-flight)", got)
	}
}

func TestJWKSCacheRejectsDuplicateKids(t *testing.T) {
	jwk, _ := rsaJWK(t, "dup-1", "RS256")
	server := newTestTLSServer(t, jwksHandler(jwk, jwk))
	cache := newTestJWKSCache(t, server, []string{"RS256"})
	if _, _, err := cache.KeyByID(context.Background(), "dup-1"); err == nil {
		t.Fatal("KeyByID error = nil, want duplicate kid error")
	}
}

func TestJWKSCacheRejectsHTTPJWKSURI(t *testing.T) {
	cache := &JWKSCache{
		jwksURI:           "http://insecure.example.com/jwks",
		allowedAlgorithms: map[string]struct{}{"RS256": {}},
		cacheTTL:          time.Hour,
		now:               time.Now,
		client:            newBoundedHTTPClient(5 * time.Second),
	}
	if _, _, err := cache.KeyByID(context.Background(), "k1"); err == nil {
		t.Fatal("KeyByID error = nil, want HTTPS JWKS URI error")
	}
}

func TestJWKSCacheRejectsTooManyKeys(t *testing.T) {
	keys := make([]JWK, maxJWKSKeys+1)
	for i := range keys {
		jwk, _ := rsaJWK(t, "k", "RS256")
		jwk.KID = "k-" + string(rune('a'+i))
		keys[i] = jwk
	}
	server := newTestTLSServer(t, jwksHandler(keys...))
	cache := newTestJWKSCache(t, server, []string{"RS256"})
	if _, _, err := cache.KeyByID(context.Background(), "k-a"); err == nil {
		t.Fatal("KeyByID error = nil, want too-many-keys error")
	}
}

func TestJWKSCacheSkipsUnusableKeys(t *testing.T) {
	good, _ := rsaJWK(t, "good-1", "RS256")
	bad := good
	bad.KID = "bad-1"
	bad.KTY = "EC" // wrong kty for RS256, makes it unusable
	server := newTestTLSServer(t, jwksHandler(good, bad))
	cache := newTestJWKSCache(t, server, []string{"RS256"})

	if _, _, err := cache.KeyByID(context.Background(), "good-1"); err != nil {
		t.Fatalf("KeyByID good-1 error = %v", err)
	}
	if _, _, err := cache.KeyByID(context.Background(), "bad-1"); err == nil {
		t.Fatal("KeyByID bad-1 error = nil, want ErrUnknownKey for unusable key")
	}
}
