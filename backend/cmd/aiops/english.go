package main

import (
	"k8s-aiops.local/backend/internal/diagnosis"
)

// englishCopy is the English rendering of one diagnosis rule. The compiled-in
// rules carry Chinese copy (the platform's default locale); the CLI renders
// findings in English for a global audience. The rule *logic* stays in
// internal/diagnosis — only the presentation copy lives here.
type englishCopy struct {
	Summary         string
	RootCauses      []string
	Recommendations []string
}

var ruleEnglish = map[string]englishCopy{
	diagnosis.RuleImagePullBackOff: {
		Summary: "Pod cannot pull its container image; containers are in ImagePullBackOff/ErrImagePull.",
		RootCauses: []string{
			"Image name or tag does not exist",
			"Registry network or DNS unreachable from nodes",
			"imagePullSecret missing, expired or not bound to the ServiceAccount",
		},
		Recommendations: []string{
			"Verify the image registry, name and tag on the workload",
			"Check registry DNS resolution and connectivity from a node",
			"Inspect imagePullSecret and its ServiceAccount/Pod references",
			"Fix the configuration and watch for a new image-pull event",
		},
	},
	diagnosis.RuleCrashLoopBackOff: {
		Summary: "Pod containers keep starting and failing; the pod is stuck in CrashLoopBackOff.",
		RootCauses: []string{
			"Application startup command, arguments or configuration cause the process to exit",
			"Dependency service, config file or Secret is unavailable",
			"Health-check misconfiguration or resource limits killing the process",
		},
		Recommendations: []string{
			"Inspect last_termination exit code, reason and finished time",
			"Read the previous container logs for the error before the last exit",
			"Verify ConfigMap, Secret, startup arguments and dependency connectivity",
			"Check resource limits and probes, then watch restart counts and new events",
		},
	},
	diagnosis.RulePodPending: {
		Summary: "Pod is Pending — scheduling or pre-startup dependencies have not completed.",
		RootCauses: []string{
			"Insufficient node resources or unsatisfiable scheduling constraints",
			"NodeSelector, affinity, taints/tolerations or topology constraints block scheduling",
			"Associated PVC, image or other pre-startup dependency is not ready",
		},
		Recommendations: []string{
			"Check the PodScheduled condition and the FailedScheduling event reason",
			"Compare node allocatable resources, taints and the pod's scheduling constraints",
			"Check PVC, StorageClass and other pre-startup dependencies",
		},
	},
	diagnosis.RulePodOOMKilled: {
		Summary: "A pod container was terminated by OOMKilled — memory pressure or limit configuration issue.",
		RootCauses: []string{
			"Container memory limit is lower than actual working set",
			"Memory leak in the application",
			"Node under memory pressure evicting the pod",
		},
		Recommendations: []string{
			"Review memory working set vs limits with metrics history",
			"Investigate application memory behavior for leaks",
			"Check node pressure events and adjust limits or scheduling",
		},
	},
}

// englishFor returns the English copy for a rule ID, falling back to a
// neutral filter-style copy when the rule is not covered by the table yet.
func englishFor(ruleID string) englishCopy {
	if copy, ok := ruleEnglish[ruleID]; ok {
		return copy
	}
	return englishCopy{
		Summary: "Diagnostic rule " + ruleID + " matched this resource.",
	}
}
