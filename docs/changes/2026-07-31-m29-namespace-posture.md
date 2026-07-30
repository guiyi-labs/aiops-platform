# M29: Namespace Governance and Capacity Posture

**Date**: 2026-07-31
**Milestone**: M29 (post-baseline governance track, P1)
**Status**: ✅ Completed
**ADR**: [0045-namespace-governance-and-capacity-posture.md](../adr/0045-namespace-governance-and-capacity-posture.md)
**Related milestones**: M25 (read-only resource workbench), M27 (alert lifecycle), M28 (controlled backup)

---

## Summary

Built a deterministic, source-cited Namespace governance posture view that joins
ResourceQuota, LimitRange, Workload (5 kinds), Pod, PodDisruptionBudget and
cluster-wide Node capacity reads into a compact, double-pane frontend view.
Every section carries its own `EvidenceCitation` so partial failures, RBAC
denials and list truncation are reported honestly rather than silently masked.

The posture is strictly read-only and explicitly refuses 7 categories of
scheduler / NetworkPolicy / saturation inference (recorded in ADR 0045 §6).

---

## Changes

### Backend (internal/namespaceposture package + HTTP layer)

1. **`internal/namespaceposture/model.go`** — Domain model
   - `SourceStatus` enum: `complete` | `partial` | `truncated` | `unavailable`
   - `EvidenceCitation` struct: `api_path`, `status`, `total`, `returned`,
     `remaining`, optional `error`, `collected_at` (RFC3339 UTC)
   - 6 section postures: `ResourceQuotaPosture`, `LimitRangePosture`,
     `WorkloadSummary`, `PodSummary`, `PDBPosture`, `NodeCapacityPosture`
   - `NamespacePosture` top-level struct + `PartialSections` convenience array
   - `PostureListEntry` compact summary for the list route
   - `newCitation` / `markTruncated` / `markError` helpers

2. **`internal/namespaceposture/service.go`** — Aggregation service
   - Bounded `KubernetesSource` interface (11 reviewed methods; NO mutation)
   - `Service.Get(clusterID, namespace)` — Required Namespace metadata read +
     6 concurrent goroutines for the remaining sections, each independently
     recording its own citation. Per-section cap = 100 items.
   - `Service.List(clusterID, query)` — Namespace list with lightweight
     count rollups; no full six-way fan-out per row to keep bounded.
   - `collectWorkloads`: 5 sub-kinds (Deployment, StatefulSet, DaemonSet, Job,
     CronJob) fan out concurrently; CronJobs intentionally contribute 0 to
     desired/ready totals (no steady-state replica dimension).
   - `collectPods`: phase + node spread, `unique_node_count`
   - `collectNodeCapacity`: cluster-level (not Namespace-scoped) capacity as
     denominator context; share-of-cluster calculation explicitly REFUSED
   - `sortedPhaseCounts`, `sortedNodeSpreads`, `copyMap`, `stringValue`,
     `namespacedAPIPath` deterministic helpers

3. **`internal/namespaceposture/service_test.go`** (NEW, 10 tests, all PASS)
   - `TestGet_ReadsNamespaceMetadata` — name/phase/evidence completeness
   - `TestGet_NamespaceNotFound` — `k8sgateway.ErrResourceNotFound` propagation
   - `TestGet_PartialSectionMarked` — RBAC-denied RQ leaves other sections OK
   - `TestGet_WorkloadAndPodAggregation` — desired/ready totals + node spread
   - `TestList_SummarizesCounts` — per-Namespace compact count rollups
   - `TestMarkTruncated_SetsTruncatedWhenRemaining` — 250/100/150 → truncated
   - `TestSortedPhaseCounts_OrdersAlphabetically` — Failed < Pending < Running
   - `TestCopyMap_NilOnEmpty` + copy isolation (no mutation-through-reference)
   - `TestStringValue_HandlesNumericTypes` — nil/string/int32/int64/int/float64
   - `TestNamespacedAPIPath_WithAndWithoutNamespace`

4. **`internal/httpserver/namespace_posture.go`** (NEW) — HTTP handlers
   - `namespacePostureHandler` with `list()` and `get()` methods
   - `writeServiceError` that maps `cluster.ErrNotFound` → 404 CLUSTER_NOT_FOUND
   - `namespace-posture` group mounted under `/api/v1/clusters/:cluster_id`

5. **`internal/httpserver/router.go`** — Route registration
   - `GET /api/v1/clusters/:cluster_id/namespace-postures` → list
   - `GET /api/v1/clusters/:cluster_id/namespace-postures/:namespace` → get

6. **Service wiring**: `cmd/server/main.go` wires `*namespaceposture.Service`
   into the HTTP server constructor (no new DB tables or migrations needed).

### Frontend (Vue 3 + TypeScript)

7. **`frontend/src/types/kubernetes.ts`** — TypeScript posture types
   - `SourceStatus`, `EvidenceCitation`, `WorkloadKindCount`, `WorkloadSummary`
   - `PodPhaseCount`, `PodNodeSpread`, `PodSummary`
   - `ResourceQuotaEntry` / `Posture`, `LimitRangePosture`, `PDBEntry` / `Posture`
   - `NodeCapacityEntry` / `Posture`, `NamespacePosture`, `PostureListEntry`

8. **`frontend/src/api/kubernetes.ts`** — API client methods
   - `listNamespacePostures(token, clusterID, name?)` → `ListResponse<PostureListEntry>`
   - `getNamespacePosture(token, clusterID, namespace)` → `NamespacePosture`
   - Cleaned up unused `BackupStorageLocation` type import

