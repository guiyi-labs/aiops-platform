package inspection

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// DefaultExecutor implements RuleExecutor using the read-only k8sgateway.Service.
// Each rule maps to a concrete gateway call (Nodes, Pods, PVCs, Deployments, etc.).
type DefaultExecutor struct {
	gateway *k8sgateway.Service
}

// NewDefaultExecutor wires the read-only Kubernetes gateway.
func NewDefaultExecutor(gateway *k8sgateway.Service) *DefaultExecutor {
	return &DefaultExecutor{gateway: gateway}
}

// Execute dispatches to the concrete rule function by rule.Code. Unknown codes
// return ErrInvalidRuleCode; transient gateway errors are propagated unchanged so
// the task loop can record them without terminating other rules.
func (e *DefaultExecutor) Execute(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	if e == nil || e.gateway == nil {
		return nil, errors.New("kubernetes gateway is not configured")
	}
	ruleCtx, cancel := context.WithTimeout(ctx, rule.Timeout)
	defer cancel()
	switch rule.Code {
	case "node_not_ready":
		return e.ruleNodeNotReady(ruleCtx, clusterID, rule)
	case "node_pressure":
		return e.ruleNodePressure(ruleCtx, clusterID, rule)
	case "pod_restart_loop":
		return e.rulePodRestartLoop(ruleCtx, clusterID, rule)
	case "pod_pending":
		return e.rulePodPending(ruleCtx, clusterID, rule)
	case "container_oom_killed":
		return e.ruleOOMKilled(ruleCtx, clusterID, rule)
	case "workload_replicas_unavailable":
		return e.ruleWorkloadReplicas(ruleCtx, clusterID, rule)
	case "pvc_pending":
		return e.rulePVCPending(ruleCtx, clusterID, rule)
	case "ingress_backend_unhealthy":
		return e.ruleIngressBackend(ruleCtx, clusterID, rule)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidRuleCode, rule.Code)
	}
}

// ruleNodeNotReady: node Ready condition is False or Unknown.
func (e *DefaultExecutor) ruleNodeNotReady(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.Nodes(ctx, clusterID, apiquery.ListQuery{Page: 1, Limit: 500})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	for _, n := range resp.Items {
		var ready string
		var reason, message string
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" {
				ready = c.Status
				reason = c.Reason
				message = c.Message
				break
			}
		}
		if ready == "True" || ready == "" {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    "",
			ResourceKind: "Node",
			ResourceName: n.Metadata.Name,
			ResourceUID:  n.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"ready_status": ready,
				"reason":       reason,
				"message":      message,
				"conditions":   summarizeNodeConditions(n.Status.Conditions),
			},
		})
	}
	return out, nil
}

// ruleNodePressure: MemoryPressure / DiskPressure / PIDPressure == True.
func (e *DefaultExecutor) ruleNodePressure(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.Nodes(ctx, clusterID, apiquery.ListQuery{Page: 1, Limit: 500})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	for _, n := range resp.Items {
		pressures := []string{}
		for _, c := range n.Status.Conditions {
			if (c.Type == "MemoryPressure" || c.Type == "DiskPressure" || c.Type == "PIDPressure") && c.Status == "True" {
				pressures = append(pressures, fmt.Sprintf("%s=%s (%s)", c.Type, c.Status, c.Reason))
			}
		}
		if len(pressures) == 0 {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			ResourceKind: "Node",
			ResourceName: n.Metadata.Name,
			ResourceUID:  n.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"pressures": pressures,
			},
		})
	}
	return out, nil
}

// rulePodRestartLoop: pod restartCount >= 5 in any container status.
func (e *DefaultExecutor) rulePodRestartLoop(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.Pods(ctx, clusterID, "", apiquery.ListQuery{Page: 1, Limit: 2000})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	for _, p := range resp.Items {
		maxRestarts := int32(0)
		var worstContainer string
		for _, cs := range p.Status.ContainerStatuses {
			if cs.RestartCount > maxRestarts {
				maxRestarts = cs.RestartCount
				worstContainer = cs.Name
			}
		}
		if maxRestarts < 5 {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    p.Metadata.Namespace,
			ResourceKind: "Pod",
			ResourceName: p.Metadata.Name,
			ResourceUID:  p.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"max_restarts":       maxRestarts,
				"worst_container":    worstContainer,
				"container_statuses": redactContainerStatuses(p.Status.ContainerStatuses),
			},
		})
	}
	return out, nil
}

// rulePodPending: Pod phase == Pending for >=10m.
func (e *DefaultExecutor) rulePodPending(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.Pods(ctx, clusterID, "", apiquery.ListQuery{Page: 1, Limit: 2000})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	cutoff := now.Add(-10 * time.Minute)
	for _, p := range resp.Items {
		if p.Status.Phase != "Pending" {
			continue
		}
		created, _ := time.Parse(time.RFC3339, p.Metadata.CreationTimestamp)
		if !created.IsZero() && created.After(cutoff) {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    p.Metadata.Namespace,
			ResourceKind: "Pod",
			ResourceName: p.Metadata.Name,
			ResourceUID:  p.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"phase":      p.Status.Phase,
				"reason":     p.Status.Reason,
				"message":    p.Status.Message,
				"conditions": summarizePodConditions(p.Status.Conditions),
				"created_at": p.Metadata.CreationTimestamp,
			},
		})
	}
	return out, nil
}

