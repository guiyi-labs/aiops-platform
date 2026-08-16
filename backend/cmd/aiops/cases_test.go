package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-aiops.local/backend/internal/knowledge"
)

func TestCasesBuiltinHit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--query", "crashloop"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (results); stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Mode: built-in rule catalog") || !strings.Contains(out, "1 case(s)") || !strings.Contains(out, "CrashLoopBackOff") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestCasesBuiltinSeverityFilter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--severity", "critical"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "2 case(s)") {
		t.Fatalf("critical filter should match OOM + node-not-ready (2), got:\n%s", out)
	}
}

func TestCasesNoMatchExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--query", "zebra-ql"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1 (no results)", code)
	}
	if !strings.Contains(stdout.String(), "No matching cases") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestCasesInvalidSeverityExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--severity", "catastrophic"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "must be one of") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCasesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--query", "oom", "-o", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	var result casesResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result.Mode != "builtin" || result.Total != 1 || len(result.Entries) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Entries[0].RuleID != ruleIDPodOOMKilled {
		t.Fatalf("rule = %q", result.Entries[0].RuleID)
	}
}

func TestFilterBuiltinSortOrder(t *testing.T) {
	entries := filterBuiltinCases("", "", 10)
	if len(entries) != len(builtinCases) {
		t.Fatalf("len = %d, want %d", len(entries), len(builtinCases))
	}
	// severity-rank descending: critical (2) → high (3) → warning (1)
	wantRanks := []int{4, 4, 3, 3, 3, 2}
	for i, entry := range entries {
		if rank := severityRank(entry.Severity); rank != wantRanks[i] {
			t.Fatalf("entry %d severity %s rank %d, want %d", i, entry.Severity, rank, wantRanks[i])
		}
	}
}

func TestCasesServerDegrades(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--query", "crashloop", "--server", server.URL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (degraded result still returns hits)", code)
	}
	if !strings.Contains(stderr.String(), "degraded to built-in") {
		t.Fatalf("stderr missing degradation notice: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "built-in rule catalog") {
		t.Fatalf("stdout should show builtin mode after degradation:\n%s", stdout.String())
	}
}

func TestCasesServerServesEntries(t *testing.T) {
	entry := knowledge.Entry{
		ID: 42, SourceDiagnosisID: 9,
		RuleID: ruleIDCrashLoopBackOff, Severity: "high", ResourceKind: "Pod",
		ResourceNamespace: "prod", ResourceName: "api-x",
		Summary: "CrashLoopBackOff in production", RootCauses: []string{"bad config"}, Recommendations: []string{"fix config"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/knowledge/entries") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []knowledge.Entry{entry}})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := runCases([]string{"--query", "crashloop", "--server", server.URL, "-o", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	var result casesResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Mode != "server" || result.Total != 1 || result.Entries[0].ID != 42 {
		t.Fatalf("server result = %+v", result)
	}
}

func TestEntryMatchesKeywords(t *testing.T) {
	entry := builtinCases[0]
	matches := []string{"crashloop", "CrashLoop", "web-7c9d5f4b6-abc12", "Exit"}
	for _, term := range matches {
		if !entryMatches(entry, strings.ToLower(term)) {
			t.Errorf("entry should match %q", term)
		}
	}
	if entryMatches(entry, "postgres") {
		t.Error("entry should not match postgres")
	}
}
