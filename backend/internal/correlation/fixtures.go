package correlation

// Golden replay fixtures for M42. Each fixture is a deterministic
// (EngineInputs, expected) pair that exercises one correlation path. The
// fixtures are the replayable contract: identical inputs + rule/correlation
// versions reproduce identical cases. Tests in fixtures_test.go assert the
// expected outcomes.
//
// The fixtures cover the 9 scenarios required by the M42 plan:
//  1. ImagePull — rollout causes image pull backoff (confirmed)
//  2. CrashLoop — rollout causes crash loop backoff (confirmed)
//  3. OOM — rollout causes OOM killed (confirmed)
//  4. Pending/PVC — storage change causes PVC pending (confirmed)
//  5. NoEndpoints — rollout causes service no ready endpoints (confirmed via topology)
//  6. UnavailableDeploy — rollout causes deployment replicas unavailable (confirmed)
//  7. NodeNotReady/Pressure — maintenance causes node failure (confirmed)
//  8. MetricBreach — rollout causes metric breach (candidate, topology-based)
//  9. BadRollout — rollout succeeded but symptoms persist (contradicted)
//
// All fixtures use stable cluster IDs (1, 2, ...), stable UIDs and stable
// timestamps so replay is byte-identical. Times are UTC.

import "time"

// fixtureClock is the anchor time for all fixtures: 2026-07-31 12:00:00 UTC.
// All observed_at and started_at values are offsets from this anchor so the
// fixtures are stable across runs.
var fixtureClock = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

// fixtureClusterA is the cluster ID used by most fixtures.
const fixtureClusterA int64 = 1

// GoldenFixture is one replayable correlation scenario.
type GoldenFixture struct {
	Name        string
	Description string
	Inputs      EngineInputs
	// ExpectResults is the number of CorrelationResults the engine should
	// produce. Most fixtures produce exactly 1.
	ExpectResults int
	// ExpectConfidence is the expected confidence of the (first) result's case.
	ExpectConfidence ConfidenceClass
	// ExpectRuleID is the expected rule ID of the (first) result.
	ExpectRuleID string
	// ExpectCandidates is the expected number of change candidates.
	ExpectCandidates int
	// ExpectSignalLinks is the expected number of signal links (at least the trigger).
	ExpectSignalLinks int
	// ExpectResourceLinks is the expected number of resource links (at least primary).
	ExpectResourceLinks int
	// ExpectColdStart is true when no change event is in the window and the
	// case should be ConfidenceUnknown.
	ExpectColdStart bool
}

// GoldenFixtures returns all 9 replay scenarios. The order is stable.
func GoldenFixtures() []GoldenFixture {
	return []GoldenFixture{
		imagePullFixture(),
		crashLoopFixture(),
		oomFixture(),
		pvcPendingFixture(),
		noEndpointsFixture(),
		unavailableDeployFixture(),
		nodeNotReadyFixture(),
		metricBreachFixture(),
		badRolloutFixture(),
	}
}

// --- Scenario 1: ImagePull (confirmed) ---
//
// A promotion rolled out a bad image; 5 minutes later the Pod enters
// ImagePullBackOff. The change target UID matches the Pod's owning
// ReplicaSet UID (same_uid via topology). All required factors match and no
// contradicting evidence exists, so the case is ConfidenceConfirmed.
func imagePullFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-5 * time.Minute)
	podUID := "pod-uid-imagepull-001"
	rsUID := "rs-uid-imagepull-001"
	deployUID := "deploy-uid-imagepull-001"

	return GoldenFixture{
		Name:        "image_pull_backoff",
		Description: "rollout precedes image pull backoff with matching topology path",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{
					ID:        101,
					SignalID:  "diag.pod.image_pull_backoff.v1",
					Producer:  "diagnosis",
					ClusterID: fixtureClusterA,
					Namespace: "app",
					Resource: ResourceCitation{
						Kind: "Pod", Namespace: "app", Name: "web-abc123", UID: podUID,
					},
					Severity:   "critical",
					State:      "active",
					Coverage:   "complete",
					Freshness:  observedAt,
					ObservedAt: observedAt,
				},
			},
			Changes: []ChangeEventInput{
				{
					ID:        201,
					ClusterID: fixtureClusterA,
					Namespace: "app",
					Kind:      "rollout",
					PlanID:    "plan-imagepull",
					Target: ResourceCitation{
						Kind: "Deployment", Namespace: "app", Name: "web", UID: deployUID,
					},
					Action:     "rollout_restart",
					Result:     "succeeded",
					Actor:      "operator",
					StartedAt:  changeStart,
					Confidence: "high",
					Source:     "platform",
				},
			},
			Edges: []TopologyEdgeInput{
				{
					ID: 301, ClusterID: fixtureClusterA,
					Kind:   "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "web-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "web-abc123", UID: podUID},
				},
				{
					ID: 302, ClusterID: fixtureClusterA,
					Kind:   "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "web", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "web-xyz", UID: rsUID},
				},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_pod_failure.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 2: CrashLoop (confirmed) ---
