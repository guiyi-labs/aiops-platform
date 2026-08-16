package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/knowledge"
)

// Rule IDs referenced by the built-in catalog. They match the compiled-in
// diagnosis rules (internal/diagnosis) so data stays interoperable with the
// platform's case library.
const (
	ruleIDCrashLoopBackOff   = "pod.crash_loop_backoff.v1"
	ruleIDImagePullBackOff   = "pod.image_pull_backoff.v1"
	ruleIDPodOOMKilled       = "pod.oom_killed.v1"
	ruleIDPodPending         = "pod.pending.v1"
	ruleIDServiceNoEndpoints = "service.no_ready_endpoints.v1"
	ruleIDNodeNotReady       = "node.not_ready.v1"
)

// builtinCases is the offline rule catalog the cases command falls back to when
// no --server is given (or the knowledge API is unreachable). It mirrors the
// knowledge.Entry model exactly, so the same type and severity semantics are
// shared with the platform's RAG case library.
var builtinCases = []knowledge.Entry{
	{
		ID: 1, SourceDiagnosisID: 1001,
		RuleID:            ruleIDCrashLoopBackOff,
		Severity:          string(knowledge.SeverityHigh),
		ResourceKind:      "Pod",
		ResourceNamespace: "demo",
		ResourceName:      "web-7c9d5f4b6-abc12",
		Summary:           "CrashLoopBackOff: container starts then exits repeatedly; pod keeps restarting.",
		RootCauses: []string{
			"Startup command, arguments or configuration make the process exit",
			"Missing dependency: config, Secret or upstream service",
		},
		Recommendations: []string{
			"Read the previous container logs before the last exit",
			"Verify ConfigMap, Secret and dependency connectivity",
		},
		NotedAt: knowledgeTime("2026-08-14T09:30:00Z"),
	},
	{
		ID: 2, SourceDiagnosisID: 1002,
		RuleID:            ruleIDImagePullBackOff,
		Severity:          string(knowledge.SeverityHigh),
		ResourceKind:      "Pod",
		ResourceNamespace: "demo",
		ResourceName:      "api-6b8f4c9d7-def34",
		Summary:           "ImagePullBackOff: pod cannot pull the container image from the registry.",
		RootCauses: []string{
			"Image name or tag does not exist",
			"Registry network or DNS unreachable",
			"imagePullSecret missing or bound to the wrong ServiceAccount",
		},
		Recommendations: []string{
			"Verify the image registry, name and tag",
			"Inspect imagePullSecret and ServiceAccount references",
		},
		NotedAt: knowledgeTime("2026-08-13T14:05:00Z"),
	},
	{
		ID: 3, SourceDiagnosisID: 1003,
		RuleID:            ruleIDPodOOMKilled,
		Severity:          string(knowledge.SeverityCritical),
		ResourceKind:      "Pod",
		ResourceNamespace: "demo",
		ResourceName:      "worker-5d4c9f8e2-ghi56",
		Summary:           "OOMKilled: container terminated by the kernel for exceeding its memory limit.",
		RootCauses: []string{
			"Container memory limit lower than the actual working set",
			"Application memory leak",
		},
		Recommendations: []string{
			"Compare memory working set vs limits over time",
			"Right-size limits or investigate application memory behavior",
		},
		NotedAt: knowledgeTime("2026-08-12T22:40:00Z"),
	},
	{
		ID: 4, SourceDiagnosisID: 1004,
		RuleID:            ruleIDPodPending,
		Severity:          string(knowledge.SeverityHigh),
		ResourceKind:      "Pod",
		ResourceNamespace: "demo",
		ResourceName:      "db-7f8e6a5c4-jkl78",
		Summary:           "Pending: pod has not been scheduled; cluster lacks resources or constraints block it.",
		RootCauses: []string{
			"Insufficient node resources",
			"NodeSelector, affinity or taints block scheduling",
		},
		Recommendations: []string{
			"Read the FailedScheduling event for the specific reason",
			"Compare node allocatable resources and pod scheduling constraints",
		},
		NotedAt: knowledgeTime("2026-08-11T07:15:00Z"),
	},
	{
		ID: 5, SourceDiagnosisID: 1005,
		RuleID:            ruleIDServiceNoEndpoints,
		Severity:          string(knowledge.SeverityWarning),
		ResourceKind:      "Service",
		ResourceNamespace: "demo",
		ResourceName:      "frontend",
		Summary:           "Service has no ready endpoints: traffic to the service has no healthy backends.",
		RootCauses: []string{
			"Backend pods are not running or not ready",
			"Selector mismatch between Service and pods",
		},
		Recommendations: []string{
			"Check backend pod status and readiness",
			"Verify the Service selector matches pod labels",
		},
		NotedAt: knowledgeTime("2026-08-10T18:20:00Z"),
	},
	{
		ID: 6, SourceDiagnosisID: 1006,
		RuleID:            ruleIDNodeNotReady,
		Severity:          string(knowledge.SeverityCritical),
		ResourceKind:      "Node",
		ResourceNamespace: "",
		ResourceName:      "node-a",
		Summary:           "NodeNotReady: node is unreachable or kubelet stopped reporting readiness.",
		RootCauses: []string{
			"kubelet down or unresponsive",
			"Node networking or control-plane connectivity lost",
		},
		Recommendations: []string{
			"Check kubelet status and logs on the node",
			"Verify node network and control-plane connectivity",
		},
		NotedAt: knowledgeTime("2026-08-09T11:50:00Z"),
	},
}

