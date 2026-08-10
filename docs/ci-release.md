# CI And Release Operations

> **安全重写说明**：Git 历史已执行安全脱敏重写，本文档中引用的旧 commit hash（如 `0baf8583956e1e987ef5043b5fd70ce33aba90e4`）已失效，仅供历史归档参考。CI run 编号不变，但关联的 revision hash 已变更。如需定位对应版本，请使用 tag 名称或按提交信息搜索。

## Workflow Inventory

| Workflow | Trigger | Runner | Result |
|---|---|---|---|
| `CI` | push to `main`, pull request, manual, reusable call | GitHub-hosted Ubuntu 24.04 | backend/frontend/manifests, race detector, golangci-lint, ESLint, 50% coverage baseline, `oasdiff` breaking-change check, offline identity/recovery readiness, isolated audit signing, credential re-encryption, PostgreSQL recovery and Compose runtime gates plus seven-day sanitized evidence |
| `Release` | semantic RC tag or manual rehearsal | GitHub-hosted Ubuntu 24.04 | multi-architecture (`linux/amd64` + `linux/arm64`) OCI images, image SPDX SBOM, Helm/Kustomize/offline assets, release manifest, provenance, signed SHA256 root and GitHub prerelease |
| `Real kind E2E` | Saturday 18:17 UTC schedule or manual suite choice | self-hosted Windows `aiops-kind` | disposable diagnosis/fleet/search and M23-M31 release/backup/governance/maintenance/restore evidence retained for 14 days |

All referenced marketplace actions are pinned to full commit SHAs. Dependabot
groups only minor and patch updates; major updates are emitted as separate PRs
for explicit review. Review the upstream tag and diff before replacing a SHA.
`pull_request_target` is prohibited.

Activation status: private remote `guiyi-labs/aiops-platform` is configured.
Hosted CI run `30348664880` passed all four jobs at revision
`0baf8583956e1e987ef5043b5fd70ce33aba90e4`, including offline identity and
recovery readiness, signed audit archival, credential re-encryption,
PostgreSQL recovery and independent Compose runtime.
The pnpm setup action runs on Node 24 without the prior deprecation warning.
Required branch checks and the dedicated `aiops-kind` runner are not enabled.
M38 added the race/lint/coverage/oasdiff checks and the M23-M31 real-kind
matrix to the workflow contracts; they take effect on the next hosted run
after the M38 revision is pushed.

## Required Branch Protection

When the repository plan permits it, protect `main` and require these checks:

- `Backend`
- `Frontend`
- `Manifests`
- `Compose runtime`
- `Race` (M38A)
- `Lint` (golangci-lint + ESLint, M38A)
- `Coverage baseline` (M38A)
- `OpenAPI breaking-change` (oasdiff, M38A)

Require pull-request review, dismiss stale approvals and block force pushes.
The current private-repository account returned HTTP 403 for this API, stating
that GitHub Pro or a public repository is required. Keep the repository private
until publication is explicitly approved; do not weaken the workflow to work
around the missing protection.
Do not add repository or environment secrets to the `CI` workflow. The Compose
job first runs signed-audit, credential-key and PostgreSQL recovery drills plus
network-disabled identity/recovery readiness gates with random ephemeral
process material, then generates a private `.env` for the separate Compose
runtime. It uploads only sanitized counts/hashes/status and service logs, never
identity/recovery inputs, archive payloads, manifests or keys, and removes the
runtime, volume and `.env` in `always()` steps.

## Local Release Verification (M97)