//
// A promotion introduced a crashing binary; the Pod enters CrashLoopBackOff.
// Same topology pattern as ImagePull. The change kind is "promotion".
func crashLoopFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-8 * time.Minute)
	podUID := "pod-uid-crashloop-002"
	rsUID := "rs-uid-crashloop-002"
	deployUID := "deploy-uid-crashloop-002"

	return GoldenFixture{
		Name:        "crash_loop_backoff",
		Description: "promotion precedes crash loop backoff with matching topology path",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{
					ID: 102, SignalID: "diag.pod.crash_loop_backoff.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "api-def456", UID: podUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt,
				},
			},
			Changes: []ChangeEventInput{
				{
					ID: 202, ClusterID: fixtureClusterA, Namespace: "app", Kind: "promotion",
					PlanID: "plan-crashloop",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "api", UID: deployUID},
					Action: "image_update", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform",
				},
			},
			Edges: []TopologyEdgeInput{
				{ID: 303, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "api-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "api-def456", UID: podUID}},
				{ID: 304, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "api", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "api-xyz", UID: rsUID}},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_pod_failure.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 3: OOM (confirmed) ---
//
// A rollout raised memory limits; the Pod is OOMKilled. Same topology pattern.
func oomFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-3 * time.Minute)
	podUID := "pod-uid-oom-003"
	rsUID := "rs-uid-oom-003"
	deployUID := "deploy-uid-oom-003"

	return GoldenFixture{
		Name:        "oom_killed",
		Description: "rollout precedes OOM killed with matching topology path",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 103, SignalID: "diag.pod.oom_killed.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "worker-ghi789", UID: podUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 203, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
					PlanID: "plan-oom",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "worker", UID: deployUID},
					Action: "rollout_restart", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
			Edges: []TopologyEdgeInput{
				{ID: 305, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "worker-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "worker-ghi789", UID: podUID}},
				{ID: 306, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "worker", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "worker-xyz", UID: rsUID}},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_pod_failure.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 4: PVC Pending (confirmed) ---
//
// A storage-class change precedes a PVC stuck in Pending. The change target
// UID matches the PVC UID (same_uid). Required factors match.
func pvcPendingFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-15 * time.Minute)
	pvcUID := "pvc-uid-pending-004"

	return GoldenFixture{
		Name:        "pvc_pending",
		Description: "storage change precedes PVC pending with same UID",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 104, SignalID: "diag.persistentvolumeclaim.pending.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "data",
					Resource: ResourceCitation{Kind: "PersistentVolumeClaim", Namespace: "data", Name: "data-claim", UID: pvcUID},
					Severity: "warning", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 204, ClusterID: fixtureClusterA, Namespace: "data", Kind: "promotion",
					PlanID: "plan-storage",
					Target: ResourceCitation{Kind: "PersistentVolumeClaim", Namespace: "data", Name: "data-claim", UID: pvcUID},
					Action: "storage_class_change", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.storage_change_causes_pvc_pending.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 5: NoEndpoints (confirmed via topology) ---
//
// A rollout on the backing Deployment causes the Service to lose ready
// endpoints. The signal is on the Service; the change is on the Deployment.
// same_uid does not match, but topology_distance connects them via the
// BackedBy edge. Required factors (topology_distance, time_distance,
// change_symptom_rule) all match.
func noEndpointsFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-10 * time.Minute)
	svcUID := "svc-uid-noep-005"
	deployUID := "deploy-uid-noep-005"
	rsUID := "rs-uid-noep-005"
	podUID := "pod-uid-noep-005"

	return GoldenFixture{
		Name:        "no_ready_endpoints",
		Description: "rollout precedes service endpoint loss via BackedBy topology",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 105, SignalID: "diag.service.no_ready_endpoints.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Service", Namespace: "app", Name: "frontend", UID: svcUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 205, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
					PlanID: "plan-noep",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "frontend", UID: deployUID},
					Action: "rollout_restart", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
			Edges: []TopologyEdgeInput{
				// Service -> Pod via BackedBy
				{ID: 307, ClusterID: fixtureClusterA, Kind: "BackedBy",
					Source: ResourceCitation{Kind: "Service", Namespace: "app", Name: "frontend", UID: svcUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "frontend-abc", UID: podUID}},
				// Pod owned by ReplicaSet
				{ID: 308, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "frontend-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "frontend-abc", UID: podUID}},
				// ReplicaSet owned by Deployment
				{ID: 309, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "frontend", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "frontend-xyz", UID: rsUID}},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_no_endpoints.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 6: UnavailableDeploy (confirmed) ---
//
// A rollout causes the Deployment to have unavailable replicas. The signal is
// on the Deployment; the change target is the same Deployment (same_uid).
func unavailableDeployFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-7 * time.Minute)
	deployUID := "deploy-uid-unavail-006"

	return GoldenFixture{
		Name:        "replicas_unavailable",
		Description: "rollout precedes deployment replicas unavailable with same UID",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 106, SignalID: "diag.deployment.replicas_unavailable.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "checkout", UID: deployUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 206, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
					PlanID: "plan-unavail",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "checkout", UID: deployUID},
					Action: "image_update", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_unavailable_deployment.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 7: NodeNotReady (confirmed) ---
