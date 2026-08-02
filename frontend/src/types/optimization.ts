// Types for the read-only optimization analyzers (M61-M67).
//
// These mirror the backend contracts exactly:
//   - cis.Status            (internal/cis/model.go)
//   - finops.WasteSummary   (internal/finops/advisor.go)
//   - deprecatedapi.Status  (internal/deprecatedapi/model.go)
//   - netpolicy.Status      (internal/netpolicy/model.go)
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
