package deployment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseAllowlistRejectsReciprocalLicenses(t *testing.T) {
	root := repositoryRoot(t)
	allowlistPath := filepath.Join(root, "docs", "security", "license-allowlist.json")
	contents, err := os.ReadFile(allowlistPath)
	if err != nil {
		t.Fatalf("read license allowlist: %v", err)
	}
	var allowlist struct {
		AllowedLicenses        []string `json:"allowedLicenses"`
		ReviewRequiredLicenses []string `json:"reviewRequiredLicenses"`
	}
	if err := json.Unmarshal(contents, &allowlist); err != nil {
		t.Fatalf("parse license allowlist: %v", err)
	}
	if len(allowlist.AllowedLicenses) == 0 {
		t.Fatal("license allowlist must declare at least one allowed license")
	}
	allowed := make(map[string]bool, len(allowlist.AllowedLicenses))
	for _, name := range allowlist.AllowedLicenses {
		allowed[name] = true
	}
	for _, reciprocal := range []string{"GPL", "LGPL", "AGPL", "UNKNOWN", "SEE-LICENSE"} {
		if allowed[reciprocal] {
			t.Errorf("reciprocal or unknown license %q must not appear on the allowed list", reciprocal)
		}
	}
	reviewed := make(map[string]bool, len(allowlist.ReviewRequiredLicenses))
	for _, name := range allowlist.ReviewRequiredLicenses {
		reviewed[name] = true
	}
	for _, required := range []string{"GPL", "LGPL", "UNKNOWN", "SEE-LICENSE"} {
		if !reviewed[required] {
			t.Errorf("reviewRequiredLicenses must include %q", required)
		}
	}
}

func TestDependencyLicenseInventoryStaysWithinAllowlist(t *testing.T) {
	root := repositoryRoot(t)
	allowlistPath := filepath.Join(root, "docs", "security", "license-allowlist.json")
	allowlistBytes, err := os.ReadFile(allowlistPath)
	if err != nil {
		t.Fatalf("read license allowlist: %v", err)
	}
	var allowlist struct {
		AllowedLicenses []string `json:"allowedLicenses"`
	}
	if err := json.Unmarshal(allowlistBytes, &allowlist); err != nil {
		t.Fatalf("parse license allowlist: %v", err)
	}
	allowed := make(map[string]bool, len(allowlist.AllowedLicenses))
	for _, name := range allowlist.AllowedLicenses {
		allowed[name] = true
	}

	inventoryPath := filepath.Join(root, "docs", "supply-chain", "dependency-licenses.md")
	contents, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatalf("read dependency license inventory: %v", err)
	}
	text := string(contents)

	tables := []struct {
		header string
	}{
		{"## Go production dependencies"},
		{"## Frontend production dependencies"},
	}
	for _, table := range tables {
		start := strings.Index(text, table.header)
		if start < 0 {
			t.Fatalf("inventory is missing section %q", table.header)
		}
		start += len(table.header)
		end := len(text)
		if next := strings.Index(text[start:], "\n## "); next >= 0 {
			end = start + next
		}
		section := text[start:end]
		lines := strings.Split(section, "\n")
		if len(lines) < 2 {
			t.Fatalf("inventory section %q has no rows", table.header)
		}
		headerSeen := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "|--") || strings.HasPrefix(trimmed, "| Package") {
				headerSeen = true
				continue
			}
			if !headerSeen {
				continue
			}
			if !strings.HasPrefix(trimmed, "|") {
				continue
			}
			columns := strings.Split(trimmed, "|")
			if len(columns) < 4 {
				t.Errorf("invalid inventory row %q", trimmed)
				continue
			}
			license := strings.TrimSpace(columns[3])
			if license == "" {
				continue
			}
			if !allowed[license] {
				t.Errorf("dependency license %q in section %q is not on the allowlist; record an ADR or remove the dependency", license, table.header)
			}
		}
	}
}