// ruleOOMKilled: any container in the pod was terminated with reason OOMKilled in last 24h.
func (e *DefaultExecutor) ruleOOMKilled(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.Pods(ctx, clusterID, "", apiquery.ListQuery{Page: 1, Limit: 2000})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	cutoff := now.Add(-24 * time.Hour)
	for _, p := range resp.Items {
		for _, cs := range p.Status.ContainerStatuses {
			// ContainerStatus.LastState is the prior terminated/waiting state.
			last := cs.LastState.Terminated
			if last == nil {
				continue
			}
			if last.Reason != "OOMKilled" {
				continue
			}
			ts, _ := time.Parse(time.RFC3339, last.FinishedAt)
			if !ts.IsZero() && ts.Before(cutoff) {
				continue
			}
			out = append(out, Finding{
				RuleCode:     rule.Code,
				SignalCode:   rule.SignalCode,
				Namespace:    p.Metadata.Namespace,
				ResourceKind: "Pod",
				ResourceName: p.Metadata.Name,
				ResourceUID:  p.Metadata.UID,
				ObservedAt:   now,
				Evidence: map[string]interface{}{
					"container":          cs.Name,
					"exit_code":          last.ExitCode,
					"finished_at":        last.FinishedAt,
					"restart_count":      cs.RestartCount,
					"memory_limit_bytes": memoryLimitFor(p, cs.Name),
				},
			})
		}
	}
	return out, nil
}

// ruleWorkloadReplicas: Deployment / StatefulSet unavailable_replicas > 0.
func (e *DefaultExecutor) ruleWorkloadReplicas(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	var out []Finding
	now := time.Now().UTC()
	limit := apiquery.ListQuery{Page: 1, Limit: 1000}

	deps, err := e.gateway.Deployments(ctx, clusterID, "", limit)
	if err != nil {
		return nil, err
	}
	for _, d := range deps.Items {
		if d.Status.UnavailableReplicas <= 0 {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    d.Metadata.Namespace,
			ResourceKind: "Deployment",
			ResourceName: d.Metadata.Name,
			ResourceUID:  d.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"spec_replicas":        d.Spec.Replicas,
				"ready_replicas":       d.Status.ReadyReplicas,
				"available_replicas":   d.Status.AvailableReplicas,
				"unavailable_replicas": d.Status.UnavailableReplicas,
			},
		})
	}
	ss, err := e.gateway.StatefulSets(ctx, clusterID, "", limit)
	if err != nil {
		return out, nil // partial ok; caller records error only if nothing succeeded
	}
	for _, s := range ss.Items {
		ready := s.Status.ReadyReplicas
		spec := int32(0)
		if s.Spec.Replicas != nil {
			spec = *s.Spec.Replicas
		}
		unavail := spec - ready
		if unavail <= 0 {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    s.Metadata.Namespace,
			ResourceKind: "StatefulSet",
			ResourceName: s.Metadata.Name,
			ResourceUID:  s.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"spec_replicas":  spec,
				"ready_replicas": ready,
			},
		})
	}
	return out, nil
}

// rulePVCPending: PVC phase == Pending for >=10m.
func (e *DefaultExecutor) rulePVCPending(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	resp, err := e.gateway.PersistentVolumeClaims(ctx, clusterID, "", apiquery.ListQuery{Page: 1, Limit: 1000})
	if err != nil {
		return nil, err
	}
	var out []Finding
	now := time.Now().UTC()
	cutoff := now.Add(-10 * time.Minute)
	for _, p := range resp.Items {
		if p.Status.Phase != "Pending" {
			continue
		}
		created, _ := time.Parse(time.RFC3339, p.Metadata.CreationTimestamp)
		if !created.IsZero() && created.After(cutoff) {
			continue
		}
		out = append(out, Finding{
			RuleCode:     rule.Code,
			SignalCode:   rule.SignalCode,
			Namespace:    p.Metadata.Namespace,
			ResourceKind: "PersistentVolumeClaim",
			ResourceName: p.Metadata.Name,
			ResourceUID:  p.Metadata.UID,
			ObservedAt:   now,
			Evidence: map[string]interface{}{
				"phase":         p.Status.Phase,
				"storage_class": p.Spec.StorageClassName,
				"access_modes":  p.Spec.AccessModes,
				"requests":      p.Spec.Resources.Requests,
				"created_at":    p.Metadata.CreationTimestamp,
			},
		})
	}
	return out, nil
}

