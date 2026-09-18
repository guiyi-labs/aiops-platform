package cluster

import (
	"strings"
	"testing"
)

func TestParseKubeconfigRejectsNonHTTPSAndMissingAuth(t *testing.T) {
	// API server must be an absolute HTTPS URL.
	if _, err := ParseKubeconfig([]byte(testKubeconfig("http://h:8080", "tok", true))); err == nil {
		t.Fatal("non-https server accepted")
	}

	// Neither token nor client certificate: authentication is impossible.
	noAuth := strings.Join([]string{
		"apiVersion: v1", "kind: Config", "current-context: test", "clusters:",
		"- name: c", "  cluster:", "    server: https://h:6443",
		"contexts:", "- name: test", "  context:", "    cluster: c", "    user: u",
		"users:", "- name: u", "  user:", "    token: \"\"",
	}, "\n")
	if _, err := ParseKubeconfig([]byte(noAuth)); err == nil {
		t.Fatal("missing token and cert accepted")
	}

	// Malformed certificate-authority-data must be rejected, not silently
	// trusted.
	badCA := strings.Join([]string{
		"apiVersion: v1", "kind: Config", "current-context: test", "clusters:",
		"- name: c", "  cluster:", "    server: https://h:6443", "    certificate-authority-data: not-base64-cert",
		"contexts:", "- name: test", "  context:", "    cluster: c", "    user: u",
		"users:", "- name: u", "  user:", "    token: tok",
	}, "\n")
	if _, err := ParseKubeconfig([]byte(badCA)); err == nil {
		t.Fatal("invalid CA data accepted")
	}
}

// TestParseKubeconfigHonorsInsecureSkipTLSVerify asserts the real-cluster
// access path: a kubeconfig that requests insecure-skip-tls-verify must produce a
// transport with InsecureSkipVerify=true, exactly what lets the platform talk
// to a kind cluster whose SAN does not include the API server hostname.
func TestParseKubeconfigHonorsInsecureSkipTLSVerify(t *testing.T) {
	raw := []byte(testKubeconfig("https://h:6443", "tok", true))
	if !strings.Contains(string(raw), "insecure-skip-tls-verify: true") {
		t.Fatal("testKubeconfig helper did not enable the insecure flag")
	}
	cfg, err := ParseKubeconfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transport == nil || cfg.Transport.TLSClientConfig == nil || !cfg.Transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify not true despite kubeconfig flag")
	}
}
