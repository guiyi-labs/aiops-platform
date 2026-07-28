# CI And Release Operations

## Workflow Inventory

| Workflow | Trigger | Runner | Result |
|---|---|---|---|
| `CI` | push to `main`, pull request, manual, reusable call | GitHub-hosted Ubuntu 24.04 | backend/frontend/manifests/Compose runtime gate plus seven-day sanitized runtime evidence |
| `Release` | semantic-version tag or manual rehearsal | GitHub-hosted Ubuntu 24.04 | checksummed versioned package; tagged runs create a GitHub Release |
| `Real kind E2E` | Saturday 18:17 UTC schedule or manual suite choice | self-hosted Windows `aiops-kind` | disposable diagnosis/fleet/search evidence retained for 14 days |

All referenced marketplace actions are pinned to full commit SHAs. Dependabot
opens grouped weekly updates; review the upstream tag and diff before replacing
a SHA. `pull_request_target` is prohibited.

Activation status: private remote `guiyi-labs/aiops-platform` is configured.
Hosted CI run `30325194933` passed all four jobs at revision
`648aea6c94fbc29fbf21d1f799df29880099d454`. Required branch checks and the
dedicated `aiops-kind` runner are not enabled yet.

## Required Branch Protection

After the initial baseline is pushed, protect `main` and require these checks:

- `Backend`
- `Frontend`
- `Manifests`
- `Compose runtime`

Require pull-request review, dismiss stale approvals and block force pushes.
Do not add repository or environment secrets to the `CI` workflow. The Compose
job generates a private `.env`, uploads only service status/logs and removes
the runtime, volume and `.env` in `always()` steps.

## Release Procedure

1. Confirm the working branch is merged and all required checks passed.
2. Run the local `scripts/verify.ps1` gate and review the sanitized artifact.
3. Create an annotated semantic version tag such as `v0.2.0` from the reviewed
   revision and push only that tag.
4. The release workflow reruns the complete reusable CI before packaging.
5. Download the workflow artifact and verify `sha256sum -c SHA256SUMS` on a
   trusted Linux host.
6. For tag runs, verify the GitHub Release points at the same revision recorded
   in `release-metadata.json`.

A manual `workflow_dispatch` with a semantic version is a package rehearsal;
it cannot call `gh release create`. Release packages contain:

- versioned backend and frontend linux-amd64 Docker image archives;
- a source archive from the exact Git revision;
- the OpenAPI contract and production dependency-license inventory;
- revision/run metadata and SHA-256 checksums.

The current phase does not publish a container registry tag, sign artifacts or
attest provenance. Those require reviewed identity, registry and key-management
policies rather than hidden workflow assumptions.

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
