# M96-A 确定性规模 Fixture

- Date: 2026-08-10
- Status: Complete
- Scope: 建立 M96 的可重复规模数据源与清单校验，不包含性能阈值或生产容量结论

## Context

M96 要求在 500 Node / 50k Pod / 100k Event 规模上为拓扑、工作负载、全局搜索和历史窗口建立可重复证据。此前只有小型内存测试和局部 benchmark，无法把这些领域固定到同一份样本，也不应把大数据集直接提交到仓库。

## What Changed

### 生成器与数据契约

- `backend/testdata/scale/m96-v1.json`：固定 schema、seed、观察时间、资源计数、每 Pod 两个 Event 和六个历史点。
- `backend/internal/scalefixture/`：流式生成 Node、workload projections、Pod、Event 和 metrics-history NDJSON；Pod 保留 owner、node、search 映射，历史样本保留资源 UID、容器、指标和时间窗口。
- `backend/internal/scalefixture/verify.go`：逐条读取 gzip 流，校验 JSON、记录数、原始字节数、压缩字节数、逐文件 SHA-256、配置哈希、覆盖映射和聚合数据哈希。
- `backend/cmd/scale-fixture/`：提供生成与校验 CLI；只保存版本化配置，生成数据落在被忽略的 `.artifacts/`。
- `docs/adr/0082-m96-deterministic-scale-fixture.md`：记录数据范围、流式格式、哈希语义和不宣称生产容量的边界。

### CI 与项目状态

- `.github/workflows/ci.yml`：后端 job 生成并校验 canonical fixture，上传 manifest、生成清单和校验清单，构建新 CLI；不上传大数据流。
- `CHANGELOG.md`、`docs/PROJECT_STATUS.md`、`docs/next-long-term-plan.md`：记录 M96-A 已完成及 M96 其余工作仍未完成。

## Verification

- `go test ./internal/scalefixture ./cmd/scale-fixture`：通过。
- `go test -p=1 -count=1 ./...`：通过，全量 Go 包测试无回归。
- `go test -cover -p=1 -count=1 -coverprofile=.m96-coverage.out ./...`：通过，全局覆盖率 60.6%。
- `go vet ./internal/scalefixture ./cmd/scale-fixture`、`git diff --check`：通过。
- `go test -race`：当前 Windows 环境缺少 `gcc`，本地无法执行；CI Linux race job 继续作为远端证据。
- `go run ./cmd/scale-fixture -config testdata/scale/m96-v1.json -output .artifacts/scale-fixture/m96-v1`：通过，生成 5 个 gzip NDJSON 流。
- `go run ./cmd/scale-fixture -verify .artifacts/scale-fixture/m96-v1`：通过，逐条校验完整输出。
- 完整清单：500 Node、5,000 workload、50,000 Pod、100,000 Event、606,000 history sample；压缩数据总量 17,651,502 bytes，`dataset_sha256=81faa1de39eaca4dfb84944ebd7bf155bdc1e3716e5f1ae6431bcdb406647c71`。
- CI 证据路径：`backend/.artifacts/scale-fixture/m96-v1/manifest.json`、`backend/.artifacts/scale-fixture/m96-generation.json`、`backend/.artifacts/scale-fixture/m96-verification.json`；生成数据本身不入库。

## Risks / Notes

- 该 fixture 是确定性基准输入，不等同于生产容量承诺；P50/P95/P99、内存峰值、取消和背压报告在后续 M96 增量完成。
- 修改 schema、映射、seed 或数量时必须提升 dataset version，并重新生成 manifest；M89/M90 仍未完成，项目状态保持 RC。
