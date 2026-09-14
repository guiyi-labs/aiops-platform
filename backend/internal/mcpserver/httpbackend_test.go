package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPBackendSendsTokenQueryAndAccept(t *testing.T) {
	var gotAuth, gotAccept, gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	// A trailing slash on the base URL must not double up in the endpoint.
	backend := NewHTTPBackend(server.URL+"/", "secret-token", 5*time.Second)
	body, err := backend.Get(context.Background(), "/api/v1/diagnoses",
		map[string]string{"limit": "5", "status": "open"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != `{"items":[]}` {
		t.Fatalf("body = %q", body)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept = %q", gotAccept)
	}
	if gotPath != "/api/v1/diagnoses" {
		t.Fatalf("path = %q", gotPath)
	}
	// url.Values encodes with sorted keys, so the request line is reproducible.
	if gotQuery != "limit=5&status=open" {
		t.Fatalf("query = %q, want sorted encoding", gotQuery)
	}
}

func TestHTTPBackendOmitsAuthorizationWithoutToken(t *testing.T) {
	var gotAuth string
	var hadHeader bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, hadHeader = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	backend := NewHTTPBackend(server.URL, "", time.Second)
	if _, err := backend.Get(context.Background(), "/api/v1/x", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hadHeader {
		t.Fatalf("an empty token must not send an Authorization header, got %q", gotAuth)
	}
}

func TestHTTPBackendRefusesPathsThatCouldEscapeTheBase(t *testing.T) {
	backend := NewHTTPBackend("http://127.0.0.1:1", "", time.Second)
	for _, path := range []string{
		"api/v1/diagnoses",       // relative without a leading slash
		"http://evil.example/x",  // absolute URL replaces the base
		"//evil.example/x",       // protocol-relative authority
		"https://evil.example/x", // same, other scheme
		"",                       // empty
	} {
		if _, err := backend.Get(context.Background(), path, nil); err == nil {
			t.Fatalf("path %q was accepted", path)
		}
	}
}

func TestHTTPBackendMapsNon2xxToError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	backend := NewHTTPBackend(server.URL, "stale-token", time.Second)
	_, err := backend.Get(context.Background(), "/api/v1/diagnoses", nil)
	if err == nil {
		t.Fatal("a 401 must surface as an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error %q does not name the status", err)
	}
}

func TestHTTPBackendRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := make([]byte, 32*1024)
		for written := 0; written <= maxBackendResponseBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	backend := NewHTTPBackend(server.URL, "", 10*time.Second)
	_, err := backend.Get(context.Background(), "/api/v1/diagnoses", nil)
	if err == nil {
		t.Fatal("an oversized response must be refused rather than buffered")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPBackendReportsTransportFailure(t *testing.T) {
	// Port 1 is reserved and nothing listens there.
	backend := NewHTTPBackend("http://127.0.0.1:1", "", 500*time.Millisecond)
	_, err := backend.Get(context.Background(), "/api/v1/diagnoses", nil)
	if err == nil {
		t.Fatal("an unreachable platform must surface as an error")
	}
	if !strings.Contains(err.Error(), "call platform API") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewHTTPBackendDefaultsTimeout(t *testing.T) {
	if got := NewHTTPBackend("http://example.invalid", "", 0).client.Timeout; got != 30*time.Second {
		t.Fatalf("timeout = %v, want 30s default", got)
	}
	if got := NewHTTPBackend("http://example.invalid", "", -time.Second).client.Timeout; got != 30*time.Second {
		t.Fatalf("negative timeout = %v, want 30s default", got)
	}
}
