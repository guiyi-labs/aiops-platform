// Package main seeds the P1 RAG case library (knowledge_entries) with a
// curated set of verified, resolved diagnoses so the demo/replay loop has
// real historical cases to retrieve. Every run is idempotent: seed-owned
// rows are cleared by provenance marker before re-insert, and the knowledge
// repository's natural-key dedup keeps re-runs stable.
package main

import (
	"time"

	"k8s-aiops.local/backend/internal/knowledge"
)

// seedProvenanceMarker prefixes the diagnosis summary for every row this tool
// owns, so a re-run can locate and clear only its own rows (the knowledge
// entries cascade via the source_diagnosis_id FK).
const seedProvenanceMarker = "seed:"

// seedClusterName is the synthetic cluster the seed diagnoses attach to. It is
// upserted by name, so repeated runs reuse the same row.
const seedClusterName = "seed-knowledge-demo"

// seedCase is one curated, resolved diagnosis destined for the case library.
type seedCase struct {
	ruleID            string
	severity          string
	resourceKind      string
	resourceNamespace string
	resourceName      string
	summary           string
	rootCauses        []string
	recommendations   []string
}

// buildSeedEntries materializes the curated catalog into knowledge.Entry
// values. notBefore anchors noted_at so retrieval ordering is deterministic
// (newest first) — seed entries are intentionally dated in the past so live,
// freshly-resolved diagnoses sort ahead of them in demos.
func buildSeedEntries(notBefore time.Time) []knowledge.Entry {
	cases := []seedCase{
		{
			ruleID:            "pod.crash_loop_backoff.v1",
			severity:          "high",
			resourceKind:      "Pod",
			resourceNamespace: "default",
			resourceName:      "aiops-demo-7d9f8b6c5d",
			summary:           "Container restart loop: last termination reason CrashLoopBackOff after app startup panic.",
			rootCauses:        []string{"Application exits on boot (missing env/config or unhandled exception)"},
			recommendations: []string{
				"Inspect the previous container logs: kubectl logs --previous",
				"Verify required env vars and config mounts are present",
				"Relax liveness probe initialDelaySeconds if startup is slow",
			},
		},
		{
			ruleID:            "pod.oom_killed.v1",
			severity:          "high",
			resourceKind:      "Pod",
			resourceNamespace: "default",
			resourceName:      "api-gateway-0",
			summary:           "Container exceeded its memory limit and was OOMKilled, then restart-looped.",
			rootCauses:        []string{"Working-set memory exceeds the configured container limit"},
			recommendations: []string{
				"Raise memory request/limit to cover the observed working set",
				"Profile the heap for leaks under peak traffic",
				"Add a memory-aware liveness probe to fail fast instead of OOM",
			},
		},
		{
			ruleID:            "pod.image_pull_backoff.v1",
			severity:          "high",
			resourceKind:      "Pod",
			resourceNamespace: "default",
			resourceName:      "web-7c9f4d5b6x",
			summary:           "Image pull failed repeatedly; pod stuck in ImagePullBackOff.",
			rootCauses:        []string{"Referenced image tag does not exist or registry auth is missing"},
			recommendations: []string{
				"Confirm the image tag exists in the registry",
				"Attach a valid imagePullSecret for private registries",
				"Pin the image by digest to avoid tag drift",
			},
		},
		{
			ruleID:            "pod.pending.v1",
			severity:          "warning",
			resourceKind:      "Pod",
			resourceNamespace: "default",
			resourceName:      "job-runner-xy99",
			summary:           "Pod unschedulable: insufficient CPU/memory or unmatched node selector/taint.",
			rootCauses:        []string{"No node satisfies the pod's resource requests or node affinity"},
			recommendations: []string{
				"Scale the node pool or free capacity on target nodes",
				"Lower the pod's CPU/memory requests if over-provisioned",
				"Reconcile nodeSelector/taints/tolerations",
			},
		},
		{
			ruleID:            "node.not_ready.v1",
			severity:          "critical",
			resourceKind:      "Node",
			resourceNamespace: "",
			resourceName:      "worker-node-3",
			summary:           "Node NotReady: kubelet stopped reporting or container runtime is down.",
			rootCauses:        []string{"Kubelet unreachable or node under disk/memory pressure"},
			recommendations: []string{
				"SSH the node and check kubelet/systemd status",
				"Free disk space / verify kubelet client cert validity",
				"Cordon and drain before investigating hardware",
			},
		},
		{
			ruleID:            "service.no_ready_endpoints.v1",
			severity:          "high",
			resourceKind:      "Service",
			resourceNamespace: "default",
			resourceName:      "frontend",
			summary:           "Service has no Ready endpoints: selector matches no healthy pods.",
			rootCauses:        []string{"Pod labels do not match the Service selector or pods are not Ready"},
			recommendations: []string{
				"Verify pod labels match the Service selector exactly",
				"Check pod readiness probe and endpoint health",
				"Inspect EndpointSlice for the Service",
			},
		},
		{
			ruleID:            "ingress.backend_unavailable.v1",
			severity:          "high",
			resourceKind:      "Ingress",
			resourceNamespace: "default",
			resourceName:      "public-api",
			summary:           "Ingress backend unavailable: referenced Service has no healthy endpoints.",
			rootCauses:        []string{"Backend Service has no Ready pods behind the Ingress path"},
			recommendations: []string{
				"Confirm the backend Service has Ready endpoints",
				"Check Ingress path/port and TLS certificate validity",
				"Validate the ingress-controller logs for routing errors",
			},
		},
		{
			ruleID:            "horizontalpodautoscaler.saturated.v1",
			severity:          "warning",
			resourceKind:      "HorizontalPodAutoscaler",
			resourceNamespace: "default",
			resourceName:      "api-hpa",
			summary:           "HPA pinned at max replicas while CPU stays above target.",
			rootCauses:        []string{"Workload cannot scale past maxReplicas to meet the CPU target"},
			recommendations: []string{
				"Raise the HPA maxReplicas ceiling",
				"Optimize the application's CPU profile per replica",
				"Confirm metrics-server is reporting accurate CPU",
			},
		},
		{
			ruleID:            "persistentvolumeclaim.pending.v1",
			severity:          "high",
			resourceKind:      "PersistentVolumeClaim",
			resourceNamespace: "default",
			resourceName:      "data-pvc",
			summary:           "PVC stuck Pending: no matching PersistentVolume could be bound.",
			rootCauses:        []string{"No PV satisfies the storage class, size, or access mode"},
			recommendations: []string{
				"Verify the StorageClass exists and is provisionable",
				"Provision a matching PV or relax the request",
				"Check accessMode/size against available capacity",
			},
		},
		{
			ruleID:            "deployment.replicas_unavailable.v1",
			severity:          "warning",
			resourceKind:      "Deployment",
			resourceNamespace: "default",
			resourceName:      "web",
			summary:           "Deployment has fewer Available replicas than desired (pods crash or Pending).",
			rootCauses:        []string{"ReplicaSet pods are CrashLooping, Pending, or failing readiness"},
			recommendations: []string{
				"Run kubectl rollout status deployment/web",
				"Inspect ReplicaSet events and pod reasons",
				"Roll back to the last healthy revision if a bad deploy caused it",
			},
		},
	}

	entries := make([]knowledge.Entry, 0, len(cases))
	for i, c := range cases {
		// Stagger noted_at so the list is ordered deterministically within the
		// seed set; live diagnoses resolve after these and sort ahead in demos.
		noted := notBefore.Add(time.Duration(i) * time.Minute)
		entries = append(entries, knowledge.Entry{
			RuleID:            c.ruleID,
			Severity:          c.severity,
			ResourceKind:      c.resourceKind,
			ResourceNamespace: c.resourceNamespace,
			ResourceName:      c.resourceName,
			Summary:           c.summary,
			RootCauses:        c.rootCauses,
			Recommendations:   c.recommendations,
			NotedAt:           noted,
		})
	}
	return entries
}
