# Repeatable diagnosis demo scenarios

This directory creates one healthy baseline and the three deterministic failure
scenarios used by the graduation-project demonstration:

| Resource | Expected state | Expected diagnosis |
|---|---|---|
| `Deployment/healthy-nginx` | Ready | none |
| `Deployment/image-pull-backoff` | `ImagePullBackOff` or `ErrImagePull` | `pod.image_pull_backoff.v1` |
| `Deployment/crash-loop-backoff` | `CrashLoopBackOff`, exit code 42 | `pod.crash_loop_backoff.v1` |
| `Service/service-without-endpoints` | zero ready EndpointSlice addresses | `service.no_ready_endpoints.v1` |

All resources are isolated in `aiops-demo`. The two healthy Nginx containers use
the unprivileged image and port 8080 so the demo does not create accidental
CrashLoopBackOff noise when Linux capabilities are dropped.

`m17-resources.yaml` adds one representative StatefulSet, DaemonSet,
ReplicaSet, suspended Job, suspended CronJob, HPA, ResourceQuota, LimitRange and
Secret. The controllers do not create additional running Pods: replica-based
fixtures use zero replicas, the DaemonSet requires an absent node label and the
batch resources are suspended. The Secret contains only a public demo payload
used to prove that the platform response retains key names but removes values
and annotations.

## Apply

A server-side dry-run containing a new Namespace does not persist that
Namespace for later objects in the same bundle. Validate and apply in this
order:

```powershell
kubectl apply -f deploy/demo-scenarios/namespace.yaml
kubectl apply --server-side --dry-run=server -k deploy/demo-scenarios
kubectl apply -k deploy/demo-scenarios
kubectl apply --server-side --dry-run=server -k deploy/managed-cluster
kubectl apply -k deploy/managed-cluster
```

Do not alternate server-side and client-side actual apply managers for an
existing Deployment merely to change a named container port. This can retain
the old list item and produce a duplicate named port. The documented workflow
uses server-side dry-run for admission validation and client-side apply for the
actual local demo lifecycle.

Wait until the healthy workloads are ready, then confirm the two Pod failures:

```powershell
kubectl -n aiops-demo rollout status deployment/healthy-nginx --timeout=90s
kubectl -n aiops-demo rollout status deployment/service-without-endpoints --timeout=90s
kubectl -n aiops-demo get pods
kubectl -n aiops-demo get endpointslices
kubectl -n aiops-demo get events --sort-by=.lastTimestamp
```

Create a short-lived token only when importing the cluster. Store the generated
kubeconfig outside the repository and delete it after the test:

```powershell
kubectl -n kube-system create token aiops-platform --duration=1h
```

## Cleanup

```powershell
kubectl delete -k deploy/managed-cluster
kubectl delete -f deploy/demo-scenarios/namespace.yaml
```