// knowledgeTime parses fixed RFC3339 timestamps into the CLI's built-in
// catalog, keeping output deterministic across runs.
func knowledgeTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic("invalid built-in case timestamp: " + value)
	}
	return parsed.UTC()
}

type casesResult struct {
	Mode    string            `json:"mode"` // "server" or "builtin"
	Server  string            `json:"server,omitempty"`
	Query   string            `json:"query"`
	Total   int               `json:"total"`
	Entries []knowledge.Entry `json:"entries"`
}

func runCases(args []string, stdout, stderr io.Writer) int {
	var (
		query    string
		severity string
		limit    int
		server   string
		format   string
	)
	flags := flag.NewFlagSet("cases", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&query, "query", "", "search term matched against rule, summary, causes and resource name")
	flags.StringVar(&severity, "severity", "", "minimum severity filter: info, warning, high, critical")
	flags.IntVar(&limit, "limit", 10, "maximum number of results")
	flags.StringVar(&server, "server", "", "platform server URL; when omitted or unreachable the command degrades to the built-in rule catalog")
	flags.StringVar(&format, "o", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "aiops cases: unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "aiops cases: -o must be text or json, got %q\n", format)
		return 2
	}
	if limit <= 0 {
		fmt.Fprintf(stderr, "aiops cases: --limit must be positive\n")
		return 2
	}
	if severity != "" && !validSeverity(severity) {
		fmt.Fprintf(stderr, "aiops cases: --severity must be one of info, warning, high, critical, got %q\n", severity)
		return 2
	}

	result := queryCases(query, severity, limit, server, stderr)
	if format == "json" {
		writeJSON(stdout, result)
	} else {
		writeCasesText(stdout, result, stderr)
	}
	if result.Total > 0 {
		return 0
	}
	return 1
}

// queryCases resolves the case list. With a --server it first asks the
// platform knowledge API; on any failure (endpoint absent, unreachable, bad
// payload) it degrades to the built-in catalog rather than failing the
// command — matching the platform's "knowledge outage never blocks diagnosis"
// discipline.
func queryCases(query, severity string, limit int, server string, stderr io.Writer) casesResult {
	if server != "" {
		if entries, ok := fetchServerCases(server, query, severity, limit); ok {
			return casesResult{Mode: "server", Server: server, Query: query, Total: len(entries), Entries: entries}
		}
		fmt.Fprintf(stderr, "aiops cases: knowledge API at %s unavailable — degraded to built-in rule catalog\n", server)
	}
	entries := filterBuiltinCases(query, severity, limit)
	return casesResult{Mode: "builtin", Query: query, Total: len(entries), Entries: entries}
}

