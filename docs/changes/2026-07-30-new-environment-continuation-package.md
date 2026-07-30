# New Environment Continuation Package

- Status: Accepted
- Date: 2026-07-30
- Base revision: `62320fcac3bbb50b33b7cd6945495264b04b026c`
- Base tag: `baseline-m25-20260730`

## Outcome

The repository now carries a self-contained continuation package for a new
development device and a new Agent. It separates three concerns:

- `docs/agent-start-here.md` gives the Agent an authoritative baseline, read
  order, architecture map, first-turn audit and non-negotiable safety rules.
- `docs/new-environment-bootstrap.md` defines a clean Windows-first toolchain,
  fresh-secret and fresh-cache policy, Compose deployment, migration checks,
  layered verification, real-kind acceptance, optional approved state transfer
  and known Docker/Go recovery procedures.
- `docs/project-vision-and-delivery-standards.md` defines the product north
  star, M26-M29 roadmap, prioritization, milestone proposal, Definition of
  Ready, risk-based verification, Definition of Done, evidence, Git/release and
  Agent exit-report standards.

README, documentation index, development guide, handoff, roadmap and CI/release
manual now link to these entry points. A delivery-asset contract test requires
their core sections so future refactors cannot silently remove the handoff.

## Transfer Boundary

The accepted transfer mechanism is Git plus freshly downloaded verified
dependencies. `.env`, credentials, kubeconfig, Docker Desktop WSL data,
volumes, language caches, `.tools`, `node_modules`, `.artifacts` and retained
kind state must not be copied from the old device. Database state is fresh by
default; any exception requires an explicitly approved encrypted logical dump,
separate key transfer and post-restore verification.

## Verification

- Markdown local-link audit: zero broken links across all changed documents.
- `git diff --check`: passed.
- `go test -count=1 ./internal/deployment`: passed.
- `scripts/verify-fast.ps1`: documentation-only scope correctly selected the
  backend delivery contract; passed in 3.25 seconds.

## Next Action

On the new device, follow `docs/agent-start-here.md` and execute the L0-L3
acceptance ladder in `docs/new-environment-bootstrap.md`. Archive that device's
sanitized acceptance as a new dated change record before beginning M26 work.
