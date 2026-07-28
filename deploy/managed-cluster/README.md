# Managed-cluster RBAC example

These manifests are applied to a target cluster, not to the AIOps platform
cluster. They demonstrate the minimum API groups and verbs needed by the
current read/diagnosis surface plus controlled Deployment restart/scale and
CronJob suspend/resume in one namespace.

1. Replace namespace `aiops-demo` in both YAML files with an explicitly approved
   workload namespace.
2. Apply `observer.yaml` once per target cluster.
3. Apply `remediator.namespace.example.yaml` once for each approved namespace.
4. Create a short-lived ServiceAccount token and embed it in the kubeconfig
   imported into the platform. Do not commit that kubeconfig or token.

`kubectl kustomize deploy/managed-cluster` is an offline syntax/rendering gate.
It does not prove that the target cluster accepts the roles; use server-side
dry-run or an isolated real cluster before production application.

The cluster-wide role has only `get`, `list` and Pod-log access. The namespaced
role adds only `get` and `patch` for Deployments and CronJobs. It grants no create, delete,
Secret, exec, attach, port-forward, Role or RoleBinding permission. Kubernetes
dry-run still requires the same `patch` authorization as execution.