func validSeverity(severity string) bool {
	switch severity {
	case "info", "warning", "high", "critical":
		return true
	default:
		return false
	}
}

func filterBuiltinCases(query, severity string, limit int) []knowledge.Entry {
	term := strings.ToLower(strings.TrimSpace(query))
	filtered := make([]knowledge.Entry, 0, len(builtinCases))
	for _, entry := range builtinCases {
		if severity != "" && severityRank(entry.Severity) < severityRank(severity) {
			continue
		}
		if term != "" && !entryMatches(entry, term) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Severity != filtered[j].Severity {
			return severityRank(filtered[i].Severity) > severityRank(filtered[j].Severity)
		}
		return filtered[i].NotedAt.After(filtered[j].NotedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func entryMatches(entry knowledge.Entry, term string) bool {
	haystack := strings.ToLower(entry.RuleID + " " + entry.Summary + " " + entry.ResourceName + " " + entry.ResourceKind + " " + strings.Join(entry.RootCauses, " "))
	return strings.Contains(haystack, term)
}

// fetchServerCases calls GET {server}/api/v1/knowledge/entries. The endpoint
// is not part of the v0.1.0 platform API surface yet; when it becomes
// available the CLI upgrades automatically, and until then any failure returns
// ok=false so the caller degrades.
func fetchServerCases(server, query, severity string, limit int) ([]knowledge.Entry, bool) {
	endpoint := strings.TrimRight(server, "/") + "/api/v1/knowledge/entries?limit=" + url.QueryEscape(fmt.Sprint(limit))
	if query != "" {
		endpoint += "&query=" + url.QueryEscape(query)
	}
	if severity != "" {
		endpoint += "&severity=" + url.QueryEscape(severity)
	}
	// #nosec G107 -- endpoint is a user-supplied --server URL that the CLI
	// intentionally requests; no proxy/redirect review applies to a CLI tool.
	response, err := http.Get(endpoint)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			_ = response.Body.Close()
		}
		return nil, false
	}
	defer response.Body.Close()
	var body struct {
		Items []knowledge.Entry `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return nil, false
	}
	return body.Items, true
}

func writeCasesText(w io.Writer, result casesResult, stderr io.Writer) {
	mode := "built-in rule catalog"
	if result.Mode == "server" {
		mode = "server " + result.Server
	}
	fmt.Fprintf(w, "Mode: %s\n", mode)
	if result.Total == 0 {
		fmt.Fprintf(w, "No matching cases found for query %q.\n", result.Query)
		return
	}
	fmt.Fprintf(w, "%d case(s) for query %q\n", result.Total, result.Query)
	for index, entry := range result.Entries {
		namespace := entry.ResourceNamespace
		if namespace == "" {
			namespace = "-"
		}
		fmt.Fprintf(w, "\n#%d [%s] %s — %s %s/%s (%s)\n", index+1, entry.Severity, entry.RuleID, entry.ResourceKind, namespace, entry.ResourceName, entry.NotedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "  Summary: %s\n", entry.Summary)
		if len(entry.RootCauses) > 0 {
			fmt.Fprintf(w, "  Root causes:\n")
			for _, cause := range entry.RootCauses {
				fmt.Fprintf(w, "    - %s\n", cause)
			}
		}
		if len(entry.Recommendations) > 0 {
			fmt.Fprintf(w, "  Recommendations:\n")
			for _, recommendation := range entry.Recommendations {
				fmt.Fprintf(w, "    - %s\n", recommendation)
			}
		}
	}
}
