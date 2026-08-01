# Test Matrix

## 2026-07-30 Baseline Addendum

This addendum is authoritative for the M21-M25 baseline; older rows below are
retained as historical evidence for their original revisions.

| Gate | Evidence | Accepted result |
|---|---|---|
| Fast repository gate | `scripts/verify-fast.ps1` | 271 Go `Test*` entries remain discoverable; all Go packages passed vet/test, 17 Vitest files/73 tests passed, and Compose/Kustomize contracts passed in 23.73 seconds after documentation alignment |
| Full repository gate | `.artifacts/verification/verify-20260730-080851.json` | Passed in 121.79 seconds; frontend production build succeeded and PostgreSQL/backend/frontend were healthy with direct and proxied readiness |
| M21 real kind | `.artifacts/m21-history-kind/m21-history-kind-20260730-080558.json` | Exact-series isolation, units, sparse outage gap, recovery, all three evaluation states and backend-restart durability passed; cleanup complete |
| M23 real kind | `.artifacts/m23-release-lifecycle-kind/m23-release-lifecycle-kind-20260729-234238.json` | Image update, exact ReplicaSet revision rollback, idempotent replay and fixture restoration passed |
| M24 real kind | `.artifacts/m24-cross-cluster-promotion-kind/m24-cross-cluster-promotion-kind-20260730-074812.json` | Two-cluster Deployment/Service promotion passed with one deduplicated ConfigMap dependency, mapped reference rewrite and complete cleanup |
| M25 real kind | `.artifacts/m25-workload-protection-kind/m25-workload-protection-kind-20260730-075311.json` | Installed/unavailable Velero capability, two bounded Backup projections, single read, 424 fallback and read-only RBAC passed |

M22 is covered by the current backend, frontend and production-build gates;
its read-only resource/log/manifest scope is archived in
`docs/changes/2026-07-30-m22-daily-troubleshooting-and-governance-workbench.md`.

## 2026-07-31 M27-M32 Final Closure Addendum

This addendum is authoritative for the M27-M32 development route; it closes
the locally executable development and acceptance work. Only external
organization production gates remain (see
`docs/changes/2026-07-30-m32-formal-closure.md`).

| Gate | Evidence | Accepted result |
|---|---|---|
| Fast repository gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 26.17s; full Go vet/test, 73 frontend tests/17 files, typecheck and Compose/Kustomize contracts |
| Full repository gate | `.artifacts/verification/verify-20260731-015255.json` | Passed in 97.68s; backend/proxy ready, frontend 200, 3 healthy Compose services |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `backend/internal/httpserver/openapi_route_test.go` | Audit found 11 missing M28-M31 routes; fixed in this revision; contract test stubs added for Backup/Maintenance/NamespacePosture/Restore |
| Migrations parity | `backend/migrations/000001`-`000024` | 24 up/down pairs; latest applied is 000024; no orphans |
| RBAC parity | M27-M31 HTTP handlers and `deploy/managed-cluster/observer.yaml` | Reviewed exact mutations only; generic update/delete remain denied; M29 is read-only |
| Audit-action parity | `backend/internal/httpserver/audit.go` | M27 (alert_rule.*), M28 (backup.*), M30 (maintenance.*), M31 (restore.*) registered; M29 read-only by design |
| M27 real kind | `.artifacts/m27-alert-lifecycle-kind/m27-alert-lifecycle-kind-20260731013733-e4a6e270.json` | Passed firing, deduplication, outage containment, complete normal-window resolution, restart persistence and cleanup |
| M28 real kind | `.artifacts/m28-backup-creation-kind/summary.json` | Passed pinned Velero/MinIO fixed-scope Backup creation, stale-source rejection, replay, RBAC and cleanup |
| M29 real kind | `.artifacts/m29-governance-posture-kind/summary.json` | Passed deterministic governance findings and cleanup |
| M30 real kind | `.artifacts/m30-node-maintenance-kind/summary.json` | Passed two-worker safe maintenance lifecycle and cleanup |
| M31 real kind | `.artifacts/m31-isolated-restore-kind/summary.json` | Passed pinned Velero/MinIO quarantine restore, mapping, replay and cleanup |
| Responsive browser | in-app acceptance | 390x844 and 1280x720 passed; no page overflow, out-of-bounds controls or warning/error logs |
| Race detector | local Windows toolchain | Blocked because `gcc` is unavailable; explicitly not reported as passed |
| Remote CI + tag/release | — | Deferred — requires user-authorized push to private remote |

No skipped suite is reported as passed. The M27-M32 ADRs (0043-0047),
per-milestone records and final archive are the authoritative scope boundaries:

- `docs/changes/2026-07-30-m27-alert-lifecycle.md`
- `docs/changes/2026-07-30-m28-controlled-backup-creation.md`
- `docs/changes/2026-07-31-m29-namespace-posture.md`
- `docs/changes/2026-07-30-m30-controlled-node-maintenance.md`
- `docs/changes/2026-07-30-m31-isolated-workload-restore-rehearsal.md`
- `docs/changes/2026-07-30-m32-formal-closure.md`
- `docs/changes/2026-07-31-final-baseline-archive.md`

## 2026-07-31 M33-M34 Post-Baseline Addendum

This addendum records the post-M32 transport and contract-debt closures. They
do not change the M32 evidence archive; they layer on top of
`baseline-m32-20260731` without breaking any public API contract.

| Gate | Evidence | Accepted result |
|---|---|---|
| M33 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 85.91s; 26 backend packages vet/test, 73 frontend tests/17 files, Compose/Kustomize contracts |
| M33 transport swap | `backend/internal/cluster/clientprovider.go` (ADR 0048) | Raw `net/http` `cluster.Registry` replaced by `client-go`-backed `ClusterClientProvider`; four gateway interfaces unchanged; Probe/Patch/no-redirect unit tests pass |
| M34 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 26.64s; 3 backend packages vet/test, 73 frontend tests/17 files, Compose/Kustomize contracts |
| M34A descriptor parity | `backend/internal/httpserver/route_descriptor_test.go` (ADR 0049) | Five invariants pass: route coverage, metadata well-formedness, HTTP method validity, APIv1 prefix, no duplicates |
| M34A audit refactor | `backend/internal/httpserver/audit.go` | Hardcoded `operations` map deleted; `auditedOperation` delegates to `findAuditedRoute` over `routeTable` |
| M34B RBAC inventory | `backend/internal/kubernetes/rbac_test.go` | 8 RBAC endpoints (Role/ClusterRole/RoleBinding/ClusterRoleBinding list+detail) verified; nil-slice normalization, path construction and 404 paths covered |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 8 new RBAC GET routes present in both `routeTable` and OpenAPI; no orphan route in either direction |
| Observer RBAC | `deploy/managed-cluster/observer.yaml` | Added `get,list` on `roles/clusterroles/rolebindings/clusterrolebindings` |
| Real-kind E2E | — | Deferred — M33 transport swap and M34 RBAC reads exercise the same `Gateway.Get` interface verified by M22-M31; multi-worker kind cluster not available locally |

Authoritative records:

- `docs/changes/2026-07-31-m33-restricted-client-go-migration.md`
- `docs/changes/2026-07-31-m34-route-descriptor-and-rbac-inventory.md`

## 2026-07-31 M35 Access Grants Addendum

This addendum records the first resource-scope authorization layer. It layers
on top of `baseline-m32-20260731` and the M33-M34 closures without breaking
any public API contract.

| Gate | Evidence | Accepted result |
|---|---|---|
| M35 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 39.44s; backend, frontend and manifests all green |
| Access-grants migration | `backend/migrations/000025_access_grants.up.sql` (ADR 0050) | Adds `user_cluster_grants` and `user_namespace_grants` tables with paired down migration |
| Policy evaluator | `backend/internal/authz/service_test.go` (ADR 0050) | 21 tests covering cluster access, namespace access, visible-cluster filtering, SystemAdmin bypass and grant CRUD |
| Authorization middleware | `backend/internal/httpserver/authz_middleware_test.go` | 11 tests covering `requireClusterAccess` and `requireNamespaceAccess` on fleet, search and resource routes |
| Grant management API | `backend/internal/httpserver/grants_test.go` | 13 tests covering list/create/delete grants, SystemAdmin bypass, 404-on-unauthorized and audit |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | Grant-management routes added under the `access-grants` tag; bidirectional parity preserved |
| Real-kind E2E | — | Deferred — multi-worker kind cluster not available locally; frontend UI and namespace query-param filtering for non-fleet/search routes also deferred |

Authoritative records:

- `docs/changes/2026-07-31-m35-lightweight-cluster-and-namespace-access-grants.md`
- `docs/adr/0050-lightweight-cluster-and-namespace-access-grants.md`

## 2026-07-31 M36 Production OIDC And MFA Addendum

This addendum records the phased OIDC provider integration. OIDC remains
disabled by default; no public API contract has changed across M36A-M36F.

| Gate | Evidence | Accepted result |
|---|---|---|
| M36A configuration | `backend/internal/config/config_test.go` (ADR 0052) | OIDC disabled by default; `loadOIDCConfig` fail-closed validation with a valid-env helper plus 21 invalid-configuration cases, production client-secret requirement and group-to-role deduplication |
| M36A migration | `backend/migrations/000026_external_identities.up.sql` | `external_identities` table with `(issuer, subject)` and `(user_id, issuer, subject)` unique constraints; paired down migration |
| M36A model | `backend/internal/oidc/model_test.go` | `ExternalIdentity` GORM model with pinned `TableName` |
| M36B discovery | `backend/internal/oidc/discovery_test.go` | Fetch+validate, TTL caching, 10 contract-violation cases, HTTPS issuer enforcement and redirect rejection |
| M36B JWKS cache | `backend/internal/oidc/jwks_test.go` | Fetch+KeyByID, fast-path no-refetch, unknown-kid fail-closed, rotation drops retired keys, TTL expiry forces refresh, single-flight coalescing, duplicate-kid rejection, HTTPS JWKS URI enforcement, too-many-keys rejection and unusable-key skipping |
| M36B key conversion | `backend/internal/oidc/keyset_test.go` | RSA/EC/Ed25519 key conversion plus 12 rejection cases (missing kid, disallowed algorithm, wrong kty, short modulus, off-curve point, wrong curve, etc.) |
| M36C PKCE + auth session | `backend/internal/oidc/pkce_test.go` (ADR 0052) | 8 tests: RFC 7636 verifier/challenge shape and uniqueness, `randomString` uniqueness, auth-session signer HMAC round trip, malformed-token rejection (4 cases), wrong-signature rejection, expired-session rejection and corrupted-payload rejection |
| M36C provider flow | `backend/internal/oidc/provider_test.go` | 15 tests + 21 subtests: `ProviderConfig` validation (10 cases), `AuthorizationURL` required params + cookie round trip, `HandleCallback` happy path through a synthetic HTTPS IdP, callback rejection (missing inputs, state mismatch, invalid session, token endpoint 500, missing `id_token`), ID-token contract violations (10 cases), disallowed signing algorithm, unknown `kid`, `amr` evidence acceptance, group-to-role dedup/sort, claim extraction, client secret browser-flow leak guard |
| M36C fast gate (backend) | `scripts/verify-fast.ps1 -Scope Backend` | Passed in 29.87s |
| M36D session/logout | `backend/internal/oidc/session_test.go`; `backend/internal/oidc/breakglass.go` | 6 session-manager tests (CompleteLogin happy path + 4 fail-closed paths + reauthentication interval), 3 provider-logout tests, 1 StripBearerPrefix test (5 cases), 7 break-glass drill tests (event recording, required fields, auditor error propagation, stale-after-interval, config defaults/cap, auditor-less operation) |
| M36E synthetic IdP E2E | `backend/internal/oidc/e2e_test.go` | `TestSyntheticIdPEndToEndLifecycle` (6 ordered subtests: Login, Authorization, Rotation, Disable, Logout, BreakGlass) + `TestSyntheticIdPEndToEndBreakGlassStaleness`; exercises discovery, JWKS cache, PKCE, ID-token verification, MFA evidence, session issuance and break-glass audit through their real implementations against a synthetic HTTPS IdP; retired signing keys fail closed with `ErrUnknownKey`; disabled user fails closed with `ErrUserDisabled` |
| M36E fast gate (backend) | `scripts/verify-fast.ps1 -Scope Backend` | Passed in 26.4s; full `internal/oidc` suite green |
| M36F HTTP wiring + GORM resolver | `backend/internal/httpserver/oidc_test.go`; `backend/internal/auth/service_test.go`; `backend/internal/oidc/gorm_resolver.go`; `backend/internal/oidc/auth_issuer.go` | 3 `IssueSessionForUser` tests (active user, disabled user, missing user); 3 OIDC handler tests (routes absent when disabled, callback rejects missing code/state, callback rejects expired auth-session cookie); GORM `IdentityResolver` resolves (issuer, subject) to prelinked local user and fails closed with `ErrSubjectNotPrelinked` when no row exists; `AuthSessionIssuer` adapter delegates to `auth.Service.IssueSessionForUser` |
| M36F OpenAPI parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | Three OIDC routes added (`GET /auth/oidc/login`, `GET /auth/oidc/callback`, `POST /auth/oidc/logout`); bidirectional route↔OpenAPI parity preserved |
| M36F fast gate (backend) | `scripts/verify-fast.ps1 -Scope Backend` | Passed in 44.3s; all 26 backend packages green |
| Real organization IdP run | — | Externally gated |

