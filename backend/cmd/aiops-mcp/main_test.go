package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresAPIURL(t *testing.T) {
	t.Setenv("AIOPS_API_URL", "")
	if err := run(); err == nil {
		t.Fatal("an unset AIOPS_API_URL must be an error rather than a default")
	}
}

func TestRunRejectsUnparsableTimeout(t *testing.T) {
	t.Setenv("AIOPS_API_URL", "http://127.0.0.1:8080")
	t.Setenv("AIOPS_API_TIMEOUT", "three seconds")
	if err := run(); err == nil {
		t.Fatal("an unparsable timeout must be an error")
	}
}

// The happy path is a protocol round trip over the process's own streams, which
// is the closest a unit test gets to the real stdio transport a client uses.
func TestRunServesARequestOverStdio(t *testing.T) {
	t.Setenv("AIOPS_API_URL", "http://127.0.0.1:8080")
	t.Setenv("AIOPS_API_TIMEOUT", "5s")
	t.Setenv("AIOPS_API_TOKEN", "test-token")

	dir := t.TempDir()
	stdinPath := filepath.Join(dir, "stdin")
	stdoutPath := filepath.Join(dir, "stdout")
	if err := os.WriteFile(stdinPath,
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}

	stdin, err := os.Open(stdinPath)
	if err != nil {
		t.Fatalf("open stdin fixture: %v", err)
	}
	defer stdin.Close()

	stdout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	defer stdout.Close()

	originalIn, originalOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdin, stdout
	defer func() { os.Stdin, os.Stdout = originalIn, originalOut }()

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	captured, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	if !strings.Contains(string(captured), `"tools"`) {
		t.Fatalf("tools/list produced no catalogue: %s", captured)
	}
}
