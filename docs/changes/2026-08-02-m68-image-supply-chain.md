# M68: Image Supply-Chain & Reproducibility Read-Only Analyzer

- Date: 2026-08-02
- Status: Development Complete (read-only analyzer; committed `a50cd52`; CI backend 7-gate + frontend green)
- ADR: 0004 (read-only posture)
- Fast gate: PASSED — CI backend 7-gate + frontend (eslint / vue-tsc / vitest / vite build) green

## Summary

Adds a read-only image supply-chain and reproducibility analyzer to the
optimization center. It answers the question an operator asks before an
incident or an audit: "which images am I actually running, and how reproducible
are they?" Reproducibility is the foundation of any CVE response — a mutable
tag (`:latest`) or a tag-only reference (no digest pin) means a rebuild can
silently change production and a CVE fix may not land where expected.

The analyzer is pure and offline (ADR 0004): it reasons statically over an
observation bundle (M65 collector via read-only List). It never contacts a
registry, never pulls a manifest. Real CVE scoring (Trivy / Grype / advisory API)
is a deliberate follow-up; this analyzer delivers the inventory and
reproducibility findings such a source would consume.

Rules (`internal/imagepolicy` pure `Evaluate`):

- `IMG_MUTABLE_TAG` → warning: image uses `:latest` or no tag, a redeploy can
  run a different build.
- `IMG_NO_DIGEST_PIN` → info: image referenced by a specific tag but no digest,
  the tag could be re-pointed to a different manifest.
- `IMG_PULL_ALWAYS_LATEST` → info: `imagePullPolicy: Always` with a mutable tag
  re-pulls whatever `:latest` currently points at on every restart.
- `IMG_SHARED_ACROSS_NS` → info: same image runs in more than one namespace,
  widening blast radius and complicating rollout.
- `IMG_MULTIPLE_TAGS` → info: one repository is referenced by several tags, an
  easy source of version skew between workloads.

Family: `supply-chain`.

## Files Changed

### New Files

- `backend/internal/imagepolicy/model.go` — `ImageInfo` (decomposed
  repository / tag / digest / pullPolicy), `ImageUsage`, `Inputs`, `Status`
  (images_total, containers_total, mutable_tag_images, unpinned_images,
  by_severity, by_family, findings) + finding codes / severities / family.
- `backend/internal/imagepolicy/service.go` — pure
  `Evaluate(clusterID, Inputs, at)`; findings sorted by severity then code.
- `backend/internal/imagepolicy/service_test.go` — 329-line table suite covering
  each rule branch (mutable tag vs digest pin vs shared / multiple tags).
- `frontend/src/views/OptimizationView.vue` — new "镜像供应链" tab (inventory
  cards: 镜像数 / 容器数 / 可变标签数 / 未锁定数 + findings table).

### Modified Files

- `backend/internal/optimization/collector.go` — `CollectImagePolicy` maps
  container image references (M65 read-only List).
- `backend/internal/optimization/service.go` — delegates `CollectImagePolicy`.
- `backend/internal/httpserver/optimization.go` — `imageAnalyze` handler
  (explicit bundle or auto-collect).
- `backend/internal/httpserver/router.go` — `POST /api/v1/optimization/image/analyze`
  (audit `optimization.image.analyze`).
- `docs/api/openapi.yaml` — `analyzeImagePosture` operation +
  `ImagePostureStatus` schema.
- `frontend/src/types/optimization.ts` — `ImagePostureStatus` interface.
- `frontend/src/api/optimization.ts` — `analyzeImage(token, clusterId)`.
- `frontend/src/api/optimization.test.ts` — client success / failure cases.

## Verification

CI gate reproduced locally before push: gofmt 0, vet 0, coverage (backend gate),
5 binaries built, golangci-lint 0, eslint 0, vue-tsc 0, vitest, vite build green.

## Notes

- `json.Number` utilization-style fields cannot be cast directly to `float64`;
  use `strconv.ParseFloat(string(b), 64)`.
- Findings reuse the canonical `internal/finding` contract and render uniformly
  with the other optimization analyzers.
