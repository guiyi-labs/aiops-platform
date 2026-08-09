# M84: Test Intensity Upgrade — fuzz targets, benchmarks and core-package coverage gate

- Date: 2026-08-09
- Status: Development Complete (local; CI gate added but pending remote push on 2026-08-10)
- ADR: n/a (test-only, no behavior change)
- Fast gate: PASSED locally — gofmt 0, go build ./..., go test ./... all green, benchmarks run

## Summary

Adds the M84 engineering-excellence workstream: seed fuzz targets across the
pure parsers/validators that back the analyzers, focused benchmarks for the hot
paths, and a CI core-package coverage gate (≥70%) alongside the existing global
≥50% baseline. The Polish roadmap's W8 "global ≥60%" delta is tracked as a
follow-up incremental lift (see Notes), not forced as a broken gate.

## Files Changed

### New Files (fuzz)

- `backend/internal/metricshistory/fuzz_quantity_test.go` — CPU/memory
  Quantity parser (k8s `resource.ParseQuantity`).
- `backend/internal/deprecatedapi/fuzz_version_test.go` — apiVersion split +
  catalog lookup + minorVersion.
- `backend/internal/apiquery/fuzz_query_test.go` — paginated list-query
  contract parser + positiveInt decoder.
- `backend/internal/netpolicy/fuzz_port_test.go` — exposed-port range parser.
- `backend/internal/namespaceposture/fuzz_ratio_test.go` — quota utilization
  ratio (flow must stay finite).
- `backend/internal/optimization/fuzz_quantity_test.go` — collector CPU/mem
  parse.
- `backend/internal/kubernetes/fuzz_revision_test.go` — rollback revision.
- `backend/internal/posture/fuzz_severity_test.go` — aggregate risk-ordering.
- `backend/internal/topology/fuzz_hash_test.go` — evidence + plan-ID hash.

### New Files (benchmark)

- `backend/internal/metricshistory/bench_evaluate_test.go` — EvaluateWindow.
- `backend/internal/topology/bench_edges_test.go` — SortEdges (500 edges).
- `backend/internal/posture/bench_aggregate_test.go` — aggregate severity sort.
- `backend/internal/capability/bench_registry_test.go` — Registry List snapshot.

### Modified Files

- `.github/workflows/ci.yml` — added "Core package coverage gate (M84)" +
  "Fuzz seed + benchmark smoke (M84)" steps after the global coverage step.

## Verification (local)

- `go test -run '^Fuzz' -count=1 ./...` over the 9 target packages: green.
- `go test -bench . -benchtime 20x -run '^$' ./internal/{metricshistory,topology,posture,capability}/`: green
  (EvaluateWindow ~15µs, SortEdges ~578µs, AggregateSeveritySort ~307µs, RegistryList ~6µs).
- `go build ./...`, `gofmt -l .` (0), and full `go test -p=1 -count=1 ./...`: all green.
- Core packages measured for the ≥70% gate: metricshistory 79.4%, apiquery 100%,
  deprecatedapi 93.2%, optimization 75.9%.

## Notes

- **Global coverage is currently 55.3%** — below the M84 "global ≥60%" target.
  Raising it needs distributed test additions across several packages (many
  repository/cmd layers are ~0%); the CI global gate therefore stays at its
  proven 50% baseline so the pushed branch remains green. The ≥60% lift is an
  explicit, tracked follow-up in `docs/polish-plan.md` W8.
- Fuzz runs in CI as seed-corpus smoke (`go test -run '^Fuzz'`), which is
  deterministic and CI-safe; long-form `-fuzz` is a developer-local action.
- Benchmark numbers are a first-time baseline (no prior benchmarks existed),
  suitable for trend tracking as stale baselines accumulate in docs.