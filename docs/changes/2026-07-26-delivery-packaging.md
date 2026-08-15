# Change Record: M5 delivery packaging

- Date: 2026-07-26
- Scope: One-command verification, real kind E2E, architecture assets and delivery archive
- Result: Passed

## Delivered

- Added `scripts/verify.ps1` for backend vet/test/build, frontend typecheck/test/build, Compose build/runtime health, three Kustomize render gates and backend/frontend/proxy HTTP checks.
- Added `scripts/e2e-kind.ps1` for repeatable real-cluster validation with an in-memory one-hour ServiceAccount kubeconfig, three deterministic diagnoses, confirmed/idempotent rollout restart, RBAC denial checks and guaranteed platform-cluster cleanup.
- Added `scripts/generate-license-report.ps1`; it inventories modules reachable from the Go server binary and the pnpm production dependency graph, then regenerates `docs/supply-chain/dependency-licenses.md`.
- Added architecture/use-case/ER/sequence diagrams, test matrix and environment record (detailed materials retained locally).
- Added a deployment contract test that keeps all required delivery assets in the normal Go suite.
- Ignored `.artifacts/`, which stores only machine-readable, sanitized local evidence.

## Compatibility fixes found by the delivery gate

The repository declares Go 1.25 while the ignored local toolchain is Go 1.24.4. The quality gate now checks the host version and falls back to the cached `golang:1.25-alpine` image. Containerized tests mount the full repository read-only because deployment and OpenAPI contract tests intentionally read `docs`, `deploy` and `scripts` outside the backend build context. The production image context remains limited to `backend`.

Windows PowerShell 5 turns redirected native stderr into error records even when a command exits successfully. The license and kind scripts now judge native commands by exit code, so normal Docker/kubectl progress and warnings are not treated as failures. Real fixture dry-runs use the same client-side apply manager as actual fixture updates, avoiding a field-manager conflict left by prior applies.

The kind kubeconfig exposes its API server on a host loopback port. That address is not reachable as loopback from the Compose backend container. The E2E script now maps the connection address to `host.docker.internal` while retaining the original host through the standard kubeconfig `tls-server-name` field. The backend parser gained tested `tls-server-name` support; CA verification remains enabled.

## Final quality gate

`scripts/verify.ps1` completed at 2026-07-26 16:47:13 +08:00 in 135.98 seconds:

- Go 1.25 container: `go vet ./...`, all backend packages and server build passed.
- Frontend: typecheck, 8 Vitest files / 26 tests and Vite production build passed.
- Compose: PostgreSQL, backend and frontend all reached `healthy`.
- HTTP: backend ready, frontend HTML 200 and frontend API proxy ready passed.
- Kustomize: platform 16 resources, managed-cluster RBAC 5 resources and demo scenarios 7 resources rendered.
- Evidence: `.artifacts/verification/verify-20260726-164713.json`.

## Real kind E2E

`scripts/e2e-kind.ps1` completed at 2026-07-26 16:44:33 +08:00 against `kind-aiops-test`, Kubernetes v1.34.0:

- Two healthy workloads were Ready; the fault Pods reached ImagePullBackOff and CrashLoopBackOff.
- The platform read 5 Pods and 2 Services and matched `pod.image_pull_backoff.v1`, `pod.crash_loop_backoff.v1` and `service.no_ready_endpoints.v1`.
- Remediation plan `c5aa02a1-e1fe-4ca8-ad6b-277199808c59` succeeded; same-key replay returned the same plan.
- The ServiceAccount could list Pods and patch Deployments in `aiops-demo`, but could not delete Pods or patch `kube-system` Deployments.
- The temporary platform cluster and cascade-owned diagnoses/remediation rows were deleted. Post-run counts were zero.
- Evidence: `.artifacts/e2e-kind/e2e-kind-20260726-164433.json`.

## Security and cleanup

A source/document scan excluding ignored tools, artifacts, dependencies and build output found no private key, long embedded token, certificate-authority payload or JWT bearer material. Evidence JSON contains only status, counts, rule IDs, resource names, plan IDs and RBAC results. It does not contain kubeconfig, CA data, ServiceAccount token, access token, password or Cookie.

The Compose application remains available at `http://localhost:18080`; backend is at `http://localhost:8080`. The local kind cluster and demo resources remain for inspection. No initial Git commit was created because the repository baseline still requires explicit human confirmation.
