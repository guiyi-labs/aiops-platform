package oidc

import (
	"context"
	"crypto"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// maxJWKSKeys bounds the number of keys parsed from a provider JWKS to prevent
// a malicious or buggy provider from exhausting memory.
const maxJWKSKeys = 16

// ErrUnknownKey indicates the provider JWKS does not contain a usable key with
// the requested kid. Verification must fail closed.
var ErrUnknownKey = fmt.Errorf("oidc: unknown signing key")

// JWKSCache is a bounded, TTL-based JWKS cache that supports key rotation
// without a restart. It fetches the provider JWKS once and reuses the parsed
// keys until the cache TTL expires or an unknown kid forces a refresh.
//
// Rotation semantics (ADR 0052):
//   - The fast path returns a cached key immediately when the kid is present
//     and the cache is fresh.
//   - When a token references a kid that is absent (or the cache is stale), a
//     single-flight refresh fetches the current JWKS. If the new JWKS contains
//     the kid it is returned; otherwise verification fails closed.
//   - When the provider retires a key, the next refresh drops it, so tokens
//     signed with the retired key stop validating.
//   - Concurrent refreshes are coalesced: only one HTTP request runs.
type JWKSCache struct {
	client            *http.Client
	jwksURI           string
	allowedAlgorithms map[string]struct{}
	cacheTTL          time.Duration
	now               func() time.Time

	mu        sync.Mutex
	keys      map[string]cachedKey
	expires   time.Time
	refreshIn *flight
}

type cachedKey struct {
	key  crypto.PublicKey
	algo string
}

// flight coalesces concurrent refreshes: the first caller performs the fetch
// and followers wait on done, then read the shared result field.
type flight struct {
	done   chan struct{}
	result error
}

// JWKSCacheConfig adapts the platform configuration into cache parameters.
type JWKSCacheConfig struct {
	JWKSURI                  string
	AllowedSigningAlgorithms []string
	FetchTimeout             time.Duration
	CacheTTL                 time.Duration
}

// NewJWKSCache constructs a JWKS cache bound to the discovered jwks_uri and the
// approved signing algorithms. FetchTimeout bounds each HTTP call; CacheTTL
// bounds how long a fetched key set is reused before a mandatory refresh.
func NewJWKSCache(cfg JWKSCacheConfig) *JWKSCache {
	timeout := cfg.FetchTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedSigningAlgorithms))
	for _, algorithm := range cfg.AllowedSigningAlgorithms {
		allowed[algorithm] = struct{}{}
	}
	return &JWKSCache{
		client:            newBoundedHTTPClient(timeout),
		jwksURI:           cfg.JWKSURI,
		allowedAlgorithms: allowed,
		cacheTTL:          ttl,
		now:               time.Now,
	}
}

// KeyByID returns the cached signing key for kid, refreshing the JWKS when the
// kid is unknown or the cache is stale. It fails closed with ErrUnknownKey when
// the provider does not publish a usable key for kid.
func (c *JWKSCache) KeyByID(ctx context.Context, kid string) (crypto.PublicKey, string, error) {
	c.mu.Lock()
	if c.keys != nil && c.now().Before(c.expires) {
		if key, ok := c.keys[kid]; ok {
			c.mu.Unlock()
			return key.key, key.algo, nil
		}
	}
	f := c.startRefreshLocked()
	c.mu.Unlock()

	select {
	case <-f.done:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	if f.result != nil {
		return nil, "", f.result
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if key, ok := c.keys[kid]; ok {
		return key.key, key.algo, nil
	}
	return nil, "", ErrUnknownKey
}

// Refresh forces a JWKS fetch, coalescing with any in-flight refresh. It is
// used on startup, after discovery refresh, and in tests.
func (c *JWKSCache) Refresh(ctx context.Context) error {
	c.mu.Lock()
	if c.refreshIn != nil {
		f := c.refreshIn
		c.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		return f.result
	}
	f := c.startRefreshLocked()
	c.mu.Unlock()
	select {
	case <-f.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return f.result
}

func (c *JWKSCache) startRefreshLocked() *flight {
	if c.refreshIn != nil {
		return c.refreshIn
	}
	f := &flight{done: make(chan struct{})}
	c.refreshIn = f
	go c.runRefresh(f)
	return f
}

func (c *JWKSCache) runRefresh(f *flight) {
	err := c.fetchAndCache(context.Background())
	c.mu.Lock()
	c.refreshIn = nil
	c.mu.Unlock()
	f.result = err
	close(f.done)
}

func (c *JWKSCache) fetchAndCache(ctx context.Context) error {
	if err := requireHTTPSURL(c.jwksURI); err != nil {
		return err
	}
	var jwks JWKS
	if err := fetchJSON(ctx, c.client, c.jwksURI, &jwks); err != nil {
		return err
	}
	if len(jwks.Keys) == 0 {
		return fmt.Errorf("oidc: JWKS contains no keys")
	}
	if len(jwks.Keys) > maxJWKSKeys {
		return fmt.Errorf("oidc: JWKS exceeds %d keys", maxJWKSKeys)
	}
	keys := make(map[string]cachedKey, len(jwks.Keys))
	seenKIDs := make(map[string]struct{}, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		if _, dup := seenKIDs[jwk.KID]; dup {
			return fmt.Errorf("oidc: JWKS contains duplicate kid %q", jwk.KID)
		}
		seenKIDs[jwk.KID] = struct{}{}
		key, algo, err := jwk.publicKey(c.allowedAlgorithms)
		if err != nil {
			continue // skip keys that are not usable for an approved algorithm
		}
		keys[jwk.KID] = cachedKey{key: key, algo: algo}
	}
	if len(keys) == 0 {
		return fmt.Errorf("oidc: JWKS contains no usable signing keys for the approved algorithms")
	}
	c.mu.Lock()
	c.keys = keys
	c.expires = c.now().Add(c.cacheTTL)
	c.mu.Unlock()
	return nil
}
