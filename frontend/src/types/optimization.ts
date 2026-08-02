// Types for the read-only optimization analyzers (M61-M70).
//
// These mirror the backend contracts exactly:
//   - cis.Status            (internal/cis/model.go)
//   - finops.WasteSummary   (internal/finops/advisor.go)
//   - deprecatedapi.Status  (internal/deprecatedapi/model.go)
//   - netpolicy.Status      (internal/netpolicy/model.go)
//   - imagepolicy.Status    (internal/imagepolicy/model.go)
//   - gitopsdrift.Status    (internal/gitopsdrift/model.go)
//   - finding.Finding       (internal/finding) — shared by every analyzer
//
// Every endpoint is read-only: the server collects an observation bundle and
// runs a pure analyzer. Nothing in this module mutates cluster state (ADR 0004).

/** Severity levels emitted by the analyzers. */
export type FindingSeverity = 'info' | 'warning' | 'critical'

/** Identifies the Kubernetes object a finding is about. */
export interface ResourceCitation {
  kind: string
  namespace?: string
  name: string
  uid?: string
  resource_version?: string
}

/**
 * A single read-only observation. Shared verbatim by the CIS and
 * deprecated-API analyzers (both alias `finding.Finding` server-side), so the
 * console renders them through one component.
 */
export interface OptimizationFinding {
  code: string
  severity: string
  summary: string
  resource: ResourceCitation
  details?: Record<string, string>
  observed_at: string
}

/* ---------------------------------------------------------------- CIS ---- */

export interface CISStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/* ------------------------------------------------------------- FinOps ---- */

/** CPU quantities are nanocores; memory quantities are bytes. `-1` means unset. */
export interface FinOpsQuantity {
  cpu_request: number
  cpu_limit: number
  mem_request: number
  mem_limit: number
}

export interface FinOpsRecommendation {
  cluster_id: number
  namespace: string
  workload_kind: string
  workload_name: string
  container_name: string
  suggested_requests: FinOpsQuantity
  suggested_limits: FinOpsQuantity
  severity: string
  rationale: string
  code: string
  /** Estimated monthly cost of idle requested resources across all replicas. */
  monthly_waste_usd: number
  replicas: number
}

export interface FinOpsWasteSummary {
  cluster_id: number
  containers_evaluated: number
  containers_over_provisioned: number
  monthly_waste_usd: number
  cpu_idle_cores: number
  mem_idle_gb: number
  recommendations: FinOpsRecommendation[]
  evaluated_at: string
}

/** Optional per-request cost override; omitted means the server default. */
export interface CostRate {
  per_core_month: number
  per_gb_month: number
}

/* ----------------------------------------------------- Deprecated API ---- */

export interface DeprecatedAPIStatus {
  cluster_id: number
  target_minor: number
  total: number
  removed: number
  deprecated: number
  clean: number
  findings: OptimizationFinding[]
  evaluated_at: string
}

/* ------------------------------------------------------------- Network ---- */

/**
 * Result of the static network reachability / NetworkPolicy posture analysis.
 * `total` counts evaluated checks (not objects); the inventory counters below
 * describe the scope the checks ran over.
 */
export interface NetworkStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  namespaces_total: number
  pods_total: number
  policies_total: number
  services_total: number
  /** Pods selected by at least one ingress policy. */
  ingress_covered_pods: number
  /** Pods selected by at least one egress policy. */
  egress_covered_pods: number
  /** Namespaces with a default-deny ingress baseline. */
  isolated_namespaces: number
  /** NodePort / LoadBalancer services. */
  exposed_services: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * Image supply-chain and reproducibility rollup (imagepolicy.Status).
 *
 * The analyzer never contacts a registry: it reasons statically over the image
 * references the workloads declare, which is what makes a later CVE response
 * possible at all — an image pinned only by a mutable tag cannot be reasoned
 * about after the fact.
 */
export interface ImageStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  /** Distinct image references (repository + tag + digest) in use. */
  images_total: number
  /** Containers referencing an image, including init containers. */
  containers_total: number
  /** Distinct images referenced by :latest or with no tag at all. */
  mutable_tag_images: number
  /** Distinct images referenced by tag only, without a digest pin. */
  unpinned_images: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * GitOps configuration-drift rollup (gitopsdrift.Status).
 *
 * The analyzer is pure and offline (ADR 0004): it only compares the live
 * object against the `kubectl.kubernetes.io/last-applied-configuration`
 * annotation that a GitOps tool (kubectl apply / Flux / Argo CD) wrote, and
 * never re-applies or mutates anything. A resource in a managed namespace with
 * no such annotation is reported as unmanaged (drift can never be reconciled
 * for it).
 */
export interface GitOpsStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  /** Resources observed across the cluster (workloads + ConfigMap/Secret). */
  resources_total: number
  /** Resources whose live spec/data no longer matches last-applied. */
  drifted_resources: number
  /** Managed-namespace resources with no last-applied annotation. */
  unmanaged_resources: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * M70 capacity-trend prediction: cluster CPU/memory allocatable capacity plus
 * the linear projection fitted over the observed usage window.
 */
export interface CapacityStatus {
  cluster_id: number
  evaluated_at: string
  /** Resources evaluated (CPU + memory), how many are at risk, and the rest. */
  total: number
  failed: number
  passed: number
  cpu_capacity_nanocores: number
  mem_capacity_bytes: number
  /** Current fitted utilization ratio (0-1). */
  cpu_current_pct: number
  mem_current_pct: number
  /** Days until 100% utilization at the fitted growth rate; -1 when not growing. */
  cpu_saturation_in_days: number
  mem_saturation_in_days: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * M71 policy-as-code posture: workload manifests checked against the
 * declarative baseline (resources, security context, host access, probes).
 */
export interface PolicyStatus {
  cluster_id: number
  evaluated_at: string
  /** Rule checks evaluated (per container + per pod level); failed = findings. */
  total: number
  failed: number
  passed: number
  workloads_total: number
  containers_total: number
  /** Workloads whose every checked container passed every rule. */
  compliant_workloads: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * M76 HPA scaling posture: HorizontalPodAutoscaler target-metric presence,
 * max-replica headroom and utilization vs target.
 */
export interface HPAStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  hpas_total: number
  at_max_replicas_count: number
  over_target_count: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}

/**
 * M77 PDB protection posture: PodDisruptionBudget coverage of replicable
 * workloads, budget achievability and current disruption state.
 */
export interface PDBStatus {
  cluster_id: number
  evaluated_at: string
  total: number
  failed: number
  passed: number
  workloads_total: number
  pdbs_total: number
  /** Replicable workloads with no matching PDB. */
  unprotected_workloads: number
  by_severity: Record<string, number>
  by_family: Record<string, number>
  findings: OptimizationFinding[]
}
