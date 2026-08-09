# ADR 0079 — M81 AIOps Closed-Loop Insight (read-only)

- Date: 2026-08-09
- Status: Accepted
- Related milestones: M81 (Polish Phase 1, workstream W5)
- Supersedes / context: extends the read-only posture philosophy (ADR 0004 /
  ADR 0076–0078) with a deterministic correlation layer between the M61-M78
  optimization findings and the M18/M43 diagnosis, M52 inspection, M55 AI
  explanation and M19/M44 controlled-operation preview contracts.

## Context

The optimization analyzers (M61-M78) emit posture findings, but the console
cannot yet answer "what do I do next with this finding" without manually
jumping between unrelated pages. The diagnosis (M18/M43), inspection (M52),
AI explanation (M55) and dry-run preview (M19) flows exist as separate
endpoints; nothing connects them.

## Decision

Introduce `backend/internal/insight` as a pure, compiled-in correlation
catalog:

- `insight.Resolve(clusterID, domain, kind, namespace, name, findingCode)`
  returns a deterministic `Runbook`:
  - `diagnoses` — the applicable deterministic diagnosis route(s) and their
    compiled-in rule IDs, keyed by resource kind;
  - `inspection` — M52 catalog rules that corroborate the finding domain;
  - `ai_explanation` — the M55 cited-explanation entry point (present when a
    diagnosis route applies);
  - `operations` — dry-run preview candidates (M19/M44 action shapes) keyed by
    resource kind; the actual remediation previews stay behind the existing
    ops_admin-guarded APIs.
- **Read-only invariant (ADR 0004):** the package and its HTTP endpoint never
  reach a cluster, never mutate state, and never widen the security boundary.
  The new endpoint requires only authentication (no write role) and performs
  no cluster side effects.
- The console deep-links from a posture finding to the runbook; every step
  still executes through the existing guarded services.

## Consequences

- The closed-loop chain "finding → diagnosis → inspection → AI explanation →
  dry-run preview" is now one deterministic contract instead of five
  unrelated pages.
- The endpoint is safe for any authenticated user and fully unit-testable
  without a cluster.
- Future analyzers only need to add entries to `insight` maps; the contract
  is append-only.