//
// A maintenance on a Node precedes it becoming NotReady. same_uid matches
// (the change target is the Node).
func nodeNotReadyFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-45 * time.Minute)
	nodeUID := "node-uid-notready-007"

	return GoldenFixture{
		Name:        "node_not_ready",
		Description: "maintenance precedes node not ready with same UID",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 107, SignalID: "diag.node.not_ready.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "",
					Resource: ResourceCitation{Kind: "Node", Namespace: "", Name: "worker-1", UID: nodeUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 207, ClusterID: fixtureClusterA, Namespace: "", Kind: "maintenance",
					PlanID: "plan-node-maint",
					Target: ResourceCitation{Kind: "Node", Namespace: "", Name: "worker-1", UID: nodeUID},
					Action: "cordondrain", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.maintenance_causes_node_failure.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 8: MetricBreach (candidate) ---
//
// A rollout on a Deployment precedes a sustained metric breach signal on a
// Pod. The Pod is related to the Deployment via topology. same_uid does not
// match, but topology_distance and time_distance connect them. The rule
// requires topology_distance, time_distance and change_symptom_rule — all
// present, so confidence should be confirmed when topology path exists.
//
// However, the metric.sustained_breach.v1 signal's primary kind is Pod, and
// the change target is the Deployment. The topology path is
// Deployment -> ReplicaSet -> Pod, which the engine resolves. This produces
// a confirmed case.
func metricBreachFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-20 * time.Minute)
	podUID := "pod-uid-metric-008"
	rsUID := "rs-uid-metric-008"
	deployUID := "deploy-uid-metric-008"

	return GoldenFixture{
		Name:        "metric_breach",
		Description: "rollout precedes sustained metric breach via RunsOn/topology path",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 108, SignalID: "metric.sustained_breach.v1", Producer: "metric",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "load-abc", UID: podUID},
					Severity: "warning", State: "active", Coverage: "partial",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 208, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
					PlanID: "plan-metric",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "load", UID: deployUID},
					Action: "rollout_restart", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
			Edges: []TopologyEdgeInput{
				{ID: 310, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "load-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "load-abc", UID: podUID}},
				{ID: 311, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "load", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "load-xyz", UID: rsUID}},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceConfirmed,
		ExpectRuleID:        "correlation.rollout_causes_metric_breach.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// --- Scenario 9: BadRollout (contradicted) ---
//
// A rollout on an UNRELATED Deployment precedes a Pod crash. There is no
// topology path between the change target (a different Deployment) and the
// signal resource (the crashing Pod). The contradicting factor
// (topology_distance: unreachable) is observed, so the candidate is
// ConfidenceContradicted. The case is retained for audit.
func badRolloutFixture() GoldenFixture {
	observedAt := fixtureClock
	changeStart := fixtureClock.Add(-12 * time.Minute)
	podUID := "pod-uid-badrollout-009"
	rsUID := "rs-uid-badrollout-009"
	deployUID := "deploy-uid-badrollout-009"
	// unrelatedDeployUID is a different Deployment with no topology edge to
	// the crashing Pod — the rollout is coincidental, not causal.
	unrelatedDeployUID := "deploy-uid-unrelated-009"

	return GoldenFixture{
		Name:        "bad_rollout_contradicted",
		Description: "rollout on unrelated deployment precedes pod crash — contradicted candidate",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 109, SignalID: "diag.pod.crash_loop_backoff.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "bad-abc", UID: podUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			Changes: []ChangeEventInput{
				{ID: 209, ClusterID: fixtureClusterA, Namespace: "app", Kind: "rollout",
					PlanID: "plan-badrollout",
					Target: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "unrelated", UID: unrelatedDeployUID},
					Action: "rollout_restart", Result: "succeeded", Actor: "operator",
					StartedAt: changeStart, Confidence: "high", Source: "platform"},
			},
			Edges: []TopologyEdgeInput{
				// The Pod's owning Deployment is "bad", NOT "unrelated".
				{ID: 312, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "bad-xyz", UID: rsUID},
					Target: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "bad-abc", UID: podUID}},
				{ID: 313, ClusterID: fixtureClusterA, Kind: "Owns",
					Source: ResourceCitation{Kind: "Deployment", Namespace: "app", Name: "bad", UID: deployUID},
					Target: ResourceCitation{Kind: "ReplicaSet", Namespace: "app", Name: "bad-xyz", UID: rsUID}},
			},
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceContradicted,
		ExpectRuleID:        "correlation.rollout_causes_pod_failure.v1",
		ExpectCandidates:    1,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
	}
}

