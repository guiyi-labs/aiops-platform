# Managed-cluster RBAC example

These manifests are applied to a target cluster, not to the AIOps platform
cluster. They demonstrate the minimum API groups and verbs needed by the
current read/diagnosis surface, cluster-level M28/M30/M31 controlled operations,
plus Deployment restart/scale and CronJob suspend/resume in one namespace.

1. Replace namespace `aiops-demo` in both YAML files with an explicitly approved
   workload namespace.
2. Apply `observer.yaml` once per target cluster.
3. Apply `remediator.namespace.example.yaml` once for each approved namespace.
4. Create a short-lived ServiceAccount token and embed it in the kubeconfig
   imported into the platform. Do not commit that kubeconfig or token.

`kubectl kustomize deploy/managed-cluster` is an offline syntax/rendering gate.
It does not prove that the target cluster accepts the roles; use server-side
dry-run or an isolated real cluster before production application.

The cluster-wide role grants read access plus only these reviewed mutations:
Node `patch`, Pod `eviction` create, Velero Backup/Restore create, quarantine
Namespace/ResourceQuota/NetworkPolicy create. It grants no generic update or
delete, Pod delete, exec, attach, port-forward, Role or RoleBinding mutation.
The namespaced role adds only `get` and `patch` for Deployments and CronJobs.
Kubernetes dry-run requires the same create/patch authorization as execution;
`backend/internal/deployment/managed_cluster_test.go` enforces the mutation
allowlist.
