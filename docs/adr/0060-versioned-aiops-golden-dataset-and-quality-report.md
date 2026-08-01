# ADR 0060: Versioned AIOps Golden Dataset And Quality Report (M45)

- Date: 2026-07-31
- Status: Accepted
- Milestone: M45
- Supersedes: none
- Related: ADR 0059 (policy-constrained automation), ADR 0058 (cited AI
  investigator), ADR 0057 (multi-signal correlation), ADR 0056 (SLO and
  error budget), ADR 0055 (temporal topology), ADR 0054 (unified signal
  model)

## Context

The optimization plan (`docs/kubesphere-optimization-plan.md` §19) requires
M45 to bind a versioned AIOps golden dataset and quality report to the
release revision. The plan §20 specifies that M39-M45 milestones must
"additionally run the same versioned golden replay set" and that "rule,
correlation, prompt, model, provider or evidence-schema changes must
produce a machine-readable before/after quality report rather than
silently replacing the baseline."

Prior to M45, each AIOps package had its own golden fixtures:
- M42 `correlation.GoldenFixtures()` — 9 correlation scenarios.
- M43 `aiinvestigator.GoldenFixtures()` — 10 investigation validation
  scenarios.

These package-specific fixtures verify each engine in isolation but do
not:
1. Exercise the **full AIOps loop** (signal → topology → SLO →
   correlation → investigation → automation → verification) as a single
   deterministic scenario.
2. Provide a **cross-cutting quality report** that tracks before/after
   metrics when any engine component changes.
3. Define the **mandatory 10-step end-to-end golden scenario** required
   by the M45 plan.
4. Include the **negative companion scenarios** (misattribution
   prevention, partial/unknown fail-closed).

M45 closes this gap without introducing new runtime behavior. The golden
dataset is an offline evaluation contract, not a runtime component.

## Decision

### 1. Golden dataset package

Create `backend/internal/golden` as the single source of truth for the
versioned AIOps golden dataset. The package is pure: it defines the
dataset structure and the expected outcomes but does not execute against
real Kubernetes, Prometheus, Loki or AI providers. The test suite
verifies dataset integrity, determinism and step coverage.

The dataset is versioned via `DatasetVersion = "1.0"`. Bumping the
version is a contract change that requires a quality report (before/after
comparison). The dataset contains 3 scenarios:

- `mandatory_end_to_end` — the 10-step mandatory golden scenario.
- `negative_misattribution` — the misattribution prevention companion.
- `negative_partial_evidence` — the partial/unknown fail-closed companion.

### 2. Mandatory 10-step end-to-end scenario

The mandatory scenario maps each step of the AIOps loop to an expected
outcome:

| Step | Stage | Packages |
|---|---|---|
| `establish_healthy_service` | Healthy service and SLO | M41 |
| `publish_bad_image` | Accepted fixed operation | M23 |
| `capture_signals` | Rollout change, Pod/Event, metric/SLO, log signals | M39, M41 |
| `build_impact_graph` | Ingress/Gateway-to-Deployment graph and timeline | M40 |
| `rank_cause_candidate` | First deterministic cause candidate | M42 |
| `generate_investigation` | AI investigation with cited evidence and uncertainty | M43 |
| `preview_approve_rollback` | Preview and approve exact revision rollback | M44 |
| `execute_verify` | Idempotent execution and resource/SLO recovery verification | M44 |
| `recover_alert` | Alert recovery, diagnosis/action outcome, notification | M27 |
| `cleanup` | Clean clusters, credentials, fixtures, artifacts | — |

Each `StepOutcome` records which AIOps stages are expected to fire
(signal captured, topology edge, SLO evaluated, correlation case,
investigation, action plan, verification status, alert recovered).
The test suite verifies that the mandatory scenario exercises every
stage of the AIOps loop.

### 3. Negative companion scenarios

Two negative companions verify safety invariants:

- **Misattribution prevention** (`negative_misattribution`): an unrelated
  simultaneous change in another Namespace must NOT be attributed to the
  primary case. The scenario expects a correlation case for the primary
  Namespace but does NOT expect an action plan (the unrelated change does
  not trigger automation).

- **Partial/unknown fail-closed** (`negative_partial_evidence`): when one
  metrics/log provider is stopped, the case must be partial/unknown
  rather than falsely healthy or resolved. The scenario expects a valid
  advisory investigation (with uncertainty) but does NOT expect alert
  recovery (partial evidence does not resolve the alert). This preserves
  the M41 fail-closed invariant.

### 4. Quality report structure

`QualityReport` is the machine-readable before/after comparison required
by the plan. It records:

- `DatasetVersionBefore` / `DatasetVersionAfter` — the dataset versions
  compared.
- `EngineVersionsBefore` / `EngineVersionsAfter` — the package versions
  (M39 signal, M40 topology, M41 SLO, M42 correlation, M43 investigator,
  M44 automation, M44 verifier) used in each replay.
- `ScenarioResults` — per-scenario `(passed_before, passed_after, delta,
  steps_passed_before, steps_passed_after, steps_total, notes)`.
- `Summary` — aggregated metrics (total scenarios, passed before/after,
  improved/regressed/preserved/unchanged, total steps).
- `ChangedComponents` — the components that changed (e.g. "correlation
  rule set", "aiinvestigator prompt").
- `Reviewer` / `Approved` — human review state.

`ClassifyDelta(before, after)` returns "preserved" (both pass),
"improved" (fail→pass), "regressed" (pass→fail) or "unchanged" (both
fail). `Summarize(results)` aggregates the per-scenario deltas into the
`QualitySummary`.

The quality report is generated offline. It never self-modifies rules,
prompts or policy online — this preserves the M43/M44 advisory-only
invariant.

### 5. Dataset is the replayable contract

The dataset is deterministic: `DefaultDataset()` returns the same
scenarios on every call. The test suite verifies this (`TestDatasetDeterminism`).
Replay integrity is the core M45 invariant: identical dataset version +
identical engine versions → identical scenario outcomes. A regression
in any engine (M39-M44) that changes a scenario outcome must produce a
quality report with `delta = "regressed"` so the reviewer can decide
whether the change is an improvement or a regression.

## Consequences

- **Golden dataset is the M45 contract.** The dataset is bound to the
  release revision. Any change to the dataset (new scenario, changed
  expected outcome) requires a version bump and a quality report.
- **Package-specific fixtures remain.** The M42 `correlation.GoldenFixtures()`
  and M43 `aiinvestigator.GoldenFixtures()` remain as package-level
  unit tests. The M45 golden dataset is the cross-cutting contract that
  ties them together.
- **No runtime behavior change.** The golden package is pure and offline.
  It does not add routes, middleware or database tables.
- **Quality report is machine-readable.** The report is JSON-serializable
  so CI can diff before/after and block regressions.
- **Deferred**: real Kubernetes/Prometheus/Loki/AI-provider replay
  (requires real environment), CI integration that generates the quality
  report on every PR, the full 10-step real-kind E2E (requires multi-
  worker kind cluster), and the frontend quality dashboard.
