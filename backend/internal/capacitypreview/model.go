// Package capacitypreview provides a read-only "capacity-aware preview" for
// the optimization center (M113-2). Given a candidate workload's resource
// requests and a bundle of node observations, it computes each node's
// remaining headroom (allocatable - current usage) for CPU, memory, GPU and
// storage, checks every constraint, and ranks the nodes from best fit to
// worst — explaining "why fits / why not" per constraint together with the
// freshness of the underlying data.
//
// The package is pure (ADR 0004): Preview takes only a caller-supplied
// observation bundle and returns a ranked preview. It never touches a
// cluster, never mutates state, and never produces anything executable — the
// output is a remediation *preview only*, consistent with the platform rule
// that no write path bypasses audit + confirmation.
package capacitypreview

import (
	"sort"
	"strconv"
	"time"

	"k8s-aiops.local/backend/internal/finops"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ConstraintStatus classifies one resource constraint check.
type ConstraintStatus string

const (
	// ConstraintSatisfied: the node has enough remaining headroom.
	ConstraintSatisfied ConstraintStatus = "satisfied"
	// ConstraintViolated: the node cannot accommodate the request.
	ConstraintViolated ConstraintStatus = "violated"
	// ConstraintUnknown: the observation bundle does not carry the data
	// needed to evaluate this constraint (missing usage or allocatable).
	// Unknown is fail-closed: it never counts as a fit.
	ConstraintUnknown ConstraintStatus = "unknown"
)

// WorkloadRequest is the candidate workload's resource demands in the same
// units the platform uses elsewhere (nanocores for CPU, bytes for memory).
// GPU is a plain device count; storage is bytes of ephemeral-storage.
type WorkloadRequest struct {
	CPURequestNanocores int64 `json:"cpu_request_nanocores"`
	MemRequestBytes     int64 `json:"mem_request_bytes"`
	GPURequest          int64 `json:"gpu_request"`
	StorageRequestBytes int64 `json:"storage_request_bytes"`
}

// NodeObservation is one node's allocatable capacity plus its live usage
// sample. Allocatable values use Kubernetes quantity strings ("4", "16Gi").
type NodeObservation struct {
	Name                  string            `json:"name"`
	Allocatable           map[string]string `json:"allocatable"`
	UsageCPU              string            `json:"usage_cpu"`                   // e.g. "1.2" cores or "1200m"
	UsageMemory           string            `json:"usage_memory"`                // e.g. "8Gi"
	UsageObservedAt       string            `json:"usage_observed_at,omitempty"` // RFC3339; empty when no metric
	Schedulable           bool              `json:"schedulable"`
	StatusReady           bool              `json:"status_ready"`
	AllocatableObservedAt string            `json:"allocatable_observed_at,omitempty"`
}

// Constraint is one evaluated resource constraint for a single node.
type Constraint struct {
	Resource string           `json:"resource"` // cpu | memory | gpu | storage
	Status   ConstraintStatus `json:"status"`
	// Remaining is the headroom observed on the node (nanocores / bytes /
	// device count). 0 when unknown.
	Remaining int64 `json:"remaining,omitempty"`
	// Required is the workload request for this resource.
	Required int64 `json:"required,omitempty"`
	// MissingNames lists the allocatable/usage fields that were absent,
	// used only when Status == unknown.
	MissingNames []string `json:"missing_names,omitempty"`
	// Note is a short human explanation (fills "why fits / why not").
	Note string `json:"note,omitempty"`
}

// NodePreview is one ranked node result.
type NodePreview struct {
	Name        string `json:"name"`
	Schedulable bool   `json:"schedulable"`
	Ready       bool   `json:"ready"`
	Fits        bool   `json:"fits"`
	// UnknownCount counts constraints that could not be evaluated. A node
	// with any unknown constraint is never marked Fits (fail-closed).
	UnknownCount int          `json:"unknown_count"`
	Score        float64      `json:"score"`
	Freshness    string       `json:"freshness,omitempty"` // RFC3339 newest observation; empty when none
	Constraints  []Constraint `json:"constraints"`
}

// Preview is the full ranked result for one evaluation.
type Preview struct {
	ClusterID   int64           `json:"cluster_id"`
	EvaluatedAt time.Time       `json:"evaluated_at"`
	Request     WorkloadRequest `json:"request"`
	// Scope mirrors the resource-context contract (M112): what this preview
	// covers and when the underlying data was observed.
	Scope            string `json:"scope"`
	ObservedAt       string `json:"observed_at,omitempty"`
	NodesTotal       int    `json:"nodes_total"`
	NodesSchedulable int    `json:"nodes_schedulable"`
	FitCount         int    `json:"fit_count"`
	// FailClosed reports whether any node had unknown constraints and was
	// therefore excluded from FitCount — the missing-data case is never
	// treated as healthy.
	FailClosed bool          `json:"fail_closed"`
	Nodes      []NodePreview `json:"nodes"`
}

// Bundle is the read-only observation bundle for one evaluation.
type Bundle struct {
	ClusterID  int64             `json:"cluster_id"`
	Nodes      []NodeObservation `json:"nodes"`
	ObservedAt time.Time         `json:"observed_at"`
}

// Empty reports whether the bundle carries nothing evaluable.
func (in Bundle) Empty() bool { return len(in.Nodes) == 0 }

// ErrEmpty is returned when the bundle has no node observations.
var ErrEmpty = &emptyBundleError{}

type emptyBundleError struct{}

func (e *emptyBundleError) Error() string {
	return "capacity preview: no node observations supplied"
}

// Preview evaluates the candidate workload against every node and returns
// the nodes ranked best-fit first. The ranking is deterministic: nodes that
// fit every constraint come first (most headroom first), then nodes that fit
// partially, then unschedulable / not-ready nodes. A node with any unknown
// constraint is never marked fitting.
func Evaluate(clusterID int64, request WorkloadRequest, bundle Bundle, evaluatedAt time.Time) (Preview, error) {
	if len(bundle.Nodes) == 0 {
		return Preview{}, ErrEmpty
	}
	out := Preview{
		ClusterID:   clusterID,
		EvaluatedAt: evaluatedAt.UTC(),
		Request:     request,
		Scope:       "cluster:" + strconv.FormatInt(clusterID, 10) + ":nodes:allocatable+usage",
		NodesTotal:  len(bundle.Nodes),
	}
	var newest time.Time
	for _, node := range bundle.Nodes {
		np := evaluateNode(request, node)
		out.Nodes = append(out.Nodes, np)
		if node.Schedulable {
			out.NodesSchedulable++
		}
		if np.Fits {
			out.FitCount++
		}
		if np.UnknownCount > 0 {
			out.FailClosed = true
		}
		if observedAt := parseFreshness(node); observedAt.After(newest) {
			newest = observedAt
		}
	}
	if !newest.IsZero() {
		out.ObservedAt = newest.UTC().Format(time.RFC3339)
	}
	sort.SliceStable(out.Nodes, func(a, b int) bool { return rankLess(out.Nodes[a], out.Nodes[b]) })
	return out, nil
}

// evaluateNode computes the per-constraint results for one node.
func evaluateNode(request WorkloadRequest, node NodeObservation) NodePreview {
	np := NodePreview{
		Name:        node.Name,
		Schedulable: node.Schedulable,
		Ready:       node.StatusReady,
	}

	if c := checkCPU(request.CPURequestNanocores, node); c != nil {
		np.Constraints = append(np.Constraints, *c)
	}
	if c := checkMemory(request.MemRequestBytes, node); c != nil {
		np.Constraints = append(np.Constraints, *c)
	}
	if c := checkGPU(request.GPURequest, node); c != nil {
		np.Constraints = append(np.Constraints, *c)
	}
	if c := checkStorage(request.StorageRequestBytes, node); c != nil {
		np.Constraints = append(np.Constraints, *c)
	}

	fits := true
	for _, con := range np.Constraints {
		if con.Status == ConstraintViolated || con.Status == ConstraintUnknown {
			fits = false
		}
		if con.Status == ConstraintUnknown {
			np.UnknownCount++
		}
	}
	np.Fits = fits && np.Schedulable && np.Ready
	np.Score = fitScore(np)
	np.Freshness = newestObservedAt(node)
	return np
}

// ---- per-resource checks --------------------------------------------------

func checkCPU(required int64, node NodeObservation) *Constraint {
	remaining, missing := computeHeadroom("cpu", node, parseCPU)
	if len(missing) > 0 {
		return &Constraint{Resource: "cpu", Status: ConstraintUnknown, Required: required, MissingNames: missing, Note: "缺少 CPU 用量/可分配数据，无法评估"}
	}
	return constraintFromHeadroom("cpu", required, remaining)
}

func checkMemory(required int64, node NodeObservation) *Constraint {
	remaining, missing := computeHeadroom("memory", node, parseMem)
	if len(missing) > 0 {
		return &Constraint{Resource: "memory", Status: ConstraintUnknown, Required: required, MissingNames: missing, Note: "缺少内存用量/可分配数据，无法评估"}
	}
	return constraintFromHeadroom("memory", required, remaining)
}

func checkGPU(required int64, node NodeObservation) *Constraint {
	if required <= 0 {
		return nil // no GPU demand — no constraint to check
	}
	// The metrics server does not expose live GPU usage; headroom is the
	// allocatable count alone (GPU scheduling is an admission decision, not
	// a measure of utilization).
	allocRaw := node.Allocatable["nvidia.com/gpu"]
	remaining := parseGPU(allocRaw)
	if allocRaw == "" {
		return &Constraint{Resource: "gpu", Status: ConstraintUnknown, Required: required, MissingNames: []string{"allocatable.nvidia.com/gpu"}, Note: "缺少 GPU 可分配数据，无法评估"}
	}
	if remaining >= required {
		return &Constraint{Resource: "gpu", Status: ConstraintSatisfied, Remaining: remaining, Required: required, Note: "可用 GPU 数量满足请求"}
	}
	return &Constraint{Resource: "gpu", Status: ConstraintViolated, Remaining: remaining, Required: required, Note: "可用 GPU 数量不足"}
}

func checkStorage(required int64, node NodeObservation) *Constraint {
	if required <= 0 {
		return nil
	}
	remaining, missing := computeHeadroom("ephemeral-storage", node, parseStorage)
	if len(missing) > 0 {
		return &Constraint{Resource: "storage", Status: ConstraintUnknown, Required: required, MissingNames: missing, Note: "缺少存储可分配数据，无法评估"}
	}
	return constraintFromHeadroom("storage", required, remaining)
}

// computeHeadroom computes allocatable - usage for one resource. It returns
// the remaining headroom plus the names of the fields that were missing; a
// non-empty missing list means the result is unusable (fail-closed).
func computeHeadroom(resourceName string, node NodeObservation, parse func(string) int64) (int64, []string) {
	usageRaw := resourceNameUsage(node, resourceName)
	allocRaw := node.Allocatable[resourceName]

	var missing []string
	usage := parse(usageRaw)
	alloc := parse(allocRaw)
	if usageRaw == "" {
		missing = append(missing, "usage."+resourceName)
	}
	if allocRaw == "" {
		missing = append(missing, "allocatable."+resourceName)
	}
	remaining := alloc - usage
	if remaining < 0 {
		remaining = 0
	}
	return remaining, missing
}

func resourceNameUsage(node NodeObservation, resourceName string) string {
	switch resourceName {
	case "cpu":
		return node.UsageCPU
	case "memory":
		return node.UsageMemory
	default:
		return ""
	}
}

func constraintFromHeadroom(resource string, required, remaining int64) *Constraint {
	status, note := ConstraintSatisfied, "剩余资源满足请求"
	if required > remaining {
		status, note = ConstraintViolated, "剩余资源不足"
	}
	return &Constraint{Resource: resource, Status: status, Remaining: remaining, Required: required, Note: note}
}

// fitScore sorts matching nodes by how much CPU headroom remains relative to
// the request (largest headroom first). Violating nodes sort to the bottom.
func fitScore(np NodePreview) float64 {
	if !np.Fits {
		return 0
	}
	for _, con := range np.Constraints {
		if con.Resource == "cpu" {
			return float64(con.Remaining)
		}
	}
	return 0
}

func rankLess(a, b NodePreview) bool {
	if a.Fits != b.Fits {
		return a.Fits
	}
	if a.UnknownCount != b.UnknownCount {
		return a.UnknownCount < b.UnknownCount
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Name < b.Name
}

func newestObservedAt(node NodeObservation) string {
	var newest time.Time
	for _, raw := range []string{node.AllocatableObservedAt, node.UsageObservedAt} {
		if t, err := time.Parse(time.RFC3339, raw); err == nil && t.After(newest) {
			newest = t
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

func parseFreshness(node NodeObservation) time.Time {
	if raw := newestObservedAt(node); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ---- quantity parsers (same units as the rest of the platform) ------------

func parseCPU(raw string) int64 {
	if raw == "" {
		return 0
	}
	q := finops.QuantityFromResourceMap(map[string]string{"cpu": raw}, nil)
	if q.CPURequest != finops.Unset {
		return q.CPURequest
	}
	return 0
}

func parseMem(raw string) int64 {
	if raw == "" {
		return 0
	}
	q := finops.QuantityFromResourceMap(map[string]string{"memory": raw}, nil)
	if q.MemRequest != finops.Unset {
		return q.MemRequest
	}
	return 0
}

func parseGPU(raw string) int64 {
	if raw == "" {
		return 0
	}
	q, err := resource.ParseQuantity(raw)
	if err != nil {
		return 0
	}
	return q.Value()
}

func parseStorage(raw string) int64 {
	return parseMem(raw)
}
