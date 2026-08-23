# AIOps Platform

**Kubernetes fault diagnosis with case memory.** Deterministic rule-based root-cause
diagnosis, automatically distilled into a searchable case library — so the next time the
same failure happens, you (or your AI) recall the past fix in seconds.

![Dashboard](docs/screenshots/01-dashboard.png)

**Live demo** — `aiops diagnose` on a real fault-injected kind cluster: rule-based findings → case recall:

![aiops CLI demo — real fault cluster](docs/screenshots/demo-cli.gif)

```bash
# Try it in one line (CLI, no cluster web stack needed)
git clone https://github.com/guiyi-labs/aiops-platform
cd aiops-platform/backend
go install ./cmd/aiops
aiops diagnose --kubeconfig ~/.kube/config
```

`Multi-cluster · Diagnostic-first · Audit-driven · AI-assisted (with citations)`

[![CI](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/guiyi-labs/aiops-platform/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-%E2%89%A570%25%20(global)%20%2F%20%E2%89%A575%25%20(core)-brightgreen)](#)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](#)
[![Vue.js](https://img.shields.io/badge/Vue.js-3-4FC08D?logo=vuedotjs&logoColor=white)](#)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.36-326CE5?logo=kubernetes&logoColor=white)](#)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue)](#)

**Companion skill:** [KubeMD](https://github.com/guiyi-labs/kubemd) — the same
diagnosis + case-memory engine, packaged as a DSH agent skill (drop-in
`~/.dsh/skills/`). CLI for terminals, KubeMD for agents.

---

## What makes it different

Most Kubernetes observability tools show you *what is wrong*. This platform also
remembers *how you fixed it*:

1. **Deterministic diagnosis** — rule-based multi-signal correlation (metrics, logs,
   events) builds an evidence timeline (`finding → evidence → assessment`). No
   black-box guessing: every conclusion is traceable.
2. **Case memory (RAG)** — every resolved diagnosis is automatically distilled into a
   searchable case library. The next time the same failure appears, retrieval (B-tree
   shortlist + optional LLM re-rank) recalls how it was fixed before — in seconds.
3. **AI explanation, verifiable** — AI assists only as an optional enhancer, and every
   citation is checked (`historical:N` evidence references the case library). Fabricated
   citations are rejected.
4. **Controlled operations** — high-risk changes (rollout restart, scale, suspend)
   always go through `dry-run → confirm → idempotent → audit`. AI never mutates your
   cluster directly.

## Core capabilities

| Diagnosis | Memory |
|---|---|
| Deterministic rule diagnosis across metrics / logs / events | Every resolved diagnosis auto-distilled into the case library |
| Evidence timeline (finding → evidence → assessment) | Historical similar-case retrieval (two-stage: B-tree + optional LLM re-rank) |
| AI explanation with verifiable citations (`historical:N` evidence) | Next time the same failure occurs, recall the past fix in seconds |

| Controlled operations | Platform |
|---|---|
| dry-run → confirm → idempotent → audit | Multi-cluster federation / global search |
| Operator / CRD (`ControlledOperation`) | Multi-tenant 2D authorization (cluster + namespace) |

## Quickstart

```bash
# Option A — CLI (recommended for trying out)
# Release binary: grab `aiops-<os>-<arch>` from the latest Release assets.
# Or from source:
git clone https://github.com/guiyi-labs/aiops-platform
cd aiops-platform/backend
go install ./cmd/aiops
aiops diagnose                                  # run deterministic rule diagnosis
aiops cases --query "crashloop"                 # search historical cases (degraded to pure rules without a server)

# Option B — Full web platform (docker compose)
git clone https://github.com/guiyi-labs/aiops-platform
cd aiops-platform
docker compose up -d
# UI: http://localhost:18080

# Option C — End-to-end demo on a real kind cluster (PowerShell)
# Spins up a fault-injected kind cluster, registers it, runs diagnoses,
# and exercises the controlled-remediation chain:
pwsh -File scripts/demo-up.ps1
```

The CLI speaks English, supports `-o json` for scripting, and uses semantic exit codes:
`0` = clean, `1` = warnings found, `2` = error.

Helm chart and Kustomize manifests are also provided under `deploy/` for cluster
deployments — see [`docs/`](docs/README.md) for details.

## Architecture

```mermaid
flowchart LR
    A[Multi-cluster / kubeconfig] -->|read-only bounded gateway| G[Gateway]
    G --> D[Deterministic Diagnosis]
    D --> E[Evidence Timeline]
    D --> K[(Case Library / RAG)]
    K -->|retrieval: B-tree + optional LLM re-rank| AID[AI explain w/ citations]
    E --> C[Console / CLI]
    D --> O[Operator / Controlled Ops]
    O -->|dry-run → confirm → idempotent| A
```

Every diagnosis conclusion is backed by an immutable evidence snapshot; the case
library is auto-populated from resolved diagnoses and never requires a human to write
it by hand.

## Demonstration scripts

| Script | What it proves |
|---|---|
| `scripts/demo-up.ps1` / `demo-down.ps1` | Full fault-injection → diagnosis → remediation replay on a real kind cluster |
| `scripts/verify-fast.ps1` | Fast local gate: gofmt → vet → build → tests (backend/frontend/manifests) |
| `backend/cmd/aiopsbench` | Offline quality benchmark: diagnosis P/R/F1 over a labeled corpus + retrieval Hit@k/MRR |
| `scripts/load-probe.mjs` | Dependency-free API latency probe (p50/p95/p99 across concurrency levels) |

The `aiopsbench` corpus (38 labeled scenarios, 12 rules) is also a CI contract:
`TestCorpusLabelsMatchEngine` fails if a rule's behavior drifts from its reviewed label.

## Engineering signals

- Coverage **≥ 70% overall** (core packages **≥ 75%**) · linters **0 issues** ·
  `go test -race` enforced in CI · backend + Vue 3 frontend typecheck clean
- CI chain: gofmt → vet → lint → coverage → **fuzz/bench** → race → frontend
  Playwright (dual-viewport) → release gate
- Go 1.26 backend · Vue 3 + TypeScript frontend · client-go v0.36.x · PostgreSQL 17
- 71 `internal/` packages · 12 CLI/server binaries under `backend/cmd/`

## Repository layout

```text
backend/             Go API service + CLI + operator
backend/internal/    diagnosis, knowledge (RAG), cluster, correlation, ... (71 packages)
backend/cmd/aiops/   aiops CLI (diagnose / cases)
backend/cmd/aiopsbench/  offline quality benchmark (P/R/F1 + retrieval Hit@k)
frontend/            Vue 3 web console
deploy/              Compose, Kubernetes, Helm chart and demo manifests
docs/                Architecture, conventions, decisions and change records
SECURITY.md          Supported versions, vulnerability reporting, threat-model boundaries
CHANGELOG.md         Keep a Changelog 1.1.0 / SemVer 2.0.0 history
```

## Project boundaries

- Kubernetes live resources are queried through the API server, never bulk-copied
  into the platform database.
- Rule-based diagnosis is the deterministic main path; AI only enhances explanations.
- AI never executes cluster changes directly.
- All high-risk operations require permission checks, human confirmation and audit logs.
- kubeconfig and other credentials must never be committed to Git.

This project starts from an **existing, reachable Kubernetes cluster** and owns Day 2
runtime management. Cluster bootstrapping (kubeadm, CNI, control-plane HA, Linux host
ops) belongs to the sibling repos
[`kubernetes-cluster-bootstrap`](https://github.com/guiyi-labs/kubernetes-cluster-bootstrap)
and [`devops-automation`](https://github.com/guiyi-labs/devops-automation).

## Roadmap

Two parallel tracks, both executed under the same archival and CI constraints:

- **Flagship deepening (P2)** — RAG demo polish (seed script + read-only knowledge
  endpoint), then multi-cluster Federation deepening: a single pane that diagnoses the
  whole fleet (cross-cluster diagnosis aggregation + batch inspection). See
  [`docs/enhancement-p2-flagship-roadmap.md`](docs/enhancement-p2-flagship-roadmap.md).
- **Product line (M112–M114)** — incident/optimization/observability organized into a
  unified context work surface reusing the runbook catalog, AI citations, controlled
  actions and SLO signals. See
  [`docs/development-roadmap-post-m110.md`](docs/development-roadmap-post-m110.md).

Production-ready claims additionally require organization-approved OIDC/MFA, physical/WAL
PITR and HA drills — see `SECURITY.md` and the long-term roadmap.

## Documentation

- Full feature walkthrough, design decisions and test matrix: [`docs/`](docs/README.md)
- Change history: [`CHANGELOG.md`](CHANGELOG.md) · change records: [`docs/changes/`](docs/changes/)
- Security model and boundaries: [`SECURITY.md`](SECURITY.md)
- License: [Apache 2.0](LICENSE)
