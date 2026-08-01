# ADR 0070: DevOps Read-Only + Cross-Cluster Copy + Backup/Restore GUI (M58)

- Date: 2026-08-01
- Status: Accepted
- Milestone: M58
- Supersedes: none
- Related: ADR 0024 (resource-originated controlled operations),
  ADR 0041 (fixed cross-cluster promotion), ADR 0042 (cluster workload
  protection integration), ADR 0049 (route descriptor contract and
  RBAC inventory), ADR 0054 (unified service identity and signal model),
  ADR 0064 (CRD discovery and browsing), ADR 0067 (intelligent
  inspection and service-mesh readonly), ADR 0069 (Helm app catalog)

## Context

Phase 3 of the post-M45 roadmap calls for three DevOps-facing backend
enhancements in M58:

1. **GitOps read-only (ArgoCD Application)**. Operators need to browse
   ArgoCD Application CRs on each managed cluster to see sync/health
   status and track which changes have been shipped by GitOps. Before
   M58 the platform had no ArgoCD visibility, so GUI users had to open
   a separate ArgoCD dashboard.
2. **Interactive cross-cluster copy (copyops)**. Promotion (M19) is
   *version-gated* and *rehearsed* via the fixed M24 promotion
   contract, but operators often need a lighter-weight "copy a bundle
   of namespaced resources from cluster A to cluster B" interaction
   during incident response, staging → prod smoke, or disaster-recovery
   rehearsals. copyops is that lighter-weight primitive, built on the
   same M19 controlled-operation state machine as M19 promotion/M22
   backup/M23 restore.
3. **Backup/Restore GUI backend support**. The platform already had
   controlled Velero backup creation (M22) and restore rehearsal
   (M23) via CRUD operations on *Plans*, but the GUI could not browse
   the *actual* Velero Backup/Restore CRs that already exist on each
   cluster. M58 adds read-only "list/get backups, list/get restores"
   endpoints so the GUI can render existing backups before users
   create new ones.

The three M58 components share two architectural requirements with
earlier milestones: (a) a *bounded* kubernetes read path (ADR 0004)
and (b) reuse of the M19 controlled-operation contract
(confirmation-token + idempotency-key + Claim state machine) wherever
a write is performed. copyops is the only new writer in M58; GitOps
and Velero browse endpoints are strictly read-only.

Key constraints:

- No ArgoCD SDK, no Velero SDK. All reads go through the same
  bounded Kubernetes gateway (ADR 0004) that already powers CRD
  discovery (ADR 0064). ArgoCD Applications are read as raw CRs via
  `customResourceWhitelist` extended with `argoproj.io/v1alpha1
  Application`. Velero Backups/Restores are read using the existing
  cluster-scope dynamic client.
- copyops bundle size is capped at 20 items and the resource kind
  list is an operator-curated whitelist (common workload and config
  kinds). Cluster-scoped resources and admission controllers are
  explicitly excluded.
- copyops must never overwrite an existing destination resource. The
  "already exists" preflight skips the item, and server-side dry-run
  failure on the specific item sets its per-item status to `skipped`
  with a human-readable reason rather than mutating the cluster.
- copyops reuses the M28 backup Compare-And-Swap (CAS) gate at
  Execute time: the SourceNamespace UID/ResourceVersion captured at
  Preview must match at Execute. This closes torn-read races where a
  namespace is deleted and recreated between Preview and Execute.
- All three components surface read-only list/get endpoints that
  honour the M40 workspace + per-cluster scoped-route layout
  (ADR 0061 workspace multi-tenancy, ADR 0062 three-tier console).

## Decisions

### 1. GitOps read-only integration uses "direct Application CR" mode only

The implementation probes for `argoproj.io/v1alpha1 Application` CRDs
on the cluster. When the CRD is absent, `/gitops/capability` reports
`{available: false, mode: "none"}` and list/get return empty results
rather than 500. When present, list/get return Application manifests
read via the standard cluster-scope dynamic client and projected into
a GUI-friendly `{name, namespace, project, sync_status, health_status,
repo_url, repo_path, destination, ..., raw_manifest}` envelope.

Explicitly rejected alternatives:

- **ArgoCD API server proxy**. Requires extra credentials, mTLS
  management, and surface area; the CR projection already gives the
  GUI everything it needs for M58 (read-only sync+health visibility).
- **Event stream publication into M40 ChangeEvents**. Deferred to a
  later milestone (the spec text marks this as a post-M58 nice-to-have
  only). M58 stops at REST browse endpoints.

Why this is safe: reads are already bounded by ADR 0004 rules
(namespace scope, list size caps, workspace+cluster scoped routes), so
the new surface is read-only and bounded.

### 2. copyops: one-row, JSONB-heavy plan table instead of child tables

In contrast to promotion (which has separate child tables for steps,
history, evidence), copyops stores an entire plan in a single row of
the new `copy_plans` table, with `resource_items`, `copy_summary`,
and `diff` stored as JSONB columns. Rationale:

- copyops plans are small (<=20 items) and short-lived (10-minute
  TTL). A single-row write gives us atomic Preview persistence
  without a transaction.
- The GUI renders the plan as a whole; it never pages *across*
  resource items of a single plan. JSONB projection is fast enough.