Before asking CI to create a tagged release, verify the package on the host
that holds the checked-out HEAD:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-verify.ps1 -Version v0.3.0-rc.1 -IncludeImages -StrictSupplyChain -StrictSignatures
```

The script requires a clean revision, asserts the semantic RC version,
assembles source/contract/license/Helm/Kustomize/offline assets and image SPDX
SBOMs, writes `release-manifest.json` plus `SHA256SUMS`, and verifies every
manifest entry. Strict mode requires both OCI archives and both Linux
platforms. Local key signing uses `COSIGN_PRIVATE_KEY` and
`COSIGN_PUBLIC_KEY`; without those inputs the directory is explicitly a
rehearsal package rather than a publishable RC.

The release workflow is fail-closed: it generates provenance before the final
checksum root, signs that root with keyless Cosign, verifies the exact workflow
identity (the tag ref for tag pushes, or the branch ref for manual package
rehearsals) and runs the strict manifest verifier before upload or publication.
It never rewrites `SHA256SUMS` after signing.
## Release Procedure

1. Confirm the working branch is merged and all required checks passed.
2. Run the local `scripts/verify.ps1` gate and review the sanitized artifact.
3. Create an annotated semantic RC tag such as `v0.3.0-rc.1` from the reviewed
   revision and push only that tag.
4. The release workflow reruns the complete reusable CI before packaging.
5. Download the workflow artifact and verify `sha256sum -c SHA256SUMS` on a
   trusted Linux host.
6. For tag runs, verify the GitHub prerelease points at the same revision
   recorded in `release-manifest.json`.
7. Run `scripts/m97-release-rehearsal.ps1` against a new kind cluster and retain
   only its redacted JSON evidence.

A manual `workflow_dispatch` with a semantic RC version is a package rehearsal;
it cannot call `gh release create`. Release packages contain:

- versioned backend and frontend OCI image archives for `linux/amd64` and
  `linux/arm64` (M38C multi-architecture build via `docker buildx`/QEMU);
- SPDX SBOM generated from each OCI archive by `syft v1.27.0`;
- the official Helm 3 chart bundle from `deploy/helm/aiops-platform/`
  (M38B);
- the Kustomize baseline and an offline bundle containing images, deployment
  assets, the Secret template, SBOMs, runbook and `OFFLINE-SHA256SUMS`;
- the enforceable license allowlist at
  `docs/security/license-allowlist.json` (M38C);
- a source archive from the exact Git revision;
- the OpenAPI contract and production dependency-license inventory;
- `release-manifest.json`, in-toto provenance, revision metadata and the
  signed SHA-256 checksum root.

The release workflow signs the checksum root with its keyless identity and the
strict manifest verifier checks the signing bundle before upload. The license allowlist
admits `MIT`/`ISC`/`BSD-2-Clause`/`BSD-3-Clause`/`Apache-2.0` only and is
enforced by `backend/internal/deployment/license_allowlist_test.go`.

The current phase does not publish a container registry tag. The GitHub
Release is a prerelease containing OCI archives; operators must mirror them to
an approved registry or use an environment-specific OCI import path. M89/M90
remain outside the RC verification boundary.

## Helm Chart

M38B ships the official Helm 3 chart at `deploy/helm/aiops-platform/`. The
chart is the supported path for parameterized deployment and runs in parallel
with the Kustomize baseline. It contains:

- `Chart.yaml`, `values.yaml`, `values.schema.json` and `.helmignore`;
- templates for namespace, service accounts, ConfigMap, PostgreSQL
  StatefulSet, backend/frontend Deployments, Services, Ingress, NetworkPolicy
  and PDB;
- ten Go contract tests at
  `backend/internal/deployment/helm_chart_test.go` guarding chart metadata,
  values, schema, required templates, security baseline and the
  "never render a Secret" rule.

The chart **never** renders a Secret. Operators must provide the
`aiops-secrets` Secret externally (see `deploy/kubernetes/secret.example.yaml`
for the schema) before installing the chart:

```powershell
kubectl apply -f /secure/path/aiops-secret.yaml
helm install aiops deploy/helm/aiops-platform -n aiops-system --create-namespace
```

## Real-Kind Runner

Register a non-production self-hosted runner with labels `windows`, `x64` and
`aiops-kind`. It must provide:

- Windows PowerShell 5.1 and Docker Desktop using Linux containers;
- `kubectl` and repository `.tools\kind-v0.30.0.exe` access;
- capacity for two concurrent single-node kind control planes;
- no production kubeconfig, cloud credential or retained platform database.

The workflow concurrency group never cancels an active suite. Each underlying
script verifies that the initial kind cluster set is restored and removes its
isolated platform, containers, network, image and credential files in
`finally`. If the runner host is forcibly terminated, an administrator must
inspect `aiops-*-e2e-*` resources before re-enabling the runner.

Metrics/demo E2E is intentionally excluded because it operates on the retained
demo path and may require a local administrator credential. It remains a
supervised local acceptance step.

## Local Contract Validation

The normal Go suite parses all workflow and Dependabot YAML and verifies the
security markers in `backend/internal/deployment/ci_workflows_test.go`.
Before changing workflow expressions, also run actionlint 1.7.7 or newer:

```powershell
actionlint -color
```

The full local gate remains:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```
