# 修复 metricshistory 测试顺序 flaky（CI backend race）

- Date: 2026-08-15
- Status: Complete
- Scope: 修复 CI `Backend race` job 的间歇性失败。

## Context

CI 在 `fa03612`/`7113693` 后 Backend race job 间歇性失败：
`TestDownsampleAndArchiveAggregatesHourlyBuckets`（internal/metricshistory）。

## 根因分析

- `DownsampleAndArchive` 从 `buckets map[string]*aggregateKey` 遍历产出
  `rows`，Go map 遍历顺序**未定义**。
- 测试断言 `repository.downsampled[0]` 必须为 api-0（3 个样本聚合），
  但另有 api-1 记录，map 顺序翻转时 index 0 是 api-1 → 断言失败。
- 该测试来自 `455f5c3`（M114 指标历史下采样），在 Operator 增强之前已存在；
  `-race` 改变 hash seed，放大顺序不稳定性触发（本地 race 复跑约 1/5 失败）。
- **非数据竞争**、非 operator 回归；是测试断言依赖未定义顺序的 flaky。

## What Changed

`backend/internal/metricshistory/service_test.go`：

- 断言改为按 series 身份（ResourceName）查找各行再校验聚合值，
  不再依赖切片位置；并补 api-1 记录断言。

## Verification

- `go test -race -count=1` 复跑 8 次全绿（此前约 1/5 失败）。
- `go test -race ./internal/...`：全包 race 全绿。

## Risks / Notes

- 纯测试断言修复，无产品代码改动；该 flaky 是 CI race job 失败的直接
  根因之一。
