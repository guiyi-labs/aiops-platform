# M96-B 后端规模基准报告

- Date: 2026-08-10
- Status: Draft
- Scope: 基于 `m96-v1` fixture 建立后端结构化 report-mode 性能基线，不预设 fail-closed 阈值

## Context

M96-A 已固定 500 Node / 50k Pod / 100k Event 输入和数据哈希，但尚未记录关键后端操作的延迟分布、内存、goroutine、分页、取消、超时与背压行为。本增量把这些观察绑定到同一 fixture 和 commit，并保留可复核 JSON/Markdown 报告。

## What Changed

### Fixture-backed 基准器

- `backend/internal/scalebench/data.go`：验证后流式加载 fixture，按 namespace 建立只读索引；全局搜索适配器只复制当前页，不复制完整匹配响应。
- `backend/internal/scalebench/report.go`：执行拓扑全 namespace 派生、全局搜索、Pod/Event 全量分页、Node/Pod 历史查询、Pod 历史评估和八槽背压流；输出 P50/P95/P99、heap、goroutine 和硬不变量。
- `backend/cmd/scale-bench/`：提供可重复 CLI，写入版本化 JSON 和 Markdown，并记录 OS/arch/Go/CPU/GOMAXPROCS/commit/fixture hashes。
- `docs/adr/0083-m96-backend-scale-benchmark.md`：冻结 report-mode 方法和阈值升级规则。

### CI 与状态

- `.github/workflows/ci.yml`：canonical fixture 校验后运行 3 warmup / 30 sample 基准，上传结构化报告并构建 `scale-bench` 二进制。
- `CHANGELOG.md`、`docs/PROJECT_STATUS.md`、`docs/next-long-term-plan.md`：记录 M96-A/B 已落地，M96 前端与壳层工作仍未完成。

## Verification

- `go test -count=1 ./internal/scalebench ./cmd/scale-bench`：通过。
- `go vet ./internal/scalebench ./cmd/scale-bench`：通过。
- 完整 `m96-v1` 预提交试跑：3 warmup / 30 samples、8 operations、全部语义不变量通过；正式 commit 绑定报告将在功能提交后重跑并补入本记录。

## Risks / Notes

- 当前结果衡量 fixture-backed 业务逻辑和有界查询，不包含 PostgreSQL 网络往返、HTTP 序列化或真实 kube-apiserver 延迟，不宣称生产容量。
- 性能值仍为 report mode；至少连续两个稳定 CI 周期后才评估回归阈值，当前只让计数、取消、超时、背压和 goroutine 不变量 fail-closed。
