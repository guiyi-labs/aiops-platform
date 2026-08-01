package deprecatedapi

import (
	"fmt"
	"strings"
)

// CatalogEntry describes one deprecated/removed apiVersion+kind, curated from
// the upstream Kubernetes API removal and deprecation announcements
// (https://kubernetes.io/docs/reference/using-api/deprecation-guide/).
//
// The RemovedIn field is the Kubernetes minor version in which the object
// type was removed from the API. A value of 0 means "not yet removed" (still
// deprecated). The check uses these to classify each object as "removed"
// (will break on upgrade to TargetMinor) or "deprecated" (still works).
type CatalogEntry struct {
	Group        string
	Version      string
	Kind         string
	DeprecatedIn int
	RemovedIn    int
	Replacement  string
	Note         string
}

// catalog is the compiled-in set of known deprecations. It is the single
// source of truth for the analyzer and is intentionally read-only.
var catalog = []CatalogEntry{
	// --- Removed in 1.16 ---
	{Group: "apps", Version: "v1beta1", Kind: "Deployment", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta1 Deployment removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta1", Kind: "StatefulSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta1 StatefulSet removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta1", Kind: "DaemonSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta1 DaemonSet removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta1", Kind: "ReplicaSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta1 ReplicaSet removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta2", Kind: "Deployment", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta2 Deployment removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta2", Kind: "StatefulSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta2 StatefulSet removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta2", Kind: "DaemonSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta2 DaemonSet removed in 1.16; migrate to apps/v1."},
	{Group: "apps", Version: "v1beta2", Kind: "ReplicaSet", RemovedIn: 16, Replacement: "apps/v1", Note: "apps/v1beta2 ReplicaSet removed in 1.16; migrate to apps/v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "Deployment", RemovedIn: 16, Replacement: "apps/v1", Note: "extensions/v1beta1 Deployment removed in 1.16; migrate to apps/v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "StatefulSet", RemovedIn: 16, Replacement: "apps/v1", Note: "extensions/v1beta1 StatefulSet removed in 1.16; migrate to apps/v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "DaemonSet", RemovedIn: 16, Replacement: "apps/v1", Note: "extensions/v1beta1 DaemonSet removed in 1.16; migrate to apps/v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "ReplicaSet", RemovedIn: 16, Replacement: "apps/v1", Note: "extensions/v1beta1 ReplicaSet removed in 1.16; migrate to apps/v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "Ingress", RemovedIn: 22, Replacement: "networking.k8s.io/v1", Note: "extensions/v1beta1 Ingress removed in 1.22; migrate to networking.k8s.io/v1."},
	{Group: "scheduling.k8s.io", Version: "v1alpha1", Kind: "PriorityClass", RemovedIn: 16, Replacement: "scheduling.k8s.io/v1", Note: "scheduling.k8s.io/v1alpha1 PriorityClass removed in 1.16."},
	{Group: "scheduling.k8s.io", Version: "v1beta1", Kind: "PriorityClass", RemovedIn: 22, Replacement: "scheduling.k8s.io/v1", Note: "scheduling.k8s.io/v1beta1 PriorityClass removed in 1.22."},

	// --- Removed in 1.22 ---
	{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "MutatingWebhookConfiguration", RemovedIn: 22, Replacement: "admissionregistration.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "ValidatingWebhookConfiguration", RemovedIn: 22, Replacement: "admissionregistration.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "apiextensions.k8s.io", Version: "v1beta1", Kind: "CustomResourceDefinition", RemovedIn: 22, Replacement: "apiextensions.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "apiregistration.k8s.io", Version: "v1beta1", Kind: "APIService", RemovedIn: 22, Replacement: "apiregistration.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "authentication.k8s.io", Version: "v1beta1", Kind: "TokenReview", RemovedIn: 22, Replacement: "authentication.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "authorization.k8s.io", Version: "v1beta1", Kind: "SubjectAccessReview", RemovedIn: 22, Replacement: "authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "authorization.k8s.io", Version: "v1beta1", Kind: "LocalSubjectAccessReview", RemovedIn: 22, Replacement: "authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "authorization.k8s.io", Version: "v1beta1", Kind: "SelfSubjectAccessReview", RemovedIn: 22, Replacement: "authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "autoscaling", Version: "v2beta1", Kind: "HorizontalPodAutoscaler", RemovedIn: 22, Replacement: "autoscaling/v2", Note: "Removed in 1.22; migrate to autoscaling/v2."},
	{Group: "autoscaling", Version: "v2beta2", Kind: "HorizontalPodAutoscaler", RemovedIn: 22, Replacement: "autoscaling/v2", Note: "Removed in 1.22; migrate to autoscaling/v2."},
	{Group: "certificates.k8s.io", Version: "v1beta1", Kind: "CertificateSigningRequest", RemovedIn: 22, Replacement: "certificates.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "coordination.k8s.io", Version: "v1alpha1", Kind: "Lease", RemovedIn: 22, Replacement: "coordination.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "coordination.k8s.io", Version: "v1beta1", Kind: "Lease", RemovedIn: 22, Replacement: "coordination.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "extensions", Version: "v1beta1", Kind: "NetworkPolicy", RemovedIn: 22, Replacement: "networking.k8s.io/v1", Note: "Removed in 1.22; migrate to networking.k8s.io/v1."},
	{Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress", RemovedIn: 22, Replacement: "networking.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "node.k8s.io", Version: "v1alpha1", Kind: "RuntimeClass", RemovedIn: 22, Replacement: "node.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1alpha1", Kind: "Role", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1alpha1", Kind: "ClusterRole", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1alpha1", Kind: "RoleBinding", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1alpha1", Kind: "ClusterRoleBinding", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "Role", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "ClusterRole", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "RoleBinding", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "rbac.authorization.k8s.io", Version: "v1beta1", Kind: "ClusterRoleBinding", RemovedIn: 22, Replacement: "rbac.authorization.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "storage.k8s.io", Version: "v1beta1", Kind: "CSIDriver", RemovedIn: 22, Replacement: "storage.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "storage.k8s.io", Version: "v1beta1", Kind: "CSINode", RemovedIn: 22, Replacement: "storage.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "storage.k8s.io", Version: "v1beta1", Kind: "StorageClass", RemovedIn: 22, Replacement: "storage.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},
	{Group: "storage.k8s.io", Version: "v1beta1", Kind: "VolumeAttachment", RemovedIn: 22, Replacement: "storage.k8s.io/v1", Note: "Removed in 1.22; migrate to v1."},

	// --- Removed in 1.25 ---
	{Group: "batch", Version: "v1beta1", Kind: "CronJob", RemovedIn: 25, Replacement: "batch/v1", Note: "Removed in 1.25; migrate to batch/v1."},
	{Group: "batch", Version: "v2alpha1", Kind: "CronJob", RemovedIn: 25, Replacement: "batch/v1", Note: "Removed in 1.25; migrate to batch/v1."},
	{Group: "discovery.k8s.io", Version: "v1beta1", Kind: "EndpointSlice", RemovedIn: 25, Replacement: "discovery.k8s.io/v1", Note: "Removed in 1.25; migrate to v1."},
	{Group: "events.k8s.io", Version: "v1beta1", Kind: "Event", RemovedIn: 25, Replacement: "events.k8s.io/v1", Note: "Removed in 1.25; migrate to v1."},
	{Group: "policy", Version: "v1beta1", Kind: "PodSecurityPolicy", RemovedIn: 25, Replacement: "", Note: "Removed in 1.25 with NO direct replacement; adopt Pod Security Admission."},

	// --- Removed in 1.29 ---
	{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta1", Kind: "FlowSchema", RemovedIn: 29, Replacement: "flowcontrol.apiserver.k8s.io/v1beta3", Note: "Removed in 1.29; migrate to v1beta3/v1."},
	{Group: "flowcontrol.apiserver.k8s.io", Version: "v1beta1", Kind: "PriorityLevelConfiguration", RemovedIn: 29, Replacement: "flowcontrol.apiserver.k8s.io/v1beta3", Note: "Removed in 1.29; migrate to v1beta3/v1."},
}

// catalogIndex maps "group|version|kind" to its entry for O(1) lookup.
var catalogIndex = func() map[string]CatalogEntry {
	idx := make(map[string]CatalogEntry, len(catalog))
	for _, e := range catalog {
		idx[key(e.Group, e.Version, e.Kind)] = e
	}
	return idx
}()

func key(group, version, kind string) string {
	return group + "|" + version + "|" + kind
}

// parseAPIVersion splits an apiVersion string into group and version. A bare
// "v1" yields an empty group (core API); "apps/v1" yields group "apps".
func parseAPIVersion(apiVersion string) (group, version string) {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

// minorVersion extracts the minor version integer from a version string such
// as "v1.22" or "1.22". Returns 0 if it cannot be parsed.
func minorVersion(v string) int {
	v = strings.TrimPrefix(v, "v")
	var major, minor int
	if _, err := fmt.Sscanf(v, "%d.%d", &major, &minor); err != nil {
		return 0
	}
	return minor
}

// Lookup resolves a resource's apiVersion+kind against the catalog.
func Lookup(apiVersion, kind string) (CatalogEntry, bool) {
	group, version := parseAPIVersion(apiVersion)
	e, ok := catalogIndex[key(group, version, kind)]
	return e, ok
}
