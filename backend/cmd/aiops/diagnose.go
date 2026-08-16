package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// finding is the English, machine-readable view of one matched diagnosis rule
// produced by the deterministic engine. The engine (diagnosis package) keeps
// its Chinese presentation copy; the CLI renders findings for a global
// audience via ruleEnglish.
type finding struct {
	RuleID          string   `json:"rule_id"`
	Severity        string   `json:"severity"`
	Kind            string   `json:"kind"`
	Namespace       string   `json:"namespace,omitempty"`
	Name            string   `json:"name"`
	Summary         string   `json:"summary"`
	RootCauses      []string `json:"root_causes"`
	Recommendations []string `json:"recommendations"`
}

type diagnoseResult struct {
	Mode     string    `json:"mode"` // "cluster" or "demo"
	Server   string    `json:"server,omitempty"`
	Scanned  int       `json:"scanned"`
	Findings []finding `json:"findings"`
	// Degraded explains, when non-empty, why the run fell back to demo data
	// instead of a live cluster (e.g. the default kubeconfig could not be
	// read or parsed). Machine-readable context for -o json.
	Degraded string `json:"degraded_reason,omitempty"`
}

// evaluatePodChain runs the same deterministic order as
// diagnosis.(*Service).evaluatePod: OOM kill → image pull back-off → crash
// loop back-off → pending. It reuses the compiled-in rules verbatim.
func evaluatePodChain(clusterID int64, pod k8sgateway.Pod, events []k8sgateway.Event, observedAt time.Time) (diagnosis.Record, bool) {
	if record, ok := diagnosis.EvaluatePodOOMKilled(clusterID, pod, events, observedAt); ok {
		return record, true
	}
	if record, ok := diagnosis.EvaluateImagePullBackOff(clusterID, pod, events, observedAt); ok {
		return record, true
	}
	if record, ok := diagnosis.EvaluateCrashLoopBackOff(clusterID, pod, events, observedAt); ok {
		return record, true
	}
	if record, ok := diagnosis.EvaluatePodPending(clusterID, pod, events, observedAt); ok {
		return record, true
	}
	return diagnosis.Record{}, false
}

func findingFromRecord(record diagnosis.Record) finding {
	copy := englishFor(record.RuleID)
	rootCauses := record.RootCauses
	if len(copy.RootCauses) > 0 {
		rootCauses = copy.RootCauses
	}
	recommendations := record.Recommendations
	if len(copy.Recommendations) > 0 {
		recommendations = copy.Recommendations
	}
	return finding{
		RuleID:          record.RuleID,
		Severity:        record.Severity,
		Kind:            record.Resource.Kind,
		Namespace:       record.Resource.Namespace,
		Name:            record.Resource.Name,
		Summary:         copy.Summary,
		RootCauses:      rootCauses,
		Recommendations: recommendations,
	}
}

func runDiagnose(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diagnose", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		kubeconfig = fs.String("kubeconfig", "", "path to kubeconfig (default: ~/.kube/config; falls back to built-in demo data when absent)")
		namespace  = fs.String("namespace", "default", "namespace to scan when --pod is not given (demo mode ignores it)")
		pod        = fs.String("pod", "", "diagnose a single pod (namespace/name); scan the namespace when empty")
		format     = fs.String("o", "text", "output format: text or json")
		timeout    = fs.Duration("timeout", 15*time.Second, "per-request timeout for cluster API calls")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "aiops diagnose: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "aiops diagnose: -o must be text or json, got %q\n", *format)
		return 2
	}
	if *timeout <= 0 {
		fmt.Fprintf(stderr, "aiops diagnose: --timeout must be positive\n")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result, err := diagnose(ctx, *kubeconfig, *namespace, *pod)
	if err != nil {
		fmt.Fprintf(stderr, "aiops diagnose: %v\n", err)
		return 2
	}
	if *format == "json" {
		writeJSON(stdout, result)
	} else {
		writeDiagnoseText(stdout, result)
	}
	if len(result.Findings) > 0 {
		return 1
	}
	return 0
}

func diagnose(ctx context.Context, kubeconfigPath, namespace, podName string) (diagnoseResult, error) {
	explicit := kubeconfigPath != ""
	path := kubeconfigPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		config, parseErr := cluster.ParseKubeconfig(raw)
		if parseErr == nil {
			client := &http.Client{Transport: config.Transport, Timeout: 15 * time.Second}
			return diagnoseCluster(ctx, client, config.Server, config.Token, namespace, podName)
		}
		if explicit {
			return diagnoseResult{}, fmt.Errorf("parse kubeconfig: %w", parseErr)
		}
		return diagnoseDemoWithReason(fmt.Sprintf("kubeconfig at %s could not be used (%v); ran against built-in demo data", path, parseErr))
	}
	if explicit {
		return diagnoseResult{}, fmt.Errorf("read kubeconfig: %w", err)
	}
	return diagnoseDemoWithReason(fmt.Sprintf("no kubeconfig found (%v); ran against built-in demo data", err))
}