9. **`frontend/src/views/NamespacePostureView.vue`** (NEW) — Double-pane UI
   - **Left pane**: Namespace list with phase badge, 5 count chips
     (workloads/pods/quotas/LR/PDB), evidence-completeness pill
     (`完整` / `N 段不完整`), search filter, cluster switcher, refresh
   - **Right pane**: Full posture for the selected Namespace
     - Namespace metadata: phase, created_at, labels count, partial-section tags
     - **ResourceQuota section** — per-quota tables of hard/used
     - **LimitRange section** — per-item rows with default/defaultRequest/
       min/max across all resource keys in the LimitRange
     - **Workload summary** — 4 summary cards (total kinds, desired, ready,
       ready-rate %) + per-kind table of count/desired/ready/available/
       updated/failed
     - **Pod distribution** — total/scheduled/node-count + by-phase list and
       by-node Top-6 list
     - **PDB section** — name, min/maxUnavailable, current/desired healthy,
       disruptions-allowed badge, expected pods
     - **Node capacity (context)** — cluster denominator with explicit
       disclaimer that share-of-cluster is NOT computed (would require
       scheduler-semantic inference)
   - Every section head renders an `EvidenceBadge` (complete → green,
     partial → gray, truncated → yellow, unavailable → red) plus
     returned/total counts when applicable
   - `collectLRKeys` / `truncateNode` helpers mounted in a separate
     `<script lang="ts">` block so the `<template>` can call them

10. **`frontend/src/router/index.ts`** — Route
    - `/namespace-posture` → `NamespacePostureView.vue` (no role gating;
      matches other Kubernetes read routes)

11. **`frontend/src/components/ConsoleLayout.vue`** — Nav entry
    - 分析与治理 → 命名空间治理, icon `LayoutGrid`, route `/namespace-posture`

### Docs + Roadmap

12. **`docs/adr/0045-namespace-governance-and-capacity-posture.md`** (NEW)
    - §1 Separate package with bounded `KubernetesSource` interface
    - §2 Evidence citation semantics (4 statuses + partial_sections)
    - §3 Concurrent bounded fan-out in `Get`
    - §4 Compact list summary, not per-Namespace full posture
    - §5 Reviewed workload kinds only (5 kinds, no ReplicaSet)
    - §6 **Explicit non-inferences**: no quota ratios, no LR conflicts, no
      PDB coverage, no node-share attribution, no NetPol reachability, no
      ownerRef expansion
    - §7 Read-only HTTP surface, no audit actions
    - §8 Frontend: two-pane view under 分析与治理

13. **`docs/changes/2026-07-31-m29-namespace-posture.md`** (this file)

14. **`docs/roadmap.md`** — M29 marked ✅ Completed on 2026-07-31 with
    verification evidence, ADR cross-reference, and closure change-log link.

---

## Verification

### Fast verification (blocking gate)

```
powershell -ExecutionPolicy Bypass -File scripts/verify-fast.ps1 -Scope All
→ Fast verification passed in 28.56 seconds
  (backend=True, frontend=True, manifests=True)
```

Breakdown:
- `gofmt`: 3 files reformatted (main.go, router.go, service_test.go)
- `go vet ./...`: PASS
- `go test ./...`: all 24 backend packages PASS, incl. 10 new
  `namespaceposture` tests
- `vue-tsc -b`: 0 frontend type errors
- `vitest run`: 17 test files · 73 frontend tests PASS
- Compose + Kustomize contracts: PASS

### Unit test inventory (new)

10 tests in `internal/namespaceposture/service_test.go`, 0 flakes after 5
consecutive runs with `-count=5`.

### Frontend typecheck

`vue-tsc --noEmit` returns exit 0. Fixed during M29:
- Removed unused `BackupStorageLocation` import from `api/kubernetes.ts`
- Replaced `(value, key) in collectLRKeys(item)` (which left `value` unused
  and caused `number` vs `string` comparison errors) with
  `(resKey, idx) in collectLRKeys(item)` + `idx === 0` for the rowspan
  merge, and `resKey` (already `string`) for map indexing — removing 4
  spurious `as string` casts and one unused-var diagnostic.

---

## Non-goals (closed, reopen as separate milestones)

- ❌ ResourceQuota usage-ratio / saturation warning → belongs to M21/M27
  diagnosis with sustained-window evidence, not posture
- ❌ LimitRange conflict detection across multiple LRs — creation-time
  ordering is unobservable from list data
- ❌ PDB selector-match coverage against Pods — requires exact label-set
  evaluation; schedule as diagnosis-rule input in a later milestone
- ❌ Namespace share-of-cluster attribution — requires QoS/preemption/
  overcommit scheduler inference we REFUSE
- ❌ NetworkPolicy reachability report — requires full set-based selector
  evaluation; separate design needed
- ❌ Workload ownerReference chain expansion (CronJob → Job → Pod)
- ❌ Any mutation (POST/PATCH/DELETE) on Namespace posture — this is a
  read-only surface by design
- ❌ new DB migration / new audit actions — no state to persist, no
  mutations to audit

---

## Known follow-ups (deferred, not blockers)

1. **Real-kind E2E script** — analogous to `scripts/e2e-m27-alert-lifecycle-kind.ps1`;
   can be added in a follow-up as the default kind cluster already has
   usable Namespace/ResourceQuota/workload fixtures.
2. **Top-N expansion in list** — `List()` currently applies the same
   section caps as `Get()`; a future revision can use `limit=0` count-only
   queries if the gateway exposes that mode.
3. **Pod QoS class breakdown** — can be added alongside phase/spread without
   violating the no-inference boundary (QoS class is observable on each
   Pod status directly).
4. **Topology-spread hint** — can be rendered as a raw observation without
   claiming scheduler causation, if the need arises.