- Replay/idempotency is easy to reason about: `ClaimAndLoad`
  atomically transitions `status = 'awaiting_confirmation' →
  'executing'` with a lease column (`locked_at`) on the same row,
  mirroring the controlled-operation pattern from promotion and
  backup.

The `id` column uses `char_length(id) = 36` (matching the CHECK
constraint in the migration) so that the copy plan row is visually
consistent with the UUID-based IDs used by promotion. The prefix
`cp-` makes the ID recognizable in logs.

### 3. copyops manifest scrubbing: fixed label/annotation prefix strip + Secret scrub toggle

Before any manifest is written into the plan's `resource_items`, the
preview pipeline scrubs it with:

- Strip known "operator-internal" label and annotation prefixes:
  `argocd.argoproj.io/`, `kubectl.kubernetes.io/last-applied-configuration`,
  etc. The exact prefix list is passed by the handler (it can grow
  without a service rebuild).
- Node-scrubbing: drop `spec.nodeName` (and equivalent pinning fields
  that don't make sense across clusters).
- Cluster-specific UIDs and resourceVersion are captured as separate
  `source_uid` / `source_resource_version` fields and *not* part of
  the destination manifest.
- If `strip_secrets = true`, `Secret` kinds are additionally replaced
  with a placeholder `data` map so operators can share plan previews
  without leaking credential values.

Why not server-side apply with field managers? copyops only does
*create* paths in M58 (never update/delete), so preflight "already
exists" combined with server-side dry-run is sufficient. Update and
delete modes are left for a future milestone where the Compare-And-
Swap gate is extended to the individual resource UIDs.

### 4. Velero Backup/Restore browse: thin projection over the existing kubernetes gateway endpoints

`backup.go` and `restore.go` in `httpserver` already expose the
controlled *Plan* CRUD (backup plans, restore-rehearsal plans). M58
extends them with `listBackups`/`getBackup`/`listRestores`/
`getRestore` endpoints that:

- Accept an optional `namespace` query param (default `velero`),
- List the GVR `velero.io/v1 Backup` / `Restore` via the existing
  dynamic client,
- Project the result into a GUI-friendly row (phase, errors,
  warnings, started/completed timestamps, storage location, schedule,
  included/excluded namespaces) with `raw_manifest` available for
  detail pages.

Why not use the M22 backup service's existing data model? M22 talks
about *Plans* (what we intend to backup); M58 GUI browse cares about
*actual Backups* on the cluster. These are orthogonal entities and
are intentionally kept separate to avoid a hard link that would make
"view any Velero backup" impossible in environments where operators
manually `velero backup create` outside the platform.

### 5. Reordered ClaimAndLoad gate: idempotency-replay checked *before* "already executed" short-circuit

The original copyops repository `ClaimAndLoad` short-circuited
anything that was not `awaiting_confirmation` to
`ErrAlreadyExecuted`, which broke idempotency replay (the most common
GUI retry pattern: user double-clicks Execute). M58 reorders the
gate: for any terminal status (`succeeded`, `failed`, `expired`), we
first check idempotency-key equality and *return the completed plan*
if it matches; only if the idempotency key is missing or mismatched
do we return `ErrAlreadyExecuted` / `ErrInvalidIdempotency`.

This mirrors the pattern callers already get in promotion, backup,
and restore — a consistency requirement that surfaced from the
service-level tests.

## Consequences

### Positive

- GUI operators now see ArgoCD sync/health, existing Velero backups,
  and one-click copy planning *inside* the platform, all through the
  same authentication and workspace-tenancy model.
- copyops is lightweight, fast to test, and reuses the already-
  audited M19 controlled-operation contract (preview/execute,
  confirmation token, idempotency replay, CAS, audit targets).
- Database migration is minimal: just one `copy_plans` table.
- No new heavy dependencies. ArgoCD and Velero integrations rely on
  the existing ADR 0004 Kubernetes gateway, which already handles
  auth, list caps, and workspace tenancy.

### Negative

- copyops create-only semantics mean "upgrade a workload" use cases
  are not covered. A later milestone will need to extend the
  preflight pipeline with a three-way merge (live cluster vs
  stripped source vs user intent) before an "Update" mode is safe.
- GitOps events are not streamed into M40 change timeline in M58. A
  follow-up M60+ enhancement is needed so sync/health deltas feed the
  topology and diagnosis engine naturally.
- Velero browse GUI rows are *projections*; they intentionally omit
  some of the deeper Backup detail fields. If operators need
  per-error rows, that should be added as a separate
  `/velero/backups/{name}/errors` endpoint rather than cramming more
  into the projection.

### Risks

- Velero Backup/Restore CRs can be extremely long-lived and numerous
  in large fleets. List caps must stay small and consistent with
  ADR 0004 bounds; we rely on the standard `apiquery.Parse` page size
  caps plus the existing `MaxListResults` gate in the k8s gateway.
- copyops Manifest scrubber cannot cover every cluster-specific
  annotation injected by every operator. In the worst case a
  namespace-scoped GVR is copied with an annotation that the
  destination cluster treats as "don't reconcile", which will
  surface as a failed/skipped item with a visible DryRunError
  rather than a silent data corruption. The item-level error
  projection keeps these visible in the GUI.
