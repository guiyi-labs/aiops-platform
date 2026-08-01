# Security Policy

## Supported Versions

The aiops-platform project is pre-1.0. Security fixes are applied only to the
latest release line. Older releases are not maintained.

| Version | Supported          |
|---------|--------------------|
| latest  | :white_check_mark: |
| < latest| :x:                |

## Reporting a Vulnerability

The aiops-platform team treats security reports as the highest-priority work.
**Do not open public GitHub issues for security problems.**

Report vulnerabilities by one of the following private channels:

1. **GitHub Security Advisory** (preferred): use
   `Security` -> `Report a vulnerability` on the repository. This keeps the
   conversation private and allows the maintainers to request CVE IDs through
   GitHub.
2. **Encrypted email**: send a PGP-encrypted report to
   `security@aiops.local`. The current public key fingerprint is published in
   the release notes of the most recent release.

Please include the following information so we can reproduce and triage the
report quickly:

- Affected version (git tag or `appVersion` from `Chart.yaml`).
- Component (backend, frontend, Helm chart, CI workflow, dependency).
- Step-by-step reproduction, including any required cluster state.
- Observed impact and any proof of concept.
- Suggested fix or mitigation, if any.

### Disclosure timeline

| Stage                         | Target       |
|-------------------------------|--------------|
| Acknowledgement of receipt    | 1 business day |
| Initial triage and severity   | 5 business days |
| Fix or mitigation published   | 30 calendar days for high/critical severity, 90 calendar days otherwise |
| Public disclosure             | After a fix is released, or after 90 days if no fix path is agreed, whichever comes first |

Reporters are credited in the change record unless they request otherwise.

## Threat model boundaries

The following boundaries are explicit, documented design decisions and must
hold for any contribution that touches them. Each boundary is enforced by a
contract test; violations fail the fast gate.

- **Read-only Kubernetes gateway.** The platform reads workload state from
  managed clusters but never mutates it without an explicit, audited
  controlled operation. See ADR 0004.
- **Sanitized, append-only audit trail.** Audit rows are append-only and
  desensitized before persistence. See ADR 0008.
- **Authorization returns 404, never 403.** Missing access returns a
  404 to avoid leaking the existence of a cluster or namespace. See
  ADR 0050.
- **Secrets are external to the Helm chart.** The chart never renders a
  Secret; operators must provide an existing Secret named `aiops-secrets`.
  See `deploy/helm/aiops-platform/values.yaml`.
- **Restricted pod security.** All workloads run as non-root with a
  read-only root filesystem and `drop: [ALL]` capabilities. The namespace
  enforces the Kubernetes `restricted` pod security standard.
- **Network policies.** A default-deny baseline is applied in front of the
  platform namespace; only the documented egress paths (PostgreSQL, DNS,
  Kubernetes API and HTTPS for AI providers) are permitted.
- **Offline readiness gates.** Identity and recovery readiness drills run
  with `--network none` so production trust anchors cannot be silently
  weakened by online checks. See ADR 0032 and ADR 0033.

## CI and supply-chain controls

The following controls are enforced by `.github/workflows/ci.yml` and the
deployment contract tests:

- **Pull-request-only triggers.** `pull_request_target` and `secrets.*` are
  forbidden in CI workflows. CI never has write access to the repository on
  pull-request events.
- **OpenAPI breaking-change check.** `oasdiff breaking --fail-on ERR` runs on
  every pull request and rejects breaking API changes.
- **Coverage baseline.** Backend coverage below 50.0% fails the gate.
- **Static analysis.** `golangci-lint` (Go) and `pnpm lint` (frontend) must
  pass cleanly.
- **Race detector.** `go test -race` runs on every pull request.
- **Verifiable releases.** Release artifacts ship with `SHA256SUMS` and are
  signed and verified with `--verify-tag`. See ADR 0028.
- **License allowlist.** Production dependencies must appear on the
  allowlist in `docs/thesis/dependency-licenses.md`; new licenses require an
  ADR update before they can be merged.

## Credential handling

- Cluster credentials are encrypted at rest with AES-GCM using a key
  versioned in the database. See ADR 0003.
- Rotating the credential encryption key is an offline, audited, dry-run-by-
  default operation driven by `credential-reencrypt`. See ADR 0030.
- The bootstrap admin password, JWT signing key and credential encryption
  key must be provided by the operator through the `aiops-secrets` Secret.
  The chart and the platform image never generate these values.

## Security-conscious contribution checklist

Before opening a pull request that touches security-sensitive code, confirm:

- [ ] No new `pull_request_target` triggers are introduced.
- [ ] No new inline secret values, generated Secrets, or `CHANGE_ME`
      defaults that could be applied by accident.
- [ ] Authorization failures still return 404.
- [ ] New dependencies are added to the license allowlist.
- [ ] New API routes are documented in `openapi.yaml` and covered by the
      route descriptor contract test.
- [ ] New environment variables are documented in `.env.example` and the
      Helm `values.yaml` (without secret values).
