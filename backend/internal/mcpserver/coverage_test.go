package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewDefaultsVersionWhenEmpty(t *testing.T) {
	server := New(&fakeBackend{}, DefaultTools(), "")
	if server.version != "dev" {
		t.Fatalf("version = %q, want dev", server.version)
	}
}

// failingReader makes the transport fail, which is the one condition Serve
// treats as terminal.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stdin exploded") }

func TestServeReturnsTransportError(t *testing.T) {
	var out strings.Builder
	err := New(&fakeBackend{}, DefaultTools(), "test").Serve(context.Background(), failingReader{}, &out)
	if err == nil {
		t.Fatal("a failing transport must be reported rather than looped on")
	}
	if !strings.Contains(err.Error(), "read request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolsCallRejectsNonObjectArguments(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"list_diagnoses","arguments":"not-an-object"}}`)
	if !strings.Contains(errorMessage(t, replies[0]), "must be an object") {
		t.Fatalf("unexpected error: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("a malformed call must never reach the backend")
	}
}

func TestToolsCallRequiresAName(t *testing.T) {
	backend := &fakeBackend{}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"arguments":{}}}`)
	if !strings.Contains(errorMessage(t, replies[0]), "requires a tool name") {
		t.Fatalf("unexpected error: %v", replies[0])
	}
	if backend.calls != 0 {
		t.Fatal("a nameless call must never reach the backend")
	}
}

func TestToolsCallAcceptsAbsentArgumentsObject(t *testing.T) {
	backend := &fakeBackend{body: []byte(`{}`)}
	replies := exchange(t, backend,
		`{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"knowledge_stats"}}`)
	if _, hasError := replies[0]["error"]; hasError {
		t.Fatalf("a tool taking no arguments must accept an absent arguments object: %v", replies[0])
	}
	if backend.gotPath != "/api/v1/aiops/knowledge/stats" {
		t.Fatalf("path = %q", backend.gotPath)
	}
	if len(backend.gotQuery) != 0 {
		t.Fatalf("query = %v, want none", backend.gotQuery)
	}
}

func TestHTTPBackendReportsUnbuildableRequest(t *testing.T) {
	// A bracketed IPv6 host with no closing bracket cannot form a URL.
	backend := NewHTTPBackend("http://[::1", "", time.Second)
	_, err := backend.Get(context.Background(), "/api/v1/diagnoses", nil)
	if err == nil {
		t.Fatal("an unbuildable request must be reported")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Fatalf("unexpected error: %v", err)
	}
}
