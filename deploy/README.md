# Deployment Assets

| Directory | Purpose |
|---|---|
| `kind/` | Local development and test cluster definitions |
| `kubernetes/` | Kustomize platform deployment baseline |
| `managed-cluster/` | Least-privilege target-cluster observer/remediator RBAC examples |
| `demo-scenarios/` | Repeatable failure demonstration resources |
| `diagnosis-e2e/` | Disposable Node/Deployment diagnosis fixtures kept outside the defense demo |

The repository root `compose.yaml` runs the complete local development stack.

## Kubernetes baseline

`kubernetes/` contains a single-platform installation baseline with PostgreSQL,
the backend, the frontend, probes, resource limits, non-root security contexts,
a TLS Ingress and default-deny NetworkPolicies. The backend Service is
`ClusterIP`; the frontend is the only public application path, so `/metrics`
remains reachable only from an in-cluster monitoring source allowed by policy.

Create the Secret outside the repository before applying:

```powershell
Copy-Item kubernetes/secret.example.yaml ..\aiops-secret.yaml
# Edit every CHANGE_ME value, then:
kubectl apply -f ..\aiops-secret.yaml
kubectl apply -k kubernetes/
```

The image names default to `k8s-aiops-backend:dev` and
`k8s-aiops-frontend:dev`, suitable for kind after `kind load docker-image`.
Production overlays should replace them with pinned registry digests and
provide the `aiops-tls` Secret. NetworkPolicy egress allows DNS, PostgreSQL,
HTTPS and the conventional Kubernetes API port; narrow external CIDRs and
ports further when the target topology is known.

The manifests under `managed-cluster/` belong on each target cluster and are
therefore excluded from the platform Kustomization. The observer role is
cluster-wide and read-only; each explicitly approved namespace gets a separate
Role that adds only Deployment `get`/`patch` for server-side dry-run and the
controlled rollout restart action.

## Real kind diagnosis demo

See `demo-scenarios/README.md` for the Namespace-first validation/apply order,
repeatable fault workloads, short-lived ServiceAccount credential guidance and
cleanup procedure. The workflow was exercised against Kubernetes v1.34.0 on
2026-07-17; detailed evidence is archived under
`docs/changes/2026-07-17-real-kind-diagnosis-remediation.md`.
