// Package insight is the M81 closed-loop correlation layer. It turns a
// read-only posture/optimization finding into a deterministic "runbook":
// which deterministic diagnosis routes apply, which M52 inspection rules
// corroborate the signal, where a cited AI explanation can be generated
// (M55), and which controlled operations are dry-run preview candidates
// (M19). It is a pure mapping over existing contracts and NEVER touches a
// cluster, mutates state, or widens the security boundary (ADR 0004).
package insight

// DiagnosisRoute describes one deterministic diagnosis path that applies to
// the finding's resource kind. The route is executed via the existing
// read-only diagnosis API (POST /clusters/:id/diagnoses).
type DiagnosisRoute struct {
	ResourceKind string   `json:"resource_kind"`
	RuleIDs      []string `json:"rule_ids"`
	Summary      string   `json:"summary"`
}

// InspectionRule is an M52 catalog rule that corroborates the finding domain.
type InspectionRule struct {
	RuleCode   string `json:"rule_code"`
	SignalCode string `json:"signal_code"`
	Summary    string `json:"summary"`
}

// AIExplanation is the M55 cited-explanation entry point. Generating an
// explanation requires an existing confirmed diagnosis; the runbook only
// describes the target so the console can deep-link to it.
type AIExplanation struct {
	Endpoint string `json:"endpoint"`
	Summary  string `json:"summary"`
}

// OperationCandidate is a dry-run preview candidate (M19 / M44). The runbook
// only advertises the action shape; the actual preview is executed by the
// existing remediation preview API and stays dry-run until explicit confirm.
type OperationCandidate struct {
	Action      string `json:"action"`
	TargetKind  string `json:"target_kind"`
	DryRunFirst bool   `json:"dry_run_first"`
	Summary     string `json:"summary"`
}

// Runbook is the closed-loop plan for one finding.
type Runbook struct {
	ClusterID   int64                `json:"cluster_id"`
	Domain      string               `json:"domain"`
	FindingCode string               `json:"finding_code"`
	Kind        string               `json:"kind"`
	Namespace   string               `json:"namespace,omitempty"`
	Name        string               `json:"name"`
	Diagnoses   []DiagnosisRoute     `json:"diagnoses"`
	Inspection  []InspectionRule     `json:"inspection"`
	AI          *AIExplanation       `json:"ai_explanation,omitempty"`
	Operations  []OperationCandidate `json:"operations"`
	ReadOnly    bool                 `json:"read_only"`
}

// diagnosisByKind maps a resource kind to the applicable deterministic
// diagnosis route. The rule IDs are the compiled-in M18/M43 rule constants.
var diagnosisByKind = map[string]DiagnosisRoute{
	"Pod": {
		ResourceKind: "Pod",
		RuleIDs: []string{
			"pod.image_pull_backoff.v1",
			"pod.crash_loop_backoff.v1",
			"pod.pending.v1",
			"pod.oom_killed.v1",
		},
		Summary: "Run the pod diagnosis chain (image pull, crash loop, pending, OOM-killed).",
	},
	"Deployment": {
		ResourceKind: "Deployment",
		RuleIDs:      []string{"deployment.replicas_unavailable.v1"},
		Summary:      "Run the deployment replica-unavailability diagnosis.",
	},
	"Service": {
		ResourceKind: "Service",
		RuleIDs:      []string{"service.no_ready_endpoints.v1"},
		Summary:      "Run the service no-ready-endpoints diagnosis.",
	},
	"Node": {
		ResourceKind: "Node",
		RuleIDs:      []string{"node.not_ready.v1", "node.pressure.v1"},
		Summary:      "Run the node readiness and pressure diagnosis chain.",
	},
	"Ingress": {
		ResourceKind: "Ingress",
		RuleIDs:      []string{"ingress.backend_unavailable.v1"},
		Summary:      "Run the ingress backend-unavailable diagnosis.",
	},
	"PersistentVolumeClaim": {
		ResourceKind: "PersistentVolumeClaim",
		RuleIDs:      []string{"persistentvolumeclaim.pending.v1"},
		Summary:      "Run the PVC pending diagnosis.",
	},
	"HorizontalPodAutoscaler": {
		ResourceKind: "HorizontalPodAutoscaler",
		RuleIDs:      []string{"horizontalpodautoscaler.saturated.v1"},
		Summary:      "Run the HPA saturation diagnosis.",
	},
}