// coldStartFixture is an additional scenario: a pod-failure signal with no
// change event in the window. The engine produces a cold-start case with
// ConfidenceUnknown so M43 can disclose uncertainty.
func coldStartFixture() GoldenFixture {
	observedAt := fixtureClock
	podUID := "pod-uid-coldstart-010"

	return GoldenFixture{
		Name:        "cold_start_no_change",
		Description: "pod failure with no change in window — unknown confidence cold start",
		Inputs: EngineInputs{
			Now: fixtureClock,
			Signals: []SignalOccurrenceInput{
				{ID: 110, SignalID: "diag.pod.image_pull_backoff.v1", Producer: "diagnosis",
					ClusterID: fixtureClusterA, Namespace: "app",
					Resource: ResourceCitation{Kind: "Pod", Namespace: "app", Name: "cold-abc", UID: podUID},
					Severity: "critical", State: "active", Coverage: "complete",
					Freshness: observedAt, ObservedAt: observedAt},
			},
			// No changes, edges or diagnoses.
		},
		ExpectResults:       1,
		ExpectConfidence:    ConfidenceUnknown,
		ExpectRuleID:        "correlation.rollout_causes_pod_failure.v1",
		ExpectCandidates:    0,
		ExpectSignalLinks:   1,
		ExpectResourceLinks: 1,
		ExpectColdStart:     true,
	}
}