// ruleIngressBackend: ingress backend Services with zero ready endpoints.
// The Kubernetes gateway only exposes per-service endpoints (ServiceEndpoints)
// not a bulk list, so we fan out per backend. The fan-out is bounded by the
// ingress count (typically <500) and we swallow per-service errors with a
// partial flag in evidence.
func (e *DefaultExecutor) ruleIngressBackend(ctx context.Context, clusterID int64, rule RuleDescriptor) ([]Finding, error) {
	ingressResp, err := e.gateway.Ingresses(ctx, clusterID, "", apiquery.ListQuery{Page: 1, Limit: 500})
	if err != nil {
		return nil, err
	}

	type backendKey struct {
		Namespace string
		Name      string
	}
	// Deduplicate backends so we don't repeatedly hit the same service across
	// ingresses or paths.
	seen := make(map[backendKey]int) // 0 = not queried, >0 ready, -1 error
	countReadyAddrs := func(ns, name string) int {
		key := backendKey{Namespace: ns, Name: name}
		if v, ok := seen[key]; ok {
			return v
		}
		ep, epErr := e.gateway.ServiceEndpoints(ctx, clusterID, ns, name)
		if epErr != nil {
			seen[key] = -1
			return -1
		}
		count := 0
		for _, ss := range ep.Subsets {
			count += len(ss.Addresses)
		}
		seen[key] = count
		return count
	}

	var out []Finding
	now := time.Now().UTC()
	for _, ing := range ingressResp.Items {
		// Rename loop variable to ingressRule to avoid shadowing the outer
		// `rule RuleDescriptor` parameter.
		for _, ingressRule := range ing.Spec.Rules {
			if ingressRule.HTTP == nil {
				continue
			}
			for _, path := range ingressRule.HTTP.Paths {
				// Note: path.Backend is a value type (IngressBackend struct),
				// only the nested .Service pointer can be nil.
				if path.Backend.Service == nil {
					continue
				}
				svc := path.Backend.Service
				ns := ing.Metadata.Namespace
				ready := countReadyAddrs(ns, svc.Name)
				if ready > 0 {
					continue
				}
				evidence := map[string]interface{}{
					"host":            ingressRule.Host,
					"path":            path.Path,
					"service_name":    svc.Name,
					"service_port":    svc.Port,
					"ready_endpoints": max(0, ready),
				}
				if ready < 0 {
					evidence["endpoints_query_error"] = true
				}
				out = append(out, Finding{
					RuleCode:     rule.Code,
					SignalCode:   rule.SignalCode,
					Namespace:    ns,
					ResourceKind: "Ingress",
					ResourceName: ing.Metadata.Name,
					ResourceUID:  ing.Metadata.UID,
					ObservedAt:   now,
					Evidence:     evidence,
				})
			}
		}
	}
	return out, nil
}

// --- evidence helpers ---

// summarizeNodeConditions reads Node.Status.Conditions via reflection. The
// gateway embeds the conditions as an anonymous struct slice so no named type
// exists. We read the exported field-by-name (Type / Status / Reason /
// Message / LastTransitionTime) and keep only non-empty string fields, with
// a size cap per field to bound evidence payloads.
func summarizeNodeConditions(conds any) []map[string]string {
	out := []map[string]string{}
	v := reflect.ValueOf(conds)
	if !v.IsValid() || v.Kind() != reflect.Slice {
		return out
	}
	keys := []string{"Type", "Status", "Reason", "Message", "LastTransitionTime"}
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			continue
		}
		m := map[string]string{}
		for _, k := range keys {
			f := elem.FieldByName(k)
			if !f.IsValid() || f.Kind() != reflect.String {
				continue
			}
			s := f.String()
			if s == "" {
				continue
			}
			m[strings.ToLower(k)] = redactLong(s, 200)
		}
		if len(m) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

func summarizePodConditions(conds []k8sgateway.PodCondition) []map[string]string {
	out := make([]map[string]string, 0, len(conds))
	for _, c := range conds {
		if c.Status != "True" {
			out = append(out, map[string]string{
				"type":    c.Type,
				"status":  c.Status,
				"reason":  c.Reason,
				"message": redactLong(c.Message, 200),
			})
		}
	}
	return out
}

func redactContainerStatuses(statuses []k8sgateway.ContainerStatus) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(statuses))
	for _, cs := range statuses {
		item := map[string]interface{}{
			"name":          cs.Name,
			"ready":         cs.Ready,
			"restart_count": cs.RestartCount,
		}
		if cs.State.Terminated != nil {
			item["state_reason"] = cs.State.Terminated.Reason
			item["state_exit_code"] = cs.State.Terminated.ExitCode
		}
		if cs.LastState.Terminated != nil {
			item["last_reason"] = cs.LastState.Terminated.Reason
			item["last_exit_code"] = cs.LastState.Terminated.ExitCode
			item["last_finished_at"] = cs.LastState.Terminated.FinishedAt
		}
		out = append(out, item)
	}
	return out
}

func memoryLimitFor(pod k8sgateway.Pod, containerName string) string {
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName && c.Resources.Limits != nil {
			if mem, ok := c.Resources.Limits["memory"]; ok {
				return mem
			}
		}
	}
	return ""
}

func redactLong(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