// inspectionByDomain maps an analyzer domain to the M52 rules that best
// corroborate findings of that family. Unknown domains yield no rules (the
// runbook still carries the diagnosis/explanation steps).
var inspectionByDomain = map[string][]InspectionRule{
	"network": {
		{RuleCode: "ingress_backend_unhealthy", SignalCode: "inspect.network.ingress_backend.v1", Summary: "Check Ingress backends for zero ready endpoints."},
	},
	"hpa": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check for unavailable replicas that HPA saturation can explain."},
	},
	"pdb": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check replicas unavailable under disruption budget constraints."},
	},
	"image": {
		{RuleCode: "pod_restart_loop", SignalCode: "inspect.workload.pod_restart_loop.v1", Summary: "Check pod restart loops from image / runtime failures."},
		{RuleCode: "container_oom_killed", SignalCode: "inspect.workload.oom_killed.v1", Summary: "Check OOM-killed containers (memory limits vs usage)."},
	},
	"capacity": {
		{RuleCode: "node_pressure", SignalCode: "inspect.node.pressure.v1", Summary: "Check node pressure conditions behind capacity trends."},
	},
	"policy": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check replicas unavailable from probe/resource-policy failures."},
	},
	"finops": {
		{RuleCode: "container_oom_killed", SignalCode: "inspect.workload.oom_killed.v1", Summary: "Check OOM events as a signal of under-provisioned limits."},
	},
	"ingress": {
		{RuleCode: "ingress_backend_unhealthy", SignalCode: "inspect.network.ingress_backend.v1", Summary: "Check Ingress backend health behind exposure findings."},
	},
	"gitops": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check replica drift against the GitOps desired state."},
	},
	"cis": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check privileged workload impact (CIS RBAC/PSA findings)."},
	},
	"deprecated_api": {
		{RuleCode: "workload_replicas_unavailable", SignalCode: "inspect.workload.replicas_unavailable.v1", Summary: "Check workload health on a deprecated API version."},
	},
}

// operationByKind lists dry-run preview candidates per resource kind. The
// actions mirror the remediation catalog; previews are always dry-run first.
var operationByKind = map[string][]OperationCandidate{
	"Deployment": {
		{Action: "deployment.rollout_restart", TargetKind: "Deployment", DryRunFirst: true, Summary: "Preview a rollout restart (annotation patch, dry-run)."},
		{Action: "deployment.scale", TargetKind: "Deployment", DryRunFirst: true, Summary: "Preview a replica-scale change (dry-run)."},
		{Action: "deployment.image_update", TargetKind: "Deployment", DryRunFirst: true, Summary: "Preview a container image update (dry-run)."},
		{Action: "deployment.rollback", TargetKind: "Deployment", DryRunFirst: true, Summary: "Preview a revision rollback (dry-run)."},
	},
	"Node": {
		{Action: "cordon", TargetKind: "Node", DryRunFirst: true, Summary: "Preview cordoning the node before maintenance (dry-run)."},
	},
	"CronJob": {
		{Action: "cronjob.suspend", TargetKind: "CronJob", DryRunFirst: true, Summary: "Preview suspending the CronJob (dry-run)."},
	},
}

// Resolve builds the closed-loop runbook for one posture finding. The mapping
// is deterministic and read-only; no cluster access happens here.
func Resolve(clusterID int64, domain, kind, namespace, name, findingCode string) Runbook {
	rb := Runbook{
		ClusterID:   clusterID,
		Domain:      domain,
		FindingCode: findingCode,
		Kind:        kind,
		Namespace:   namespace,
		Name:        name,
		ReadOnly:    true,
	}
	if route, ok := diagnosisByKind[kind]; ok {
		rb.Diagnoses = append(rb.Diagnoses, route)
	}
	if rules, ok := inspectionByDomain[domain]; ok {
		rb.Inspection = append(rb.Inspection, rules...)
	}
	if len(rb.Diagnoses) > 0 {
		rb.AI = &AIExplanation{
			Endpoint: "/api/v1/diagnoses/{diagnosis_id}/explanations",
			Summary:  "Generate a cited AI explanation once the diagnosis is confirmed (M55).",
		}
	}
	if ops, ok := operationByKind[kind]; ok {
		rb.Operations = append(rb.Operations, ops...)
	}
	return rb
}
