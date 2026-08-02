// Package imagepolicy performs a read-only image supply-chain and
// reproducibility analysis for one cluster.
//
// It answers the question an operator asks before an incident or an audit:
// "which container images am I actually running, and how reproducible are
// they?" Reproducibility is the foundation of any CVE response: if an image
// is referenced by a mutable tag (e.g. :latest) or by tag alone (no digest
// pin), a rebuild can silently change what runs in production and a CVE fix
// may not land where expected.
//
// The analyzer is pure and offline: it never contacts a registry, never pulls
// a manifest and never mutates anything (ADR 0004). It reasons statically over
// an observation bundle that the caller supplies (or that the M65 collector
// gathers via read-only List calls). Real CVE scoring requires a vulnerability
// source (Trivy / Grype / an advisory API) and is a deliberate follow-up; this
// analyzer delivers the inventory and reproducibility findings that such a
// source would consume.
package imagepolicy

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Finding reuses the platform's canonical read-only posture Finding contract
// (see internal/finding) so the frontend renders image findings uniformly
// with the other optimization analyzers.
type Finding = k8sfinding.Finding

const (
	SeverityInfo     = k8sfinding.SeverityInfo
	SeverityWarning  = k8sfinding.SeverityWarning
	SeverityCritical = k8sfinding.SeverityCritical
)

// Finding family, reported in Finding.Details["family"] and rolled up into
// Status.ByFamily.
const FamilySupplyChain = "supply-chain"

// Finding codes emitted by Evaluate.
const (
	// CodeMutableTag: the image uses :latest or no tag, so a redeploy can
	// silently run a different build.
	CodeMutableTag = "IMG_MUTABLE_TAG"
	// CodeNoDigestPin: the image is referenced by a specific tag but not by
	// digest, so the tag could be re-pointed to a different manifest.
	CodeNoDigestPin = "IMG_NO_DIGEST_PIN"
	// CodePullAlwaysLatest: imagePullPolicy: Always with a mutable tag — every
	// restart re-pulls whatever :latest currently points at.
	CodePullAlwaysLatest = "IMG_PULL_ALWAYS_LATEST"
	// CodeSharedAcrossNamespaces: the same image runs in more than one
	// namespace, widening its blast radius and complicating rollout.
	CodeSharedAcrossNamespaces = "IMG_SHARED_ACROSS_NS"
	// CodeMultipleTags: one image repository is referenced by several tags,
	// an easy source of version skew between workloads.
	CodeMultipleTags = "IMG_MULTIPLE_TAGS"
)

// ContainerRef identifies the workload container that references an image.
type ContainerRef struct {
	Namespace    string `json:"namespace"`
	WorkloadKind string `json:"workload_kind"`
	WorkloadName string `json:"workload_name"`
	Container    string `json:"container"`
}

// ImageInfo is the decomposed form of a container image reference.
type ImageInfo struct {
	// Repository is the registry + path + name, without tag or digest.
	Repository string `json:"repository"`
	// Tag is the tag portion ("" when the reference is digest-only or the tag
	// was omitted, in which case Kubernetes defaults to :latest).
	Tag string `json:"tag,omitempty"`
	// Digest is the @sha256:... portion, when present.
	Digest string `json:"digest,omitempty"`
	// PullPolicy is the container's imagePullPolicy, when supplied.
	PullPolicy string `json:"pull_policy,omitempty"`
}

// ImageUsage links one container to one resolved image reference.
type ImageUsage struct {
	Image     ImageInfo    `json:"image"`
	Container ContainerRef `json:"container"`
}

// Inputs is the read-only observation bundle for one cluster evaluation.
type Inputs struct {
	Usages []ImageUsage `json:"usages,omitempty"`
}

// Empty reports whether the bundle carries nothing to analyze.
func (in Inputs) Empty() bool {
	return len(in.Usages) == 0
}

// Status is the rollup returned for one cluster evaluation.
type Status struct {
	ClusterID   int64     `json:"cluster_id"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	// Total is the number of individual checks evaluated, Failed the number
	// that produced a finding, Passed the remainder.
	Total  int `json:"total"`
	Failed int `json:"failed"`
	Passed int `json:"passed"`
	// Inventory counters give the console a one-line summary of the scope.
	ImagesTotal     int `json:"images_total"`
	ContainersTotal int `json:"containers_total"`
	// MutableTagImages counts distinct images referenced by :latest or no tag.
	MutableTagImages int `json:"mutable_tag_images"`
	// UnpinnedImages counts distinct images referenced by tag (not digest).
	UnpinnedImages int            `json:"unpinned_images"`
	BySeverity     map[string]int `json:"by_severity"`
	ByFamily       map[string]int `json:"by_family"`
	Findings       []Finding      `json:"findings"`
}
