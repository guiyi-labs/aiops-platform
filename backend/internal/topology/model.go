package topology

import "time"

// EdgeKind enumerates the reviewed relationship edge types persisted by M40.
// These are the only supported edge kinds; unlisted relationships are never
// created. Same-name or temporal proximity alone never creates an edge.
type EdgeKind string

const (
	EdgeOwns        EdgeKind = "Owns"        // Deployment→ReplicaSet, ReplicaSet→Pod (via OwnerReference)
	EdgeSelects     EdgeKind = "Selects"     // Service→Pod (via label selector)
	EdgeRoutesTo    EdgeKind = "RoutesTo"    // Ingress/Gateway→Service (via backend config)
	EdgeBackedBy    EdgeKind = "BackedBy"    // Service→EndpointSlice→Pod (via EndpointSlice endpoints)
	EdgeRunsOn      EdgeKind = "RunsOn"      // Pod→Node (via spec.nodeName)
	EdgeMounts      EdgeKind = "Mounts"      // Pod→PVC (via pod volumes)
	EdgeScales      EdgeKind = "Scales"      // HPA→Deployment/ReplicaSet (via scaleTargetRef)
	EdgeProtectedBy EdgeKind = "ProtectedBy" // Workload→PDB (via selector match)
)

// DerivationMethod records how an edge was derived. OwnerReference, selector,
// EndpointSlice and backend evidence remain distinct — they are never merged
// into a single "inferred" bucket.
type DerivationMethod string

const (
	DerivationOwnerReference DerivationMethod = "owner_reference"
	DerivationLabelSelector  DerivationMethod = "label_selector"
	DerivationEndpointSlice  DerivationMethod = "endpointslice"
	DerivationBackendConfig  DerivationMethod = "backend_config"
	DerivationNodeName       DerivationMethod = "node_name"
	DerivationVolumeMount    DerivationMethod = "volume_mount"
	DerivationScaleTarget    DerivationMethod = "scale_target_ref"
	DerivationPDBSelector    DerivationMethod = "pdb_selector"
)

// ResourceCitation is the exact observed entity at an edge endpoint. The
// primary key is cluster_id + kind + UID; a name-only fallback is explicitly
// marked Incomplete. This mirrors signal.ResourceCitation so M40 edges can be
// consumed by M42 correlation without a translation layer.
type ResourceCitation struct {
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	UID        string `json:"uid,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
}

// Edge is a reviewed relationship between two exact Kubernetes entities. Each
// edge records source, target, kind, derivation method, first/last observation
// and a validity interval. Edges are append-only with time validity: when a
// selector changes or a Pod is recreated, the old edge is closed (valid_to
// set) and a new edge is created.
type Edge struct {
	ID              int64            `json:"id"`
	ClusterID       int64            `json:"cluster_id"`
	Kind            EdgeKind         `json:"kind"`
	Source          ResourceCitation `json:"source"`
	Target          ResourceCitation `json:"target"`
	Derivation      DerivationMethod `json:"derivation"`
	FirstObservedAt time.Time        `json:"first_observed_at"`
	LastObservedAt  time.Time        `json:"last_observed_at"`
	ValidFrom       time.Time        `json:"valid_from"`
	ValidTo         *time.Time       `json:"valid_to,omitempty"`
	ReviewEvidence  []EvidenceRef    `json:"review_evidence,omitempty"`
	SourceHash      string           `json:"source_hash,omitempty"`
	// AggregateCount carries the collapsed edge multiplicity when the graph
	// is served by GET /topology/graph?collapse=1 (advisory, read-only).
	AggregateCount int `json:"aggregate_count,omitempty"`
}

// EvidenceRef points at a stable, redacted evidence snapshot. Mirrors
// signal.EvidenceRef for cross-package reuse.
type EvidenceRef struct {
	Kind        string `json:"kind"`
	ID          int64  `json:"id"`
	ContentHash string `json:"content_hash,omitempty"`
}

// EdgeFilter bounds topology edge queries.
type EdgeFilter struct {
	ClusterID int64
	Namespace string
	EdgeKind  EdgeKind
	SourceUID string
	TargetUID string
	ValidAt   *time.Time
	Limit     int
}

// EdgeListResponse is the bounded paginated response.
type EdgeListResponse struct {
	Items     []Edge `json:"items"`
	Total     int64  `json:"total"`
	Truncated bool   `json:"truncated,omitempty"`
}

// TopologyGraph is the evidence-graph payload returned by the topology API.
// Nodes and edges disclose source completeness, truncation and remaining
// counts. Hidden cluster/Namespace data never enters results.
type TopologyGraph struct {
	ClusterID    int64             `json:"cluster_id"`
	Namespace    string            `json:"namespace,omitempty"`
	Nodes        []GraphNode       `json:"nodes"`
	Edges        []Edge            `json:"edges"`
	Completeness GraphCompleteness `json:"completeness"`
	GeneratedAt  time.Time         `json:"generated_at"`
}

type GraphNode struct {
	Resource  ResourceCitation `json:"resource"`
	EdgeCount int              `json:"edge_count"`
}

type GraphCompleteness struct {
	State     string `json:"state"` // complete | partial | unavailable | truncated
	Truncated bool   `json:"truncated,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
}

// ChangeEvent is a normalized platform-operation outcome in the change
// timeline. It links M23-M31 plan/audit/request identities and reports
// missing/forbidden/partial sources separately.
type ChangeEvent struct {
	ID           int64            `json:"id"`
	ClusterID    int64            `json:"cluster_id"`
	Namespace    string           `json:"namespace,omitempty"`
	Kind         string           `json:"kind"` // promotion | backup | maintenance | restore | rollout | audit
	PlanID       string           `json:"plan_id,omitempty"`
	Target       ResourceCitation `json:"target"`
	Action       string           `json:"action"`
	SafeDiffHash string           `json:"safe_diff_hash,omitempty"`
	Revision     string           `json:"revision,omitempty"`
	Actor        string           `json:"actor,omitempty"`
	StartedAt    time.Time        `json:"started_at"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
	Result       string           `json:"result"` // succeeded | failed | pending | partial
	AuditID      *int64           `json:"audit_id,omitempty"`
	RequestID    string           `json:"request_id,omitempty"`
	Evidence     []EvidenceRef    `json:"evidence,omitempty"`
	Confidence   string           `json:"confidence"` // high | low
	Source       string           `json:"source"`     // platform | k8s_event | delivery_adapter
}

// ChangeTimelineFilter bounds change timeline queries.
type ChangeTimelineFilter struct {
	ClusterID int64
	Namespace string
	Kind      string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}

// ChangeTimelineResponse is the bounded paginated response.
type ChangeTimelineResponse struct {
	Items     []ChangeEvent `json:"items"`
	Total     int64         `json:"total"`
	Truncated bool          `json:"truncated,omitempty"`
}

// SetEdgeCount sets the advisory aggregate edge-count on an Edge (used by
// the collapse view transform for large graphs).
func SetEdgeCount(e *Edge, n int) {
	if e == nil {
		return
	}
	e.AggregateCount = n
}

// IncEdgeCount increments the advisory aggregate edge-count on an Edge.
func IncEdgeCount(e *Edge) {
	if e == nil {
		return
	}
	e.AggregateCount++
}