func diagnoseDemoWithReason(reason string) (diagnoseResult, error) {
	result, err := diagnoseDemo()
	if err != nil {
		return diagnoseResult{}, err
	}
	result.Degraded = reason
	return result, nil
}

func diagnoseDemo() (diagnoseResult, error) {
	pods, events := demoCluster()
	result := diagnoseResult{Mode: "demo", Scanned: len(pods)}
	observedAt := time.Now().UTC()
	for _, pod := range pods {
		if record, ok := evaluatePodChain(0, pod, events[pod.Metadata.UID], observedAt); ok {
			result.Findings = append(result.Findings, findingFromRecord(record))
		}
	}
	sortFindings(result.Findings)
	return result, nil
}

func diagnoseCluster(ctx context.Context, client *http.Client, server, token, namespace, podName string) (diagnoseResult, error) {
	api := &apiClient{client: client, server: strings.TrimRight(server, "/"), token: token}
	pods, err := api.listPods(ctx, namespace)
	if err != nil {
		return diagnoseResult{}, err
	}
	if podName != "" {
		target := normalizedPodName(podName)
		filtered := make([]k8sgateway.Pod, 0, 1)
		for _, pod := range pods {
			if pod.Metadata.Name == target {
				filtered = append(filtered, pod)
				break
			}
		}
		if len(filtered) == 0 {
			return diagnoseResult{}, fmt.Errorf("pod %q not found in namespace %q", target, namespace)
		}
		pods = filtered
	}
	result := diagnoseResult{Mode: "cluster", Server: server, Scanned: len(pods)}
	observedAt := time.Now().UTC()
	for _, pod := range pods {
		events, err := api.podEvents(ctx, namespace, pod.Metadata.UID)
		if err != nil {
			return diagnoseResult{}, fmt.Errorf("fetch events for pod %s: %w", pod.Metadata.Name, err)
		}
		if record, ok := evaluatePodChain(0, pod, events, observedAt); ok {
			result.Findings = append(result.Findings, findingFromRecord(record))
		}
	}
	sortFindings(result.Findings)
	return result, nil
}

func normalizedPodName(name string) string {
	return strings.TrimPrefix(name, "pod/")
}

func sortFindings(findings []finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return severityRank(findings[i].Severity) > severityRank(findings[j].Severity)
		}
		return findings[i].Name < findings[j].Name
	})
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func writeDiagnoseText(w io.Writer, result diagnoseResult) {
	if result.Mode == "demo" {
		fmt.Fprintf(w, "Mode: demo — %s\n", result.Degraded)
	} else {
		fmt.Fprintf(w, "Mode: cluster (%s)\n", result.Server)
	}
	fmt.Fprintf(w, "Scanned %d pod(s), %d finding(s)\n", result.Scanned, len(result.Findings))
	if len(result.Findings) == 0 {
		fmt.Fprintf(w, "No issues found.\n")
		return
	}
	for index, item := range result.Findings {
		fmt.Fprintf(w, "\n#%d [%s] %s — %s/%s\n", index+1, item.Severity, item.RuleID, item.Namespace, item.Name)
		fmt.Fprintf(w, "  Summary: %s\n", item.Summary)
		if len(item.RootCauses) > 0 {
			fmt.Fprintf(w, "  Root causes:\n")
			for _, cause := range item.RootCauses {
				fmt.Fprintf(w, "    - %s\n", cause)
			}
		}
		if len(item.Recommendations) > 0 {
			fmt.Fprintf(w, "  Recommendations:\n")
			for _, recommendation := range item.Recommendations {
				fmt.Fprintf(w, "    - %s\n", recommendation)
			}
		}
	}
}

func writeJSON(w io.Writer, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

// apiClient talks directly to the Kubernetes API server using the connection
// settings parsed from kubeconfig (cluster.ParseKubeconfig). Only two bounded
// read paths are needed: pod fetch (single or list) and pod events.
type apiClient struct {
	client *http.Client
	server string
	token  string
}

type listEnvelope[T any] struct {
	Items []T `json:"items"`
}

func (c *apiClient) getJSON(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.server+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("API %s returned %s: %s", path, response.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (c *apiClient) listPods(ctx context.Context, namespace string) ([]k8sgateway.Pod, error) {
	var envelope listEnvelope[k8sgateway.Pod]
	if err := c.getJSON(ctx, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods", &envelope); err != nil {
		return nil, err
	}
	return envelope.Items, nil
}

func (c *apiClient) podEvents(ctx context.Context, namespace, uid string) ([]k8sgateway.Event, error) {
	if uid == "" {
		return nil, nil
	}
	selector := url.QueryEscape("involvedObject.uid=" + uid)
	var envelope listEnvelope[k8sgateway.Event]
	if err := c.getJSON(ctx, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/events?fieldSelector="+selector, &envelope); err != nil {
		return nil, err
	}
	return envelope.Items, nil
}
