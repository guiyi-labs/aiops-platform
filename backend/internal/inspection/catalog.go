package inspection

import "time"

// DefaultCatalog is the compile-time rule catalog for M52. 8 KubeEye-style rules
// covering node health, workload lifecycle, storage and network ingress. The catalog is
// append-only; new rule codes are added at the end and never renumbered.
// Each rule maps 1:1 into an M39 signal_code via the descriptor so findings can be
// correlated, ranked and routed alongside diagnosis / alert / metric signals.
func DefaultCatalog() []RuleDescriptor {
	return []RuleDescriptor{
		{
			Code:            "node_not_ready",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainNode,
			DefaultSeverity: SeverityCritical,
			SignalCode:      "inspect.node.not_ready.v1",
			Description:     "Node Ready condition is False or Unknown for >=5m.",
			Remediation:     "Drain the node, check kubelet, container runtime, network connectivity and node pressure conditions.",
			Timeout:         10 * time.Second,
		},
		{
			Code:            "pod_restart_loop",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainWorkload,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.workload.pod_restart_loop.v1",
			Description:     "Pod has restarted >=5 times within the last hour.",
			Remediation:     "Check pod events, container logs, OOMKilled / CrashLoopBackOff status and the pod spec.",
			Timeout:         15 * time.Second,
		},
		{
			Code:            "pvc_pending",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainStorage,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.storage.pvc_pending.v1",
			Description:     "PersistentVolumeClaim is in Pending phase for >=10m.",
			Remediation:     "Verify StorageClass exists, provisioner is healthy, PV capacity is available, access modes match.",
			Timeout:         10 * time.Second,
		},
		{
			Code:            "pod_pending",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainWorkload,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.workload.pod_pending.v1",
			Description:     "Pod is in Pending phase for >=10m (scheduling / image / init failures).",
			Remediation:     "Check pod events for scheduler failures, image pull errors, init-container errors and PVC bindings.",
			Timeout:         15 * time.Second,
		},
		{
			Code:            "workload_replicas_unavailable",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainWorkload,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.workload.replicas_unavailable.v1",
			Description:     "Deployment/StatefulSet/DaemonSet has unavailable replicas for >=5m.",
			Remediation:     "Inspect pod status, events, readiness probes, rolling update strategy and resource quotas.",
			Timeout:         15 * time.Second,
		},
		{
			Code:            "node_pressure",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainNode,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.node.pressure.v1",
			Description:     "Node has MemoryPressure / DiskPressure / PIDPressure == True for >=5m.",
			Remediation:     "Identify offending workloads, check eviction thresholds, expand capacity or add nodes.",
			Timeout:         10 * time.Second,
		},
		{
			Code:            "container_oom_killed",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainWorkload,
			DefaultSeverity: SeverityCritical,
			SignalCode:      "inspect.workload.oom_killed.v1",
			Description:     "A container in a running Pod was OOMKilled in the last 24h.",
			Remediation:     "Review container memory limits vs actual working set; raise limits or fix memory leak.",
			Timeout:         15 * time.Second,
		},
		{
			Code:            "ingress_backend_unhealthy",
			SchemaVersion:   "1.0",
			Domain:          RuleDomainNetwork,
			DefaultSeverity: SeverityWarning,
			SignalCode:      "inspect.network.ingress_backend.v1",
			Description:     "Ingress backend Service has zero ready endpoints (all Pods not Ready).",
			Remediation:     "Check Service selector, Pod readiness probes, endpoint slices and ingress controller.",
			Timeout:         15 * time.Second,
		},
	}
}

// CatalogByCode returns a map from rule_code -> descriptor for O(1) lookups in the
// hot execution path.
func CatalogByCode(catalog []RuleDescriptor) map[string]RuleDescriptor {
	m := make(map[string]RuleDescriptor, len(catalog))
	for _, r := range catalog {
		m[r.Code] = r
	}
	return m
}

// RuleCodes returns the catalog rule codes in registration order (used by
// the M82 analyzer discovery contract).
func RuleCodes(catalog []RuleDescriptor) []string {
	out := make([]string, 0, len(catalog))
	for _, r := range catalog {
		out = append(out, r.Code)
	}
	return out
}

// IsValidRuleCode reports whether code is in the catalog.
func IsValidRuleCode(catalog map[string]RuleDescriptor, code string) bool {
	_, ok := catalog[code]
	return ok
}
