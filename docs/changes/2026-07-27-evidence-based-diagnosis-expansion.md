# M18 Evidence-Based Diagnosis Expansion

- Status: Accepted
- Date: 2026-07-27
- Scope: deterministic current-state diagnosis, replayable fixtures, fixed API/UI actions and real kind evidence

## Outcome

M18 adds four versioned rules without widening the Kubernetes mutation boundary:

| Rule | Required current-state evidence |
|---|---|
| `node.pressure.v1` | Node is Ready and MemoryPressure, DiskPressure or PIDPressure is True |
| `persistentvolumeclaim.pending.v1` | PVC phase is Pending and at least one Warning Event is linked by the exact PVC UID |
| `horizontalpodautoscaler.saturated.v1` | current or desired replicas reached maxReplicas and ScalingLimited=True/TooManyReplicas |
| `ingress.backend_unavailable.v1` | a non-ExternalName Service backend has zero Ready EndpointSlice/Endpoints addresses |

Node NotReady remains authoritative before pressure evaluation. PVC Pending alone does not match, so WaitForFirstConsumer without a Warning Event remains normal. HPA TooFewReplicas is a minimum-bound condition and does not match. Ingress backend Service reads are deduplicated; any Service or endpoint collection error fails the whole diagnosis instead of emitting a partial conclusion.

## Replay And Product Surface

`backend/internal/diagnosis/testdata/m18-fixtures.json` contains positive and negative snapshots for all four rules. The fixture test asserts rule versions, non-empty evidence and a stable observation time. Kubernetes Event lookup now exposes a generic exact-UID reader reused by Pod and PVC diagnosis.

The fixed diagnosis endpoint accepts canonical `Ingress`, `PersistentVolumeClaim` and `HorizontalPodAutoscaler` kinds, with `PVC` and `HPA` retained as input aliases. OpenAPI and API documentation list the same contract. The resource workbench exposes icon actions only when the visible current state can plausibly satisfy each rule: Node NotReady/pressure, Ingress Service routes, PVC Pending and HPA saturation.

## Real Kind Evidence

`deploy/demo-scenarios/m18-diagnosis-resources.yaml` adds a missing-StorageClass PVC, an Ingress pointing to the existing zero-endpoint Service, and an HPA used for status-subresource injection. `m18-pressure-node.yaml` is intentionally excluded from Kustomize and is created only during E2E; `finally` always deletes it.

The retained Kubernetes v1.34.0 run passed on 2026-07-27 with evidence at `.artifacts/e2e-kind/e2e-kind-20260727-165019.json`. It persisted seven expected rule IDs, including all four M18 rules, preserved Metrics Server samples, passed remediation execution/idempotent replay and retained platform cluster ID 39 (`demo-kind-20260727-165016`). The temporary Node was absent after the run.

Browser acceptance used the real retained cluster. Ingress and PVC list actions were present; an Ingress action opened the persisted `ingress.backend_unavailable.v1` evidence. A synthetic HPA saturation window exposed the HPA action; once the controller replaced that synthetic status, the action result correctly became no-match because diagnosis reads current state. Desktop layout was coherent, and a 390x844 viewport reported body/dialog widths of 375/375 with no horizontal overflow or browser errors.

## Explicit Limitation

M18 does not implement a sustained restart rule. The public Pod model and diagnosis source contain one current snapshot plus a cumulative restart count. They do not retain a sampling window, two comparable snapshots or missing-sample semantics. A cumulative count cannot prove that restarts are still sustained, so implementing that rule now would invent temporal evidence. It remains deferred until bounded time-series sampling is designed and tested.

## Verification

- Related Go packages: passed.
- Frontend typecheck: passed.
- Vitest: 12 files / 55 tests passed.
- Kustomize client and server dry-run: passed for the 22-object demo render.
- Real kind E2E: passed, `.artifacts/e2e-kind/e2e-kind-20260727-165019.json`.
- Final full gate: passed in 105.17 seconds with 123 Go `Test*` entries, 12 Vitest files / 55 tests, Docker image builds, three healthy Compose services, Kustomize `16/5/22/3` and runtime API/proxy checks. Evidence: `.artifacts/verification/verify-20260727-172323.json`.