Authoritative records:

- `docs/changes/2026-07-31-m36a-oidc-config-and-external-identities.md`
- `docs/changes/2026-07-31-m36b-oidc-discovery-and-jwks-cache.md`
- `docs/changes/2026-07-31-m36c-oidc-authorization-code-and-pkce-flow.md`
- `docs/changes/2026-07-31-m36d-oidc-session-logout-and-breakglass.md`
- `docs/changes/2026-07-31-m36e-synthetic-idp-end-to-end-gate.md`
- `docs/changes/2026-07-31-m36f-oidc-http-wiring-and-gorm-resolver.md`
- `docs/adr/0052-production-oidc-and-mfa.md`

## 2026-07-31 M40 Temporal Topology And Change Intelligence Addendum

This addendum records the M40 temporal topology and change timeline closure.
M40 persists reviewed relationship edges (8 kinds) and normalizes M23-M31
platform-operation outcomes into a unified change timeline. The native M21-M31
signal path is unchanged; M40 is an evidence-graph and change-timeline
normalizer. No public API contract was broken beyond the two new
`aiops/topology` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M40 edge model | `backend/internal/topology/model.go` (ADR 0055) | 8 EdgeKind values (Owns/Selects/RoutesTo/BackedBy/RunsOn/Mounts/Scales/ProtectedBy), 8 DerivationMethod values, ResourceCitation (kind/namespace/name/UID/incomplete), Edge with validity interval, ChangeEvent with confidence/source |
| M40 collector | `backend/internal/topology/collector.go` | Snapshot reads 8 resource types with bounded paging (1000-page cap); DeriveEdges deterministically derives all 8 edge kinds from exact observed evidence; SourceHash for change detection |
| M40 repository | `backend/internal/topology/repository.go` | GormRepository with ON CONFLICT DO UPDATE for edge refresh and change-event idempotency; NopRepository for testing; partial unique index uq_topology_edges_active enforces at-most-one-active-edge |
| M40 service | `backend/internal/topology/service.go` | CollectNamespace (snapshot→derive→upsert→close stale), CollectCluster, GetTopologyGraph (nodes+completeness), GetChangeTimeline, IngestChangeEvent (validated) |
| M40 change normalizer | `backend/internal/topology/normalizer.go` | FromPlan/FromAudit pure mapping; domain statuses normalized (succeeded/failed/expired/partial/awaiting_confirmation/executing → succeeded/failed/failed/partial/pending/pending); confidence high for platform+audit_id |
| M40 collector tests | `backend/internal/topology/collector_test.go` (ADR 0055) | 13 tests: each edge kind derivation, all-kinds integration, edge hash determinism, selector match helper |
| M40 normalizer tests | `backend/internal/topology/normalizer_test.go` (ADR 0055) | 11 tests: FromPlan (succeeded/pending/expired→failed/partial/default action/validation), FromAudit (succeeded/denied→failed), normalizePlanStatus table, HashSafeDiff, IngestChangeEvent validation/defaults |
| M40 service tests | `backend/internal/topology/service_test.go` | 5 tests: CollectNamespace (disabled/empty/success), GetTopologyGraph (empty/with edges), GetChangeTimeline |
| M40 migration | `backend/migrations/000029_topology_edges_and_change_events.up.sql` | topology_edges (partial unique active index, query indexes, CHECK constraints) + change_events (idempotent plan_id index, CHECK constraints); paired down migration |
| M40 HTTP routes | `backend/internal/httpserver/topology.go`; `router.go` | GET /api/v1/aiops/topology/graph + GET /api/v1/aiops/topology/changes; read-only; 503 when unconfigured, 400 on invalid query, 200 with bounded result |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 2 aiops/topology routes added; bidirectional route↔OpenAPI parity preserved |
| M40 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 73.46s; 29 backend packages (including `topology`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Background collection worker | — | Deferred — API ready, worker pending |
| Plan-completion ingestion hook | — | Deferred — API ready, hook pending |
| Real PostgreSQL integration | — | Deferred — requires full Compose stack |
| Real-kind E2E | — | Deferred — requires multi-worker kind cluster |
| Frontend UI | — | Deferred |
| Retention worker | — | Deferred — stale-edge closing and old-event pruning |

Authoritative records:

- `docs/changes/2026-07-31-m40-temporal-topology-and-change-intelligence.md`
- `docs/adr/0055-temporal-topology-and-change-intelligence.md`

## 2026-07-31 M41 SLO, Error Budget And Impact Addendum

This addendum records the M41 SLO closure. M41 introduces server-owned SLI
templates, versioned SLO definitions, deterministic evaluation with explicit
missing-data handling, and burn-alert transitions that feed the existing M27
alert lifecycle. The native M21-M31 signal path is unchanged; M41 is a
deterministic evaluator that reads from the M37 capability providers. No
public API contract was broken beyond the eight new `aiops/slos` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M41 data model | `backend/internal/slo/model.go` (ADR 0056) | 3 SLITemplate values (request_success_ratio/request_latency_target_ratio/workload_readiness), 2 MissingDataPolicy (unavailable/fail_open), 5 EvaluationState (healthy/burning_slow/burning_fast/breached/unavailable), 3 EvaluationCoverage (complete/partial/unavailable), Definition (versioned, bounded burn windows), Evaluation (append-only, deterministic) |
| M41 catalog | `backend/internal/slo/catalog.go` (ADR 0056) | TemplateDescriptor + compiled catalog map; ValidateDefinition single validation entry point; DefaultMissingDataPolicy returns unavailable even for workload_readiness (fail-open requires explicit opt-in) |
| M41 evaluator | `backend/internal/slo/evaluator.go` | Pure Evaluate: counter resets detected as monotonicity violations (counter→0); sparse data → CoveragePartial; no samples → CoverageUnavailable; clock boundaries inclusive window_start, exclusive window_end; classifyState precedence breached > burning_fast > burning_slow > healthy; zero error budget (objective==1.0) handled explicitly |
| M41 repository | `backend/internal/slo/repository.go` | GormRepository with ON CONFLICT DO NOTHING for idempotent evaluation inserts; partial unique index uq_slo_definitions_active for at-most-one-active-definition; NopRepository for testing/disabled mode |
| M41 service | `backend/internal/slo/service.go` | CreateDefinition (version=1), PatchDefinition (actor required, version increment), DeleteDefinition (enabled=false, row retained), EvaluateSLO (404 > 503 precedence, persists unavailable, emits BurnTransition only on state change, sink best-effort) |
| M41 evaluator tests | `backend/internal/slo/evaluator_test.go` (ADR 0056) | 14 tests: healthy path, breach, counter reset, missing-data fail-closed, missing-data fail-open for workload_readiness, fail-open rejection for request templates, nil source, source error, disabled definition, fast burn, slow burn, partial coverage, samples outside window, zero error budget |
| M41 service tests | `backend/internal/slo/service_test.go` | 13 tests: create success, create invalid input (6 subtests), patch increments version, patch requires actor, delete disables, delete not found, evaluate no evaluator, evaluate disabled, evaluate persists and emits transition, evaluate steady-state no transition, evaluate sink failure no rollback, list pagination, list limit clamping |
| M41 catalog tests | `backend/internal/slo/catalog_test.go` | 4 tests: ValidateDefinition (28 subtests), ValidateCreate requires creator/owner, LookupTemplate, AllTemplates, DefaultMissingDataPolicy |
| M41 HTTP handlers | `backend/internal/httpserver/slo_test.go` | 11 tests: list templates 200, list definitions 200, list invalid cluster_id 400, get not found 404, get invalid ID 400, create invalid body 400, evaluate 503, evaluate not found 404, list evaluations 200, list evaluations invalid version 400, delete 204, nil service 503 (7 subtests) |
| M41 migration | `backend/migrations/000030_slo_definitions_and_evaluations.up.sql` | slo_definitions (CHECK constraints on template/policy/objective/window/burn bounds, partial unique active index, query indexes) + slo_evaluations (CHECK constraints on state/coverage/window/event-count/ratio bounds, query indexes); paired down migration |
| M41 HTTP routes | `backend/internal/httpserver/slo.go`; `router.go` | 8 routes under /api/v1/aiops/slos: GET /templates, GET /, POST / (SystemOpsAdmin), GET /:id, PATCH /:id (SystemOpsAdmin), DELETE /:id (SystemOpsAdmin), POST /:id/evaluate (SystemOpsAdmin), GET /:id/evaluations |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 8 aiops/slos routes added; 10 schemas added (SLITemplateCatalog, SLITemplateDescriptor, SLOServiceRef, SLOActorRef, SLODefinition, SLODefinitionCreate, SLODefinitionPatch, SLODefinitionList, SLOEvaluation, SLOEvaluationList); bidirectional route↔OpenAPI parity preserved |
| M41 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 67.02s; 30 backend packages (including `slo` at 0.555s), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Real Prometheus/Loki integration | — | Deferred — requires running provider or recorded-fixture harness |
| Real-kind E2E (burn→M27) | — | Deferred — requires multi-worker kind cluster with metrics |
| Frontend SLO management UI | — | Deferred |
| Background evaluation worker | — | Deferred — API ready, worker pending |
| Multi-window burn rate | — | Deferred — data model stores window lengths; V1 uses single-window burn rate (conservative) |
| Production wiring in cmd/server/main.go | — | Deferred — routes registered and contract verified; production deployment requires constructing service with real repository and evaluator |

Authoritative records:

- `docs/changes/2026-07-31-m41-slo-error-budget-and-impact.md`
- `docs/adr/0056-slo-error-budget-and-impact.md`

## 2026-07-31 M42 Multi-Signal Correlation And Deterministic RCA Addendum

This addendum records the M42 correlation closure. M42 introduces a
deterministic, replayable multi-signal correlation engine that links M39
signal occurrences, M40 topology edges/change events and existing diagnosis
records into bounded cases. The diagnosis record remains the human
status/SLA/feedback source of truth; correlation cases are candidates, not
incidents. No public API contract was broken beyond the six new
`aiops/correlation` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M42 data model | `backend/internal/correlation/model.go` (ADR 0057) | 4 ConfidenceClass (confirmed/candidate/contradicted/unknown), 3 CaseStatus (active/resolved/stale), 4 SignalRelation (trigger/context/change/outcome), 4 ResourceRelation (primary/upstream/downstream/related), Case (deterministic case_key SHA-256), SignalLink, ResourceLink, ChangeCandidate, ActionCandidate (fixed M19 codes), CorrelationResult; CorrelationVersion=1.0 |
| M42 catalog | `backend/internal/correlation/catalog.go` (ADR 0057) | RuleDescriptor + compiled catalog map with 6 V1 rules (rollout_causes_pod_failure, rollout_causes_unavailable_deployment, rollout_causes_no_endpoints, maintenance_causes_node_failure, pvc_pending_causes_pod_failure, rollout_causes_metric_breach); fail-closed LookupRule; RulesForTriggerSignal |
| M42 engine | `backend/internal/correlation/engine.go` | Pure Correlate: explicit factors (same_uid/topology_distance/time_distance/change_symptom_rule/signal_freshness/signal_completeness/diagnosis_match/contradicting_signal); edgeIndex bidirectional BFS; classifyConfidence pure function; case_key dedup with merge |
| M42 golden fixtures | `backend/internal/correlation/fixtures.go` | 9 scenarios (ImagePull/CrashLoop/OOM/PVC-Pending/NoEndpoints/ReplicasUnavailable/NodeNotReady/MetricBreach/BadRollout-contradicted) + cold-start; deterministic (inputs, expected) pairs |
| M42 repository | `backend/internal/correlation/repository.go` | GormRepository with idempotent UpsertResult (ON CONFLICT DO NOTHING); unique indexes on case_key (active), (case_id, signal_occurrence_id, relation), (case_id, uid, relation), (case_id, change_event_id); NopRepository |
| M42 service | `backend/internal/correlation/service.go` | CorrelateNamespace (bounded lookback, idempotent persist), GetCase, ListCases, ListTimeline, GetCaseGraph, ListActionCandidates (deployment.rollback/deployment.rollout_restart — no execute endpoint) |
| M42 catalog tests | `backend/internal/correlation/catalog_test.go` (ADR 0057) | 5 tests: AllRules (no duplicates), LookupRule (known/unknown), RulesForTriggerSignal (known/unknown), CorrelationVersion non-empty, RequiredFactors non-empty |
| M42 fixtures tests | `backend/internal/correlation/fixtures_test.go` (ADR 0057) | 3 tests: TestGoldenFixtures (10 subtests covering 9 scenarios + cold-start), TestGoldenFixturesDeterminism (replay byte-identical case_keys), TestGoldenFixturesCaseKeyStability (stable across engine instances) |
| M42 service tests | `backend/internal/correlation/service_test.go` | 10 tests: CorrelateNamespace (fake provider + real engine), CorrelateNamespace (NopInputProvider), GetCase, ListCases, ListTimeline, GetCaseGraph, ListActionCandidates (not-found/rollback/pod-rollout-restart/service-backing) |
| M42 HTTP handlers | `backend/internal/httpserver/correlation_test.go` | 9 tests: list rules 200, list cases missing cluster_id 400, list cases 200, timeline 200, get case not-found 404, graph not-found 404, actions not-found 404, invalid id 400, service unavailable 503 |
| M42 migration | `backend/migrations/000031_diagnosis_correlation.up.sql` | correlation_cases + correlation_signal_links + correlation_resource_links + correlation_change_candidates (CHECK constraints, unique indexes); paired down migration |
| M42 HTTP routes | `backend/internal/httpserver/correlation.go`; `router.go` | 6 read-only routes under /api/v1/aiops/correlation: GET /rules, GET /cases, GET /cases/timeline, GET /cases/:id, GET /cases/:id/graph, GET /cases/:id/actions |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 6 aiops/correlation routes added; correlation tag + schemas added; bidirectional route↔OpenAPI parity preserved |
| M42 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 63.26s; 31 backend packages (including `correlation`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Background correlation worker | — | Deferred — API ready, worker pending |
| Signal-ingestion hook | — | Deferred — trigger correlation on new signal occurrence |
| Real PostgreSQL integration test | — | Deferred — requires full Compose stack |
| Real-kind E2E | — | Deferred — requires multi-worker kind cluster |
| Frontend correlation UI | — | Deferred |
| M43 AI investigator integration | — | Deferred |
| M44 safe automation integration | — | Deferred |

Authoritative records:

- `docs/changes/2026-07-31-m42-multi-signal-correlation-and-deterministic-rca.md`
- `docs/adr/0057-multi-signal-correlation-and-deterministic-rca.md`

## 2026-07-31 M43 Cited And Evaluated AI Investigator Addendum

This addendum records the M43 AI investigator closure. M43 introduces a
cited and evaluated AI investigator bound to M42 correlation cases. The
investigation is a read-only advisory: it never modifies the case,
diagnosis or alert. Every factual claim cites an authorized evidence ID;
fabricated, out-of-scope or unauthorized citations reject the entire
output. The model cannot upgrade a candidate to confirmed cause. On
provider failure, budget exhaustion or citation rejection, a failed
investigation is persisted with `failure_reason` set so deterministic
investigation remains available. No public API contract was broken beyond
the four new `aiops/investigator` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M43 data model | `backend/internal/aiinvestigator/model.go` (ADR 0058) | InvestigatorVersion=1.0; 3 InvestigationStatus (completed/failed/stale); 3 HypothesisConfidence (high/medium/low); 7 EvidenceKind (signal_occurrence/topology_edge/change_event/diagnosis_record/slo_evaluation/correlation_case/change_candidate); Investigation (deterministic investigation_key SHA-256 over case_id+investigator_version+prompt_hash); Hypothesis (claim/confidence/evidence_ids/disconfirming_evidence/next_checks); Citation; EvidenceRef; Prompt; ProviderResult; bound constants (MaxHypotheses=8, MaxCitations=64, MaxUncertainties=16) |
| M43 runbook catalog | `backend/internal/aiinvestigator/catalog.go` (ADR 0058) | RunbookDescriptor + compiled catalog map with 4 V1 runbooks (rollback_last_rollout=deployment.rollback, rollout_restart_pods=deployment.rollout_restart, inspect_pvc_capacity=advisory, inspect_node_maintenance=advisory); fail-closed LookupRunbook; EligibleRunbooks (advisory always eligible); ValidateRunbookEligibility |
| M43 prompt + validator | `backend/internal/aiinvestigator/prompt.go`; `provider.go` (ADR 0058) | BuildPrompt (system prompt with role/schema/citation/runbook/prohibition/injection-defense + user prompt with redacted authorized evidence only); buildAuthorizedEvidence (case + signals + change candidates); ValidateProviderResult 8 rules (non-empty summary/impact, 1..8 hypotheses, authorized citations, authorized hypothesis/disconfirming evidence, 1..64 citations, eligible runbook, no "confirm root cause" claims, bounded next_checks/uncertainties); total rejection; PromptHash stable SHA-256 |
| M43 golden fixtures | `backend/internal/aiinvestigator/fixtures.go` | 10 validation scenarios (correct_cited_investigation, insufficient_evidence, conflicting_evidence, prompt_injection_rejected, hidden_scope_citation_rejected, fabricated_citation_rejected, ineligible_runbook_rejected, confirm_root_claim_rejected, empty_summary_rejected, no_citations_rejected); deterministic (provider result, authorized evidence, eligible codes, expected valid/invalid + failure substring) pairs |
| M43 repository | `backend/internal/aiinvestigator/repository.go` | GormRepository with Save (insert + MarkStale), Get, ListByCase, ListByFilter, MarkStale; JSONB wrapper; partial unique index uq_ai_investigations_active on (case_id, investigation_key) WHERE status != 'stale'; NopRepository |
| M43 service | `backend/internal/aiinvestigator/service.go` | Investigate (read case + eligible codes, build prompt, call provider, validate, persist completed/failed); GetInvestigation; ListByCase; ListRunbooks; CaseReader interface; NopCaseReader; On provider failure → failed/provider_error; on validation failure → failed/citation_rejected (provider summary retained); computeInvestigationKey deterministic |
| M43 catalog tests | `backend/internal/aiinvestigator/catalog_test.go` (ADR 0058) | 5 tests: LookupRunbook (known/unknown/empty), AllRunbooks (self-consistency), EligibleRunbooks (advisory always/gated/both codes), ValidateRunbookEligibility (advisory/eligible/absent/unknown/empty) |
| M43 provider/fixtures tests | `backend/internal/aiinvestigator/provider_test.go` (ADR 0058) | 4 tests: TestGoldenFixtures (10 subtests), TestGoldenFixturesCoverage (all 10 required scenarios), TestValidateProviderResultEdgeCases (8 subtests: empty impact/no hypotheses/invalid confidence/no evidence/empty claim/too many hypotheses/authorized+unauthorized disconfirming), TestNopProvider (valid/no-case-evidence), TestDecodeProviderJSON (valid/malformed/trim) |
| M43 prompt tests | `backend/internal/aiinvestigator/prompt_test.go` | 8 tests: BuildPrompt (evidence authorization), SystemContainsRunbooks, UserContainsCaseFacts, NoEligibleRunbooks, PromptHashStability, PromptHashChangesWithEvidence, PromptHashIgnoresFactorChanges, BuildAuthorizedEvidence (dedup/empty), MarshalEvidenceForHash (determinism) |
| M43 service tests | `backend/internal/aiinvestigator/service_test.go` | 15 tests: InvestigateSuccess, CaseNotFound, ProviderFailurePersistsFailed, CitationRejectionPersistsFailed, IneligibleRunbookPersistsFailed, Disabled, NopProviderProducesValidResult, InvestigationKeyDeterministic, GetInvestigation (found/not-found), ListByCase (truncated/not-truncated), ListRunbooks, ComputeInvestigationKey, NewServiceDefaults, NopCaseReader, NopRepository, EligibleActionCodesError, NilEligibleCodesTreatedAsEmpty |
| M43 HTTP handlers | `backend/internal/httpserver/aiinvestigator_test.go` | 12 tests: list runbooks 200, list investigations invalid case_id 400, negative case_id 400, bad limit 400, list 200, get invalid id 400, get not-found 404, generate invalid case_id 400, generate case not-found 404, generate success 200, generate provider-failure 404, generate preserves actor from context |
| M43 migration | `backend/migrations/000032_aiinvestigator.up.sql`; `000032_aiinvestigator.down.sql` | ai_investigations table (CHECK constraints on status/tokens, completed-summary/completed-citations/failed-reason invariants; partial unique index uq_ai_investigations_active; FK to correlation_cases ON DELETE CASCADE); paired down migration |
| M43 HTTP routes | `backend/internal/httpserver/aiinvestigator.go`; `router.go` | 4 routes under /api/v1/aiops/investigator: GET /runbooks, GET /cases/:case_id/investigations, GET /investigations/:id, POST /cases/:case_id/investigations (POST is the only write; actor from session) |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 4 aiops/investigator routes added; aiinvestigator tag + 8 schemas (InvestigatorRunbookList, InvestigatorRunbook, InvestigationListResponse, Investigation, InvestigationActor, InvestigationHypothesis, InvestigationCitation, EvidenceRef) added; bidirectional route↔OpenAPI parity preserved |
| M43 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 37.47s; 31 backend packages (including `aiinvestigator`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Real AI provider integration | — | Deferred — Responses-compatible HTTP provider wiring pending |
| Provider budget/reservation | — | Deferred — mirror aiexplain daily token budget |
| Real PostgreSQL integration test | — | Deferred — requires full Compose stack |
| Real-kind E2E | — | Deferred — requires multi-worker kind cluster |
| Frontend investigator UI | — | Deferred |
| M44 safe automation wiring | — | Deferred — preview/confirm/execute eligible runbook via M19 paths |

Authoritative records:

- `docs/changes/2026-07-31-m43-cited-and-evaluated-ai-investigator.md`
- `docs/adr/0058-cited-and-evaluated-ai-investigator.md`

## 2026-07-31 M44 Policy-Constrained Automation And Post-Action Verification Addendum

This addendum records the M44 safe-automation closure. M44 closes the AIOps
loop: an eligible M43 runbook is materialized into an action plan, gated
through deterministic policy checks, approved by a human (L2 default;
four-eyes for rollback/image_update), executed idempotently against the
Kubernetes source, and verified against captured pre/post evidence. The
action plan lifecycle is `draft → previewed → approved → executing →
succeeded/failed → verified` (plus terminal `expired`/`cancelled`). Policy
gates are rechecked immediately before execute — stale UID/RV, opened freeze
window, exhausted PDB budget, or exceeded attempt cap all fail closed.
Post-action verification compares SLO and resource state deterministically;
missing evidence never resolves a diagnosis automatically (yields `unknown`).
When verification yields ineffective/failed, a server-owned rollback
contract drafts a rollback plan when safe, or escalates to a human. No public
API contract was broken beyond the ten new `aiops/automation` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M44 data model | `backend/internal/automation/model.go` (ADR 0059) | AutomationVersion=1.0; VerifierVersion=1.0; 4 AutomationLevel (L0/L1/L2/L3; L2 default, L3 not enabled); 9 PlanStatus (draft/previewed/approved/executing/succeeded/failed/expired/cancelled/verified); 2 ApprovalType (single/four_eyes); 3 GateStatus (passed/failed/skipped); 8 GateCode (uid_rv_recheck/scope/pdb_blast_radius/slo_burn/freeze_window/concurrent_plans/attempt_cap/rollback_point); ActionPlan (deterministic plan_key SHA-256 over case_id+runbook_id+target_uid+automation_version); ActionVerification (deterministic verification_key SHA-256 over plan_id+verifier_version+evidence_hash); EvidenceSnapshot; SLOSnapshot; bound constants (MaxAttemptsPerTarget=5, AttemptWindowSeconds=3600, DefaultPlanTTLSeconds=600, DefaultClaimTTLSeconds=60, DefaultCooldownSeconds=300, MinCooldownSeconds=60) |
| M44 sentinel errors | `backend/internal/automation/errors.go`; `repository.go`; `service.go` | 24 sentinel errors: ErrInvalidRunbook/ErrRunbookNotInCatalog/ErrAdvisoryRunbookNotExecutable/ErrRunbookNotEligible/ErrNoRollbackPoint/ErrUnsupportedAction/ErrUnsupportedTargetKind/ErrInvalidOperation/ErrOperationNoChange/ErrTargetChanged/ErrNotDraft/ErrNotPreviewed/ErrSelfApprovalForbidden/ErrPolicyGateFailed/ErrInvalidIdempotency/ErrExecutionFailed/ErrNotVerifiable/ErrCaseNotFound/ErrDisabled/ErrPlanNotFound/ErrVerificationNotFound/ErrConfirmationInvalid/ErrExpired/ErrInProgress/ErrAlreadyExecuted/ErrNotApproved |
| M44 runbook catalog | `backend/internal/automation/catalog.go` (ADR 0059) | RunbookDescriptor + compiled catalog map with 2 V1 executable runbooks (rollback_last_rollout=deployment.rollback/four_eyes, rollout_restart_pods=deployment.rollout_restart/single); fail-closed LookupRunbook; AllRunbooks; mirrors M43 catalog but only executable (advisory-only runbooks cannot be materialized) |
| M44 policy gate evaluator | `backend/internal/automation/gates.go` (ADR 0059) | GateContext (Now/PreviewSnapshot/CurrentSnapshot/ScopeDecision/PDBEvidence/BlastRadius/SLOBurnState/FreezeWindow/ConcurrentPlanCount/AttemptCount/AttemptMax/RollbackPoint); GateEvaluator stateless+pure; RequiredGates(actionCode) returns action-specific gate set (core: uid_rv_recheck/scope/freeze_window/concurrent_plans/attempt_cap; Pod-affecting add pdb_blast_radius; SLO-bound add slo_burn; rollback adds rollback_point); Evaluate (preview); Recheck (execute, Rechecked=true); AllPassed (skipped is non-failure); FailedGates; 8 per-gate evaluators with fail-closed semantics |
| M44 repository | `backend/internal/automation/repository.go` | GormRepository with SavePlan/GetPlan/GetPlanForExecute (row lock)/ListPlans/CountAttemptsSince/CountConcurrentPlans/MarkPreviewed/Approve/Claim (idempotent, row-lock, stale reclaimable)/Complete/Fail/MarkVerified/Cancel/ExpireStale/SaveVerification/GetVerification/GetVerificationByPlan/UpdateVerification; JSONB wrapper; partial unique index uq_action_plans_active (one non-terminal plan per plan_key); partial unique index uq_action_verifications_active (one pending verification per plan); NopRepository |
| M44 service | `backend/internal/automation/service.go` | CreatePlan (validate runbook+eligibility, materialize parameters, capture target snapshot, compute plan_key, issue confirmation token 32B base64 hashed at rest, persist draft); Preview (refresh snapshot, evaluate gates, store results, transition to previewed); Approve (enforce four-eyes distinctness); Execute (recheck gates, idempotent claim, build+apply patch, transition succeeded/failed, schedule verification); Verify (run verifier, evaluate rollback contract on ineffective/failed, mark verified); Cancel; ListPlans; GetPlan; GetVerification; CaseReader/NopCaseReader; KubernetesSource interface; ServiceOption (WithNow/WithPlanTTL/WithClaimTTL/WithCooldown/WithEvidenceProvider) |
| M44 post-action verifier | `backend/internal/automation/verifier.go` (ADR 0059) | EvidenceProvider interface (CapturePreSnapshot/CapturePostSnapshot); NopEvidenceProvider; Verifier pure given (plan, pre, post); VerifierOption (WithVerifierProvider/WithVerifierNow/WithVerifierCooldown); CreateVerification (capture pre at execute time, compute verification_key); Evaluate (capture post after cooldown, compareEvidence, classifyStatus); compareEvidence deterministic (SLO transitions take precedence: healthy>burning_slow>burning_fast>breached; resource state compared for non-SLO or unchanged SLO; missing → ComparisonInsufficient); classifyStatus maps to effective/ineffective/failed/unknown; helpers sloStateRank/sloBoundAction/resourceInt/resourceStr/resourceBool/hashSnapshot/computeVerificationKey |
| M44 gates tests | `backend/internal/automation/gates_test.go` (ADR 0059) | 11 tests: TestRequiredGates (6 subtests per action), TestEvaluateUIDRV (5 subtests: missing preview/target gone/UID changed/RV changed/match), TestEvaluateScope (allowed/denied/empty), TestEvaluateFreezeWindow (active/inactive), TestEvaluateConcurrentPlans (0/1+), TestEvaluateAttemptCap (under/over/default), TestEvaluateRollbackPoint (no revision/current/valid), TestEvaluateSLOBurn (breached+rollback/breached+image_update/burning_fast/healthy/unavailable), TestEvaluatePDBBlastRadius (unavailable/exceeds cap/negative allowed/within), TestAllPassed (all passed/one failed/skipped ok), TestRecheck (stamps Rechecked=true, preserves order) |
| M44 verifier tests | `backend/internal/automation/verifier_test.go` | 17 tests: TestCreateVerification (success/clamp cooldown/propagate error), TestEvaluatePostSnapshotCaptureFailed (yields failed+missing_evidence), TestCompareEvidenceSLOImproved, TestCompareEvidenceSLOWorse, TestCompareEvidenceMissingPre, TestCompareEvidenceMissingPost, TestCompareEvidenceResourceScaleImproved, TestCompareEvidenceResourceScaleUnchanged, TestCompareEvidenceRolloutRestartImproved, TestCompareEvidenceRolloutRestartUnchangedWhenPodsNotReady, TestClassifyStatus (improved→effective/worse→ineffective/insufficient→unknown/missing→unknown), TestHashSnapshot (determinism), TestSloStateRank (order), TestSloBoundAction (deployment.* SLO-bound, cronjob not), TestVerifierEvaluateEndToEndMissingEvidenceIsUnknown |
| M44 service tests | `backend/internal/automation/service_test.go` | 17 tests: CreatePlanRejectsEmptyRunbook, CreatePlanRejectsUnknownRunbook, CreatePlanRejectsAdvisoryRunbook, CreatePlanRejectsIneligibleRunbook, CreatePlanRejectsWhenCaseNotFound, ApproveRejectsNonPreviewed, ApproveRejectsSelfApprovalFourEyes, ApproveAcceptsDifferentApproverFourEyes, ExecuteRejectsEmptyConfirmationToken, ExecuteRejectsInvalidIdempotencyKey, VerifyRejectsNonVerifiablePlan, CancelDisabledService, ApprovalTypeFor (rollback→four_eyes, others→single), ComputePlanKey (determinism+version bump), ListPlansReturnsResponseWithTruncated, ListPlansNotTruncatedWhenItemsEqualTotal, ListPlansNotTruncatedWhenEmpty; uses fakeRepository (in-memory), fakeCaseReader, stubKubernetesSource |
| M44 HTTP handlers | `backend/internal/httpserver/automation_test.go` | 21 tests: list runbooks 200/503, list plans 200/invalid limit/invalid case_id, create plan missing fields/unknown runbook/case not-found/ineligible runbook, get plan invalid id/404, preview invalid id/404, approve invalid id, execute missing confirmation, cancel 404, verify 404, get verification 404, writeError maps 25 sentinel errors (25 subtests), isValidPlanID (valid/invalid), buildChangePreview (scale/rollback/image_update/suspend) |
| M44 migration | `backend/migrations/000033_policy_constrained_automation.up.sql`; `000033_policy_constrained_automation.down.sql` | action_plans + action_verifications tables (CHECK constraints on status/approval_type/evidence_comparison/verification_status; four-eyes distinctness CHECK; missing-evidence→insufficient+unknown CHECK; partial unique index uq_action_plans_active; partial unique index uq_action_verifications_active; FKs to correlation_cases(id) and ai_investigations(id) ON DELETE SET NULL); paired down migration |
| M44 HTTP routes | `backend/internal/httpserver/automation.go`; `router.go` | 10 routes under /api/v1/aiops/automation: GET /runbooks, GET /plans, POST /plans, GET /plans/:plan_id, POST /plans/:plan_id/preview, POST /plans/:plan_id/approve, POST /plans/:plan_id/execute, POST /plans/:plan_id/cancel, POST /plans/:plan_id/verify, GET /plans/:plan_id/verification (write routes require rolesSystemOpsAdmin; read routes require auth only; actor from session; Idempotency-Key header read by execute) |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 9 aiops/automation paths added; automation tag + 9 schemas (AutomationRunbookList, AutomationRunbook, CreateActionPlanRequest, ApproveActionPlanRequest, ExecuteActionPlanRequest, ActionPlanListResponse, ActionPlanResponse, ActionVerification, PolicyGate) added; bidirectional route↔OpenAPI parity preserved |
| M44 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 67.17s; 30 backend packages (including `automation`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Background verification worker | — | Deferred — cooldown-based scheduling of Verifier.Evaluate |
| Stale executing reclaim worker | — | Deferred — auto-reclaim after claimTTL |
| ExpireStale background worker | — | Deferred — TTL-based expiration of awaiting plans |
| Real Kubernetes integration tests | — | Deferred — PatchDeployment/PatchCronJob/RolloutHistory via client-go |
| Real Prometheus/SLO integration | — | Deferred — EvidenceProvider reads from SLO service + k8s source |
| Real PostgreSQL integration test | — | Deferred — requires full Compose stack |
| Real-kind E2E | — | Deferred — preview → approve → execute → verify path |
| Frontend automation UI | — | Deferred — plan list, plan detail with gate timeline, verification panel |
| L3 pre-authorized automatic execution | — | Deferred — requires separate ADR with shadow mode/narrow policy/canary/kill switch |
| Rollback-plan auto-execution | — | Deferred — M44 drafts the rollback plan; auto-execute requires stricter gates |
| M42 ActionCandidate → M44 plan auto-suggestion | — | Deferred — operator picks runbook by ID today |

Authoritative records:

- `docs/changes/2026-07-31-m44-policy-constrained-automation-and-post-action-verification.md`
- `docs/adr/0059-policy-constrained-automation-and-post-action-verification.md`

## 2026-07-31 M45 Versioned AIOps Golden Dataset And Quality Report Addendum

This addendum records the M45 golden dataset closure. M45 introduces the
versioned AIOps golden dataset and quality report as the replayable contract
for the full AIOps loop (M39-M44). The dataset contains 3 scenarios: the
mandatory 10-step end-to-end golden scenario plus 2 negative companions
(misattribution prevention, partial/unknown fail-closed). The quality report
structure records before/after comparison per scenario, aggregated summary
metrics, changed components, and human review state. M45 production gates
(hosted CI, production OIDC/MFA, HA PostgreSQL, signed releases, real-kind
E2E) remain external.

| Gate | Evidence | Accepted result |
|---|---|---|
| M45 golden dataset | `backend/internal/golden/model.go` (ADR 0060) | DatasetVersion=1.0; ScenarioVersion=1.0; 10 StepID constants (establish_healthy_service/publish_bad_image/capture_signals/build_impact_graph/rank_cause_candidate/generate_investigation/preview_approve_rollback/execute_verify/recover_alert/cleanup); AllSteps ordered list; 3 ScenarioID constants (mandatory_end_to_end/negative_misattribution/negative_partial_evidence); StepOutcome with expected signal/topology/SLO/correlation/investigation/action plan/verification/alert recovery flags; Scenario (ID/Version/Description/Steps/Negative); Dataset (Version/Scenarios); DefaultDataset() returning 3 scenarios |
| M45 mandatory 10-step scenario | `backend/internal/golden/model.go` | Maps each step to expected outcome: establish_healthy_service (M41 healthy), publish_bad_image (M23 release), capture_signals (M39+M41), build_impact_graph (M40 topology), rank_cause_candidate (M42 correlation), generate_investigation (M43 cited AI), preview_approve_rollback (M44 approved), execute_verify (M44 verified+effective), recover_alert (M27 lifecycle), cleanup |
| M45 negative misattribution | `backend/internal/golden/model.go` | Unrelated simultaneous change in another Namespace must NOT be attributed to primary case; expects correlation case but does NOT expect action plan (unrelated change does not trigger automation) |
| M45 negative partial evidence | `backend/internal/golden/model.go` | When one metrics/log provider is stopped, case must be partial/unknown not falsely healthy; expects valid advisory investigation (with uncertainty) but does NOT expect alert recovery (partial evidence does not resolve alert); preserves M41 fail-closed invariant |
| M45 quality report | `backend/internal/golden/quality.go` (ADR 0060) | QualityReport (ReportVersion/DatasetVersionBefore/After/EngineVersionsBefore/After/ScenarioResults/Summary/GeneratedAt/ChangedComponents/Reviewer/Approved); EngineVersions tracking M39-M44 (Signal/Topology/SLO/Correlation/Investigator/Automation/Verifier); ScenarioQuality (ScenarioID/PassedBefore/PassedAfter/Delta/StepsPassedBefore/After/Total/Notes); QualitySummary (TotalScenarios/PassedBefore/After/Improved/Regressed/Preserved/Unchanged/TotalStepsBefore/After/Total); ClassifyDelta (preserved/improved/regressed/unchanged); Summarize; JSON-serializable; generated offline; never self-modifies rules/prompts/policy online |
| M45 golden tests | `backend/internal/golden/model_test.go` (ADR 0060) | 9 tests: TestDatasetVersion (version=1.0), TestDefaultDatasetIntegrity (3 scenarios/unique IDs/10 steps in order/negatives marked), TestMandatoryScenarioStepCoverage (exercises signal+topology+SLO+correlation+investigation+action plan+verification+alert recovery), TestNegativeMisattributionScenario (case expected, no action plan), TestNegativePartialEvidenceScenario (valid advisory investigation, no alert recovery), TestDatasetDeterminism (same scenarios on every call), TestClassifyDelta (4 cases), TestSummarize (aggregation math), TestQualityReportEndToEnd (3 scenarios with 1 regression) |
| M45 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 64s; 31 backend packages (including `golden`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Hosted CI with Linux race + real-kind matrix | — | Deferred — external production gate |
| Production OIDC/MFA + break-glass evidence | — | Deferred — external production gate |
| Multi-replica deployment with PDB/topology spread | — | Deferred — external production gate |
| External HA PostgreSQL with WAL/PITR + RPO/RTO | — | Deferred — external production gate |
| Multi-instance no-duplicate-business-effect | — | Deferred — external production gate |
| Signed multi-arch release with SBOM/provenance | — | Deferred — external production gate |
| Real-kind E2E for full 10-step scenario | — | Deferred — requires multi-worker kind cluster |
| Real Prometheus/Loki/AI-provider replay in CI | — | Deferred — requires real provider credentials |
| Frontend quality dashboard | — | Deferred — UI for quality report visualization |
| CI quality report generation on every PR | — | Deferred — CI integration to block regressions |

Authoritative records:

- `docs/changes/2026-07-31-m45-versioned-aiops-golden-dataset-and-quality-report.md`
- `docs/adr/0060-versioned-aiops-golden-dataset-and-quality-report.md`

## 2026-07-31 M39 Unified Signal Model Addendum

This addendum records the M39 signal model closure. M39 normalizes existing
M21-M31 outputs into a unified `signal_occurrences` table. The native M21-M31
signal path is unchanged; M39 is an evidence normalizer. No public API contract
was broken beyond the new `aiops` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M39 signal model | `backend/internal/signal/model.go` (ADR 0054) | Occurrence envelope with signal_id/producer/cluster_id/namespace/resource citation (kind/name/UID/incomplete)/severity/state/fingerprint/coverage/freshness/window/observed_at/ingested_at/expires_at/attributes/evidence refs |
| M39 catalog | `backend/internal/signal/catalog.go` | 28 SignalDescriptors across 7 domains (workload/node/network/storage/governance/change/metric); fail-closed Lookup; MapSeverity with fallback |
| M39 fingerprint dedup | `backend/internal/signal/repository.go` `ComputeFingerprint` | SHA256 over identity fields excluding ObservedAt; unique DB index + ON CONFLICT DO UPDATE |
| M39 normalizers | `backend/internal/signal/normalizer_test.go` (ADR 0054) | 12 tests: diagnosis (mapped/unmapped/resolved), alert (firing/resolved), metric (firing/non-firing), posture (mapped/unmapped), change (succeeded/failed/pending) |
| M39 service | `backend/internal/signal/service_test.go` (ADR 0054) | 10 tests: fingerprint stability across redelivery, fail-closed for unregistered, incomplete UID marking, retention expiry, severity fallback, catalog invariants, dedup, list clamping, overview partial flag, cleanup |
| M39 HTTP handlers | `backend/internal/httpserver/signal_test.go` | 9 tests: overview 200/503/400, signals list 200/400, catalog 200, cluster scope, ingest integration |
| M39 migration | `backend/migrations/000028_signal_occurrences.up.sql` | signal_occurrences table with unique (signal_id, fingerprint) index, query indexes, CHECK constraints; paired down migration |
| M39 configuration | `backend/internal/config/config.go` | SignalConfig (Enabled, RetentionBatch, ListLimit, OverviewTopN, OverviewWindow, CleanupInterval); disabled by default; fail-closed validation |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 3 aiops routes added; bidirectional route↔OpenAPI parity preserved |
| M39 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 59.59s; 28 backend packages (including `signal`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Real PostgreSQL integration | — | Deferred — requires full Compose stack |
| SourceReader adapter | — | Deferred — M40 temporal topology |
| Batch ingestion worker | — | Deferred — API ready, worker pending |
| Frontend UI | — | Deferred |

Authoritative records:

- `docs/changes/2026-07-31-m39-unified-signal-model.md`
- `docs/adr/0054-unified-service-identity-and-signal-model.md`

## 2026-07-31 M37 Capability Plane Adapters Addendum

This addendum records the capability adapter and alert-route closure. All
adapters are disabled by default; the server runs identically to the current
deployment when no provider is configured. No public API contract was broken
beyond the new `capability` and `alert-routes` routes.

| Gate | Evidence | Accepted result |
|---|---|---|
| M37A provider contracts | `backend/internal/capability/provider_test.go` (ADR 0053) | 18 tests: fixed-template query validation (template, cluster, namespace, time range, step, limit, direction), Nop provider returns `StateUnavailable`, normalized result shape with `coverage`/`state`/`freshness` |
| M37A HTTP handlers | `backend/internal/httpserver/capability_test.go` | 8 tests: 503 when provider unset, 400 on missing/invalid inputs, 200 on valid queries, masked-error invariant |
| M37B service | `backend/internal/alertroute/service_test.go` (ADR 0053) | 40 tests: route matching, dedupe-key rendering, silence enforcement (permanent forbidden, max 7d, reason required), receiver CRUD with in-use guard, route CRUD with ownership, delivery lifecycle (pending→delivering→delivered/dead), retry backoff, batch claiming, webhook dispatch (success/4xx/5xx/timeout), masked-URL invariant |
| M37B HTTP handlers | `backend/internal/httpserver/alertroute_test.go` | 27 tests: role matrix (SystemOpsAdmin mutations, SystemSecurityAudit deliveries), ownership (404 for other users), receiver in-use conflict, route priority bounds, silence duration bounds, delivery listing filter, masked-URL invariant |
| M37B migration | `backend/migrations/000027_alert_routes_and_silences.up.sql` | Four tables (`alert_route_receivers`, `alert_routes`, `alert_silences`, `alert_route_deliveries`); priority CHECK 1..100; silence CHECK `ends_at > starts_at` and duration ≤ 7d; partial indexes on enabled routes and active deliveries; paired down migration |
| M37 configuration | `backend/internal/config/config.go` | `CapabilityConfig` and `AlertRouteConfig` with fail-closed validation (HTTPS endpoints, bounded timeouts); both disabled by default |
| OpenAPI ↔ Router parity | `docs/api/openapi.yaml`; `TestRegisteredRoutesMatchOpenAPI` | 2 capability routes + 10 alert-route routes added; bidirectional route↔OpenAPI parity preserved; fixed two YAML parsing errors (unquoted colons in `createAlertRoute`/`createAlertSilence` descriptions) |
| M37 fast gate | `scripts/verify-fast.ps1 -Scope All` | Passed in 51.06s; 27 backend packages (including `capability` and `alertroute`), 81 frontend tests/18 files, Compose/Kustomize contracts |
| Real Prometheus/Loki provider | — | Deferred — requires external provider access |
| Real-kind alert-route E2E | — | Deferred — requires multi-worker kind cluster with synthetic webhook receiver |

Authoritative records:

- `docs/changes/2026-07-31-m37-capability-plane-adapters.md`
- `docs/adr/0053-capability-plane-adapters.md`

## 2026-07-31 M38 Engineering, Delivery And Supply-Chain Hardening Addendum

This addendum records the engineering/delivery/supply-chain hardening closure.
It does not change any public API contract.

| Gate | Evidence | Accepted result |
|---|---|---|
| M38 fast gate (backend) | `scripts/verify-fast.ps1 -Scope Backend` | Passed in 36.5s |
| M38 fast gate (manifests) | `scripts/verify-fast.ps1 -Scope Manifests` | Passed in 2.47s |
| CI completeness (M38A) | `.github/workflows/ci.yml`; `backend/internal/deployment/ci_workflows_test.go` (ADR 0051) | Pull-request gate now requires `go test -race -p=1 -count=1 ./...`, `golangci-lint@v2.12.2` with `.golangci.yml`, `pnpm lint` (ESLint flat config), 50.0% coverage baseline and `oasdiff breaking --fail-on ERR`; real-kind E2E workflow covers M23-M31 |
| Helm chart (M38B) | `deploy/helm/aiops-platform/`; `backend/internal/deployment/helm_chart_test.go` | Official Helm 3 chart with `Chart.yaml`, `values.yaml`, `values.schema.json` and 9 templates; 10 contract tests guard structure, values, schema, required templates, security baseline and the "never render a Secret" rule |
| Supply chain (M38C) | `.github/workflows/release.yml`; `docs/security/license-allowlist.json`; `backend/internal/deployment/license_allowlist_test.go` | Releases build `linux/amd64` + `linux/arm64` OCI images via `docker buildx`/QEMU, generate SPDX SBOMs with `syft v1.27.0`, and bundle the Helm chart, license allowlist and SHA256 manifest; allowlist admits `MIT`/`ISC`/`BSD-2-Clause`/`BSD-3-Clause`/`Apache-2.0` only; 2 contract tests guard the allowlist |
| Delivery assets | `SECURITY.md`; `CHANGELOG.md` | Tracked delivery assets; ADR 0051 records the seven decisions |
| Real-kind E2E | — | Deferred — cosign image signing, `helm lint` in CI and the Helm upgrade/rollback matrix require authorized hosted CI |

Authoritative records:

- `docs/changes/2026-07-31-m38-engineering-delivery-and-supply-chain-hardening.md`
- `docs/adr/0051-engineering-delivery-and-supply-chain-hardening.md`

更新时间：2026-07-27

测试结论以本轮命令输出和 `.artifacts/` 中的脱敏证据为准。未实际执行的环境测试不得标记为通过。

## 自动化层级

| 层级 | 范围 | 主要入口 | 当前覆盖 |
|---|---|---|---|
| Backend 单元/集成 | 领域规则、仓储事务、HTTP/RBAC、OpenAPI、部署合同、fleet fan-out、全局搜索与私有保存筛选器 | `go vet ./...`、`go test -p=1 -count=1 ./...` | 152 个 Go `Test*` 入口，包含 PostgreSQL、httptest、Kubernetes fake/loopback 测试 |
| Frontend 单元 | API 参数序列化、认证、集群、资源健康、资源指标、拓扑、事件、诊断、审计、通知、受控操作、fleet、全局搜索与保存筛选器 | `pnpm typecheck`、`pnpm test -- --run` | 14 个 Vitest 文件，当前基线 59 个用例 |
| Build | Go 二进制、Vue 生产资源、Docker 镜像 | `scripts/verify.ps1` | Go 1.25 容器构建、Vite build、Compose build |
| Manifest contract | 平台部署、目标集群 RBAC、演示与独立诊断清单 | `kubectl kustomize` + `backend/internal/deployment` | Secret 分离、探针、资源限制、NetworkPolicy、最小 RBAC、演示和合成故障场景 |
| Real kind E2E | 真实 Kubernetes API、短期凭据、7 条保留环境诊断、2 条独立诊断、处置和权限边界 | `scripts/e2e-kind.ps1`、`scripts/e2e-diagnosis-kind.ps1` | `kind-aiops-test` + 一次性 kind / Kubernetes v1.34.0 |

## 关键需求追踪

| 需求 | Backend | Frontend | Real kind | 通过标准 |
|---|---|---|---|---|
| 登录与当前角色生效 | auth service/router tests | `auth.test.ts` | API 登录 | 停用、改角色或改密后旧会话立即失效 |
| 集群凭据安全 | credential/cluster repository tests | `clusters.test.ts` | 短期 ServiceAccount token 接入 | 明文不落库、不回显，探测返回 Ready Conditions |
| 多集群只读资源 | Kubernetes gateway/sanitization tests | `kubernetes.test.ts`、`kubernetes-events.test.ts` | 17 类列表/详情与 Event 查询 | 所有请求显式绑定 `cluster_id`；ConfigMap/Secret 值、StorageClass 参数和 Secret labels/annotations 不回显；Event 精确匹配资源 |
| 常用工作负载与策略 | 九类固定路由、公开模型、空集合和 RBAC tests | `kubernetes.test.ts`、`kubernetes-events.test.ts` | 九类真实 fixture 与 list/detail 深链 | StatefulSet/DaemonSet/ReplicaSet/Job/CronJob/HPA/ResourceQuota/LimitRange/Secret 均可读；Secret 只返回 key 名；无任意 GVK/写代理 |
| 真实资源指标 | fixed Metrics contracts/error tests | `resource-metrics.test.ts`、`kubernetes.test.ts` | Metrics API 缺失/可用 + SubjectAccessReview | unavailable path 保持 424；available path Node/Pod Metrics 非空；利用率仅使用同名 Node allocatable；Pod 排行有界且显示覆盖率；get/list 允许而 create 拒绝 |
| ImagePullBackOff | rule and diagnosis tests | `diagnosis.test.ts` | 故障 Pod | 命中 `pod.image_pull_backoff.v1` 并保存证据 |
| CrashLoopBackOff | rule and previous-state tests | `diagnosis.test.ts` | 故障 Pod | 命中 `pod.crash_loop_backoff.v1` 且包含上次终止状态 |
| Pod Pending | scheduling condition/event rule tests | `diagnosis.test.ts` | Pending Pod | 命中 `pod.pending.v1` 并保存 PodScheduled/FailedScheduling 证据 |
| Pod OOMKilled | termination state rule tests | `diagnosis.test.ts` | OOMKilled Pod | 命中 `pod.oom_killed.v1` 并保存退出码、重启次数和 Event 证据 |
| Service 无就绪端点 | EndpointSlice/Endpoints tests | `diagnosis.test.ts` | 错误 selector Service | 命中 `service.no_ready_endpoints.v1` |
| Node NotReady | Node Condition rule/gateway tests | `diagnosis.test.ts` | 独立合成 Node，Ready=False | 命中 `node.not_ready.v1` 并保存 2 条 Condition 证据；健康 Node 不命中 |
| Deployment 副本不可用 | replica-count rule/gateway tests | `diagnosis.test.ts` | 独立停滞 Deployment，2/2/0/0/2 | 命中 `deployment.replicas_unavailable.v1` 并保存全部副本计数 |
| Node 压力 | `m18_rules_test.go`、Node pressure evaluator | `diagnosis.test.ts`、Workloads UI | 保留 kind 集群上的临时 Ready + MemoryPressure Node | 命中 `node.pressure.v1`，NotReady 优先且保留压力 Condition |
| PVC Pending | exact UID Event、PVC evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | 缺失 StorageClass PVC + 显式 Warning Event | 命中 `persistentvolumeclaim.pending.v1`；无 Warning 的 WaitForFirstConsumer 不命中 |
| HPA 饱和 | HPA status/metric evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | maxReplicas + TooManyReplicas status snapshot | 命中 `horizontalpodautoscaler.saturated.v1`；TooFewReplicas 不命中 |
| Ingress 后端不可用 | route extraction, dedup and endpoint evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | 指向零 Ready Endpoint 的 Service | 命中 `ingress.backend_unavailable.v1`，采集错误不输出部分结论 |
| 人工流程与 SLA | state machine/assignment tests | diagnosis UI/API tests | 确认 ImagePullBackOff | 非法跳转 409，活动和负责人历史追加保存 |
| AI 解释护栏 | schema/citation/budget/concurrency tests | diagnosis API tests | 本地 E2E 默认关闭 | Provider 失败不影响规则；引用必须对应证据 ID |
| 审计与导出 | middleware/repository/CSV tests | `audit.test.ts` | 写操作审计 | RBAC 正确、敏感字段不进入记录、CSV 公式安全 |
| 通知 outbox | trigger/worker/retry tests | `notifications.test.ts` | 本地 E2E 默认关闭 | 事务入队、签名、重试、dead/requeue 行为稳定 |
| 受控 rollout restart | preview/execute/idempotency tests | `diagnosis.test.ts` | preview + execute + replay | 一次 dry-run、一次真实 patch、同键重放不二次写入 |
| Deployment scale | strict request/repository/service/HTTP tests | `diagnosis.test.ts`、Deployment detail | 1→2、同键重放、受控恢复到 1 | 仅接受 0..1000 整数；diff、UID/resourceVersion、dry-run、确认和历史完整；no-change 拒绝 |
| CronJob suspend/resume | strict request/repository/service/gateway tests | `diagnosis.test.ts`、CronJob detail | resume→suspend 并恢复原值 | 状态感知操作、布尔 diff、UID/resourceVersion、dry-run、确认和历史完整；no-change 拒绝 |
| 目标集群最小权限 | deployment contract tests | 不适用 | `kubectl auth can-i` | observer 保持只读；remediator 仅可在 `aiops-demo` get/patch Deployment/CronJob；删除 Pod 和修改 `kube-system` 均拒绝 |
| 有界多集群健康比较 | fleet ordering/concurrency/timeout/partial tests | `fleet.test.ts`、Dashboard | 保留 kind 真实数据路径；并发语义由确定性 stub 验证 | 最多 20 集群/4 并发/4 秒/每类 100 样本；失败局部化、覆盖率显式、无任意查询或新增写权限 |
| 有界全局资源搜索 | global search validation/ordering/concurrency/timeout/coverage tests | `global-search.test.ts`、全局搜索页 | 保留 kind 上的固定 Pod/Deployment/Service/Ingress 读取路径 | 名称 2..64、可选精确 Namespace、20 集群/4 并发/4 秒/每类与全局 100；失败与未搜索集群显式、无任意 GVK/selector/原始对象 |
| 当前用户保存筛选器 | normalization/ownership/compatibility/strict HTTP/audit tests | `global-search.test.ts`、全局搜索页保存筛选器区 | PostgreSQL 000015、API 并发 22 请求与真实 kind 查询复用 | 每用户最多 20 条、大小写不敏感名称唯一、完整覆盖修复不兼容记录、他人 ID 等同 404、审计无查询正文、无共享/selector/GVK/结果持久化 |

## M5 交付门禁

| 门禁 | 证据 | 状态 |
|---|---|---|
| 交付资产合同测试 | `TestDeliveryAssetsCoverVerificationAndThesisMaterials` | 2026-07-26 通过，已纳入全量 Go suite |
| 一键质量门禁 | `.artifacts/verification/verify-20260726-171602.json` | 通过，135.2 秒，三服务 healthy |
| M8 一键质量门禁 | `.artifacts/verification/verify-20260726-190540.json` | 通过，291.75 秒；七条规则代码基线、27 个前端用例、三服务 healthy、Kustomize 16/5/7 |
| M9 Node/Deployment kind E2E | `.artifacts/diagnosis-e2e/diagnosis-e2e-20260726-193724.json` | 通过，Kubernetes v1.34.0；两规则、证据 2/1、RBAC yes/yes/no/no、全量临时资源清理 |
| M9 一键质量门禁 | `.artifacts/verification/verify-20260726-194237.json` | 通过，283.95 秒；108 个 Go 测试入口、27 个前端用例、三服务 healthy、Kustomize 16/5/7/3 |
| M10 一键质量门禁 | `.artifacts/verification/verify-20260727-094104.json` | 通过，142.28 秒；Event API 合同、28 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M11 一键质量门禁 | `.artifacts/verification/verify-20260727-100626.json` | 通过，235.92 秒；资源健康/Namespace 拓扑合同、33 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M12 一键质量门禁 | `.artifacts/verification/verify-20260727-103859.json` | 通过，186.24 秒；四类固定详情 API 与深链接工作台、35 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M13 一键质量门禁 | `.artifacts/verification/verify-20260727-115131.json` | 通过，154.08 秒；八类固定详情、安全裁剪、关联事件、112 个 Go 测试入口、38 个前端用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M13 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-114800.json` | 通过，Kubernetes v1.34.0；三规则、处置幂等、RBAC yes/yes/no/no，并保留 `demo-kind-20260727-114759` |
| M14 一键质量门禁 | `.artifacts/verification/verify-20260727-132355.json` | 通过，154.69 秒；固定 EndpointSlice 列表、空集合回归、完整五类拓扑、114 个 Go 测试入口、44 个前端用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M14 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-130455.json` | 通过，Kubernetes v1.34.0；真实 Ingress、2 个业务 EndpointSlice、三规则、处置幂等、RBAC yes/yes/no/no，并保留 `demo-kind-20260727-130453` |
| M15 一键质量门禁 | `.artifacts/verification/verify-20260727-135642.json` | 通过，165.03 秒；固定 Node/Pod Metrics、discovery 确认的专用 424、quantity 聚合、119 个 Go 测试入口、12 个 Vitest 文件/49 个用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M15 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-134216.json`、`.artifacts/demo/demo-ready-20260727-134216.json` | 通过，Kubernetes v1.34.0；核心 Node 200/1、Node/Pod Metrics 均 424、SAR get/list=true/create=false，三规则和处置幂等通过，并保留 `demo-kind-20260727-134215`（cluster ID 33） |
| M15 Dashboard browser | `docs/changes/2026-07-27-real-resource-metrics-foundation.md` | 通过；1280x720 文档 1265/1265，390x844 文档 375/375；不可用状态、核心资源数据和六张指标卡完整，无 warning/error 日志 |
| M16 Metrics available E2E | `.artifacts/metrics-e2e/metrics-e2e-20260727-142714.json`、`.artifacts/e2e-kind/e2e-kind-20260727-142714.json` | 通过，Kubernetes v1.34.0 / Metrics Server v0.8.0；直连与平台均返回 1 Node/12 Pods，cluster ID 34，三规则和处置幂等继续通过 |
| M16 一键质量门禁 | `.artifacts/verification/verify-20260727-143242.json` | 通过，242.56 秒；119 个 Go 测试入口、12 个 Vitest 文件/51 个用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M16 Dashboard browser | `docs/changes/2026-07-27-real-metrics-utilization-consumers.md` | 通过；1280x720 文档 1265/1265，CPU/Memory 利用率、12/12 排行、指标切换和 Pod 深链正确；390x844 文档 375/375、排行单列，无 warning/error 日志 |
| M17 一键质量门禁 | `.artifacts/verification/verify-20260727-155239.json` | 通过，148.86 秒；九类固定 list/detail/OpenAPI/RBAC 合同、121 个 Go 测试入口、12 个 Vitest 文件/54 个用例、三服务 healthy、Kustomize 16/5/19/3 与运行态 HTTP 检查通过 |
| M17 Metrics + real kind E2E | `.artifacts/metrics-e2e/metrics-e2e-20260727-152830.json`、`.artifacts/e2e-kind/e2e-kind-20260727-152830.json`、`.artifacts/api-m17/api-m17-20260727-154748.json` | 通过，Kubernetes v1.34.0 / Metrics Server v0.8.0；九类 fixture list/detail、Secret `dataKeys=[example-key]`、RBAC Secret list=yes/create=no、HPA list=yes、三规则和处置幂等通过；保留 cluster ID 35 |
| M17 workbench browser | `.artifacts/browser-m17/`、`docs/changes/2026-07-27-common-workload-policy-coverage.md` | 通过；1280x720 四分类/八工作负载标签、代表性深链和零溢出；390x844 两列标签、面板内表格滚动、375px 抽屉、Secret 脱敏和 HPA Conditions 修复后无重叠；无 warning/error 日志 |
| M18 real kind diagnosis | `.artifacts/e2e-kind/e2e-kind-20260727-165019.json` | 通过，Kubernetes v1.34.0；7 条规则包含 4 条 M18 规则、Metrics、处置幂等、RBAC 通过；临时压力 Node 清理完成，保留 cluster ID 39 |
| M18 workbench browser | `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md` | 通过；Ingress/PVC/HPA 入口与 Ingress 证据抽屉可见，390x844 body/dialog 375/375 无横向溢出，浏览器错误日志为空 |
| M18 一键质量门禁 | `.artifacts/verification/verify-20260727-172323.json` | 通过，105.17 秒；123 个 Go 测试入口、12 个 Vitest 文件/55 个用例、Docker 镜像、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M19 一键质量门禁 | `.artifacts/verification/verify-20260727-180428.json` | 通过，143.85 秒；固定操作目录、严格请求/迁移/OpenAPI/RBAC 合同、128 个 Go 测试入口、12 个 Vitest 文件/56 个用例、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M19 real kind operations | `.artifacts/e2e-kind/e2e-kind-20260727-180557.json` | 通过，Kubernetes v1.34.0；7 条诊断与 rollout 回归、Deployment 1→2/同键重放/恢复 1、CronJob resume/suspend/恢复、`aiops-demo` allow 与 `kube-system` deny 均符合预期 |
| M19 workbench browser | `docs/changes/2026-07-27-controlled-operations-catalog.md` | 通过；1280x720 与 390x844 的 Deployment scale、CronJob 状态操作、类型化 diff、确认和资源历史可用；移动端仅一条 overlay 滚动条，无横向溢出或 warning/error 日志 |
| M20 Phase 1 一键质量门禁 | `.artifacts/verification/verify-20260727-190133.json` | 通过，104.53 秒；有界 fleet/OpenAPI/交付合同、133 个 Go 测试入口、13 个 Vitest 文件/57 个用例、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 fleet runtime/browser | `docs/changes/2026-07-27-bounded-multi-cluster-health.md` | 通过；两个已启用平台记录中的不可用记录被局部隔离，当前真实 kind 路径返回 1/1 Node、12/15 Pod、5/7 Deployment、10 Warning；1280x720 文档 1265/1265，390x844 文档 375/375 且 780px 表格仅在 277px 容器内滚动，无 warning/error 日志；不声明两个物理集群 |
| M20 Phase 2 双集群 fleet E2E | `.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json` | 通过；两个独立 Kubernetes v1.34.0 kind 集群与隔离平台运行时，直接/平台 Node、Pod、Deployment、Event 计数一致；ID 排序、limit、401/400、只读 RBAC、4003ms `timed_out`、恢复、`unavailable` 局部隔离及八项清理断言全部通过 |
| M20 Phase 2 最终质量门禁 | `.artifacts/verification/verify-20260727-194724.json` | 通过，223.18 秒；Go vet/全包测试/server build、13 个 Vitest 文件/57 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 Phase 3 全局搜索门禁 | `.artifacts/verification/verify-20260727-210308.json` | 通过，168.62 秒；固定四类搜索、覆盖率/OpenAPI/交付合同、140 个 Go 测试入口、14 个 Vitest 文件/58 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 Phase 3 search browser | `docs/changes/2026-07-27-bounded-global-resource-search.md` | 通过；真实 kind 返回 Pod/Deployment/Service/Ingress 四类 `nginx` 匹配，过期 peer 局部失败，Pod 深链精确打开；1280x720 文档 1265/1265，390x844 文档 375/375 且 760px 表格仅在 279px 容器内滚动，无 warning/error 日志 |
| M20 Phase 4 saved filters runtime/browser | `docs/changes/2026-07-27-user-owned-global-search-filters.md` | PostgreSQL/API 通过：并发 22 次创建严格为 20 成功/2 冲突并清零；浏览器创建、重命名、覆盖、应用与 URL 联动通过，桌面 1265/1265、移动 375/375，760px 结果表仅在 279px 容器内滚动且无 warning/error；原生删除确认因浏览器控制超时未声明 UI 完整通过，DELETE API 已通过并完成清理 |
| M20 Phase 4 最终质量门禁 | `.artifacts/verification/verify-20260727-222753.json` | 通过，351.1 秒；151 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、OpenAPI/交付合同和运行态 HTTP 检查通过 |
| M20 Phase 5 双集群 search E2E | `.artifacts/search-e2e/search-e2e-20260727-225358.json` | 通过；两个独立 Kubernetes v1.34.0 kind 集群与隔离平台运行时，9 条固定四类资源按 cluster/kind/Namespace/name 稳定排序；401/400、默认 2/2 覆盖、`cluster_limit=1`、9→3 全局截断、四类 `TIMEOUT`、恢复、四类 `QUERY_FAILED`、健康 peer 结果、只读 RBAC 与八项清理断言全部通过 |
| M20 Phase 5 最终质量门禁 | `.artifacts/verification/verify-20260727-230204.json` | 通过，158.94 秒；151 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、交付合同和运行态 HTTP 检查通过 |
| M20 Phase 6 CI/release 合同 | `backend/internal/deployment/ci_workflows_test.go`、`docs/changes/2026-07-28-versioned-ci-release-pipeline.md` | 通过；3 个 workflow 和 Dependabot/actionlint 配置均可解析，官方 Actions 固定 SHA，PR 只读且无 secrets，手动发布只产包、tag 才可发布，自托管 kind 作业非取消式运行；actionlint 1.7.7 零告警 |
| M20 Phase 6 最终质量门禁 | `.artifacts/verification/verify-20260728-100752.json` | 通过，180.85 秒；152 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、CI/release 交付合同和运行态 HTTP 检查通过 |
| M20 Phase 6 首次托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30325194933` | 通过；revision `648aea6c94fbc29fbf21d1f799df29880099d454` 的 Backend、Frontend、Manifests、Compose runtime 全部成功，运行态健康、脱敏证据上传和 teardown 通过 |
| M20 Phase 8 PostgreSQL 恢复演练 | `.artifacts/postgres-recovery/postgres-recovery-20260728-131325.json`、`docs/changes/2026-07-28-postgres-backup-restore.md` | 本地与托管 CI `30331048635` 均通过；PostgreSQL 17 源实例应用 15 个迁移并写入身份/RBAC/集群凭据/诊断/审计/筛选器合成数据，custom dump 导出后源实例先销毁，再在全新实例恢复；迁移和表级计数一致、外键异常为 0，容器/匿名卷/临时备份/进程凭据清理通过 |
| M20 Phase 8 托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30331048635` | 通过；revision `24ed4af7b74ec85438c0c8cc005f27ecf6e74886` 的 Backend、Frontend、Manifests、PostgreSQL recovery、Compose runtime、脱敏证据上传和 teardown 全部成功 |
| M20 Phase 8 本地质量门禁 | `.artifacts/verification/verify-20260728-125500.json` | 通过，278.81 秒；Go 1.25 容器全包测试与构建、14 个 Vitest 文件/59 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3、运行态 HTTP 和 actionlint 1.7.7 零告警通过 |
| M20 Phase 9 应用密钥再加密 | `.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`、`.artifacts/verification/verify-20260728-141111.json`、`docs/changes/2026-07-28-credential-key-reencryption.md` | 本地隔离实体验证通过；2 条 v1 凭据 dry-run 保持不变，损坏第二行使整批回滚且首行摘要不变，修复后 2 条转为 v2，v2-only 后端解密成功，五项清理断言通过；288.9 秒完整门禁含 163 个 Go 测试入口、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3 |
| M20 Phase 9 托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30334216631` | 通过；revision `151bc7ee848391e37b74d59f489bbe804d9234ff` 的 Backend、Frontend、Manifests、隔离凭据再加密、PostgreSQL 恢复、随机生产配置 Compose、HTTP、脱敏上传与 teardown 全部成功 |
| M20 Phase 10 签名审计归档 | `.artifacts/audit-archive/audit-archive-20260728-154047.json`、`.artifacts/verification/verify-20260728-153059.json`、`docs/changes/2026-07-28-signed-audit-archives.md` | 本地隔离 PostgreSQL 演练通过；2 条合成脱敏审计行按 ID 升序归档并由外部可信公钥验签，3 条候选在 `max-records=2` 时拒绝且无文件，一字节篡改被拒绝，五项清理通过；361.34 秒完整门禁含 167 个 Go 测试入口、三个后端二进制、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3；托管 runs `30338972042`/`30339580960` 暴露并清理 Linux 可移植性边界，最终 run `30340088789` 四个 job、三项数据库演练、Compose/HTTP/上传/teardown 全部通过 |
| M20 Phase 11 OIDC/MFA 就绪门禁 | `.artifacts/identity-readiness/identity-readiness-20260728-165405.json`、`.artifacts/verification/verify-20260728-165939.json`、`docs/changes/2026-07-28-identity-readiness-gate.md` | 本地与托管 CI `30345051371` 通过；离线严格解析 policy/discovery/JWKS，14 项检查覆盖 HTTPS issuer/endpoint、Code + PKCE S256、scope/签名/JWKS、claim/MFA、不可变 subject 绑定、session/logout 与 break-glass；无网络演练拒绝 issuer/PKCE 和 MFA/email-linking 降级且清理完整；300.97 秒本地全量门禁含 171 个 Go 测试入口、四个后端构建目标、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3；Ubuntu 四个 job、脱敏上传和 teardown 全部通过，不声明生产 SSO 已启用 |
| M20 Phase 12 生产恢复就绪门禁 | `.artifacts/postgres-recovery/postgres-recovery-20260728-174419.json`、`.artifacts/recovery-readiness/recovery-readiness-20260728-174509.json`、`.artifacts/verification/verify-20260728-175233.json`、`docs/changes/2026-07-28-recovery-readiness-gate.md` | 本地与托管 CI `30348664880` 通过；真实 PostgreSQL 17 逻辑恢复覆盖 16 个迁移、源销毁后新实例恢复、快照相等、外键异常 0 和四项清理；离线 15 项策略/证据检查通过并拒绝单份备份、陈旧证据、保留 dump 和清理不完整；199.35 秒本地全量门禁含 175 个 Go 测试入口、五个后端构建目标、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3；Ubuntu 四个 job、脱敏上传和 teardown 全部通过，`production_recovery_validated=false` |
| M21 Phase 1 历史指标基础 | `.artifacts/verification/verify-20260728-193305.json`、`backend/internal/metricshistory`、`backend/migrations/000017_metrics_history.up.sql` | 本地 281.66 秒门禁与托管 CI `30355560521` 均通过；182 个 Go 测试入口所在全包 vet/test/build、14 个 Vitest 文件/59 个用例、前端构建、三服务 healthy、Kustomize 16/5/22/3 和直连/代理 readiness 通过；真实 PostgreSQL 应用 migration 17 并确认两张表、组合外键、结果一致性/稀疏序列约束和三个索引；服务测试覆盖七日保留、1,800 样本、24 小时/1,440 点查询、缺样本不补零、失败码准入与有界清理；Hosted Backend/Frontend/Manifests 及 7m11s Compose runtime 全部成功 |
| M21 Phase 2 有界后台指标采集 | `.artifacts/verification/verify-20260728-223526.json`、`backend/internal/metricshistory/collector.go`、`docs/adr/0035-bounded-background-metrics-collection.md` | 本地 782.71 秒门禁与托管 CI `30369559322` 均通过；195 个 Go 测试入口所在全包 vet/test 和五个后端构建目标、14 个 Vitest 文件/59 个用例、前端生产构建、前后端 Docker 镜像、三服务 healthy、Kustomize 16/5/22/3 与直连/代理 readiness 全部通过；专项测试覆盖官方 Kubernetes Quantity 精确换算、负数/溢出拒绝、启用集群过滤、稳定排序、Node/Pod 公平容量轮转、六类稳定失败码、请求并发上限、即时采集/清理及取消；Hosted Backend/Frontend/Manifests 及 6m11s Compose runtime 全部成功，包含隔离演练、随机配置启动、健康/HTTP、脱敏上传和清理 |
| M21 Phase 3 鉴权精确序列历史 API | `.artifacts/metrics-history-e2e/metrics-history-e2e-20260729-081759.json`、`.artifacts/verification/verify-20260729-082024.json`、`docs/adr/0036-authenticated-exact-series-metrics-history.md` | 本地与 revision `cf20c66c588e35b9a29d492661bc99a8e1cb498b` 的托管 CI `30411146049` 均通过；真实 PostgreSQL E2E 验证跨集群/跨序列隔离、两个有序点、三次采集覆盖、一个稀疏缺样本、后端重启持久性与夹具级联清理；115.83 秒完整本地门禁含 Go vet、199 个 Go 测试入口、五个后端构建目标、14 个 Vitest 文件/59 个用例、前端生产构建、前后端 Docker 镜像、三服务 healthy、Kustomize 16/5/22/3 和直连/代理 readiness；Hosted Backend/Frontend/Manifests 及 8m21s Compose runtime 全部成功，包含历史隔离/重启演练、脱敏上传和清理 |
| Real kind E2E | `.artifacts/e2e-kind/e2e-kind-20260726-171621.json` | 通过，三规则、处置幂等、RBAC 与默认自动清理均符合预期 |
| 敏感信息扫描 | `docs/changes/2026-07-26-delivery-packaging.md` | 通过，未匹配私钥、长 token、CA payload 或 JWT bearer material |
| 答辩环境冷启动/清理 | `.artifacts/demo/demo-ready-20260726-170602.json` | 通过，从空 kind 环境重建；全清理后数据库三类 QA 行为 0 |
| 答辩截图 | `docs/thesis/screenshots/capture-metadata.json` | 通过，4 张 1440x1000 页面截图已人工检查；待按当前远端 revision 重采集绑定证据 |

## 故障注入数据

`deploy/demo-scenarios` 固化四组工作负载：健康 Nginx、健康 Service 后端、不存在镜像触发的 ImagePullBackOff、启动后退出触发的 CrashLoopBackOff，以及 selector 不匹配的 Service；M13 另加健康 Ingress、32Mi PVC 和只含普通运行配置的 ConfigMap。M17 再加入 scale-to-zero StatefulSet、DaemonSet、ReplicaSet、暂停的 Job/CronJob、HPA、ResourceQuota、PVC-only LimitRange 和只含诱饵测试数据的 immutable Secret。M18 加入缺失 StorageClass 的 Pending PVC、断后端 Ingress 和可注入状态的饱和 HPA；压力 Node 只在 E2E 期间创建。这些新增 fixture 不额外启动 Pod。所有演示对象位于 `aiops-demo` Namespace，可独立清理。

`deploy/diagnosis-e2e` 另行固化合成 NotReady Node 与 2 副本停滞 Deployment，
仅由一次性 M9 kind 门禁使用，不加入答辩三条诊断场景。
