# M115 论文实验摘要：诊断覆盖 / P95 延迟 / 幂等性 / 资源成本

- Date: 2026-08-15
- Status: Complete
- Scope: 新增答辩材料 `docs/thesis/experiment-summary.md`，补齐 Obsidian P0 清单「最终实验摘要」项

## Context

Obsidian「GitHub 项目展示优化与后续路线」P0 清单要求为旗舰项目补最终实验摘要：
诊断覆盖率或准确率、P95 延迟、受控操作幂等性、资源成本。本节以
M115 基线（`e7daa6f`）为准，全部数字来源于现场实测或既有 drill 证据。

## What Changed

### 文档
- `docs/thesis/experiment-summary.md`：新增实验摘要，含 4 节
  （1 诊断覆盖与准确率：3 规则复现 + demo drill 41/41 + 覆盖率 ~74%，并明确
  不宣称真实集群上的精确率/召回率；2 API P95 延迟：diagnoses p95 3.54ms /
  clusters 2.12ms / ready 2.08ms，注明本地功能级测量方法；3 受控操作幂等性：
  同 Idempotency-Key 重放返回同一计划且 mock 仅 1 次 mutation，跨环境复现；
  4 资源成本：平台三服务合计约 92 MiB 内存），附局限与后续。

## Verification

- 幂等性现场复测（2026-08-15）：preview 计划 `fca00e11-…` → execute `succeeded` →
  同 key 重放返回同计划、`/mock/mutations total=1`，与 M102 drill 证据一致。
- P95 实测：每端点 50 采样，`python3` 计时统计 p50/p95/p99（数据见文档表格）。
- 资源实测：`docker stats --no-stream`（postgres 55.4 MiB / backend 33.0 MiB /
  frontend 3.3 MiB / demo-mock 6.5 MiB）。
- 既有证据引用：`.artifacts/demo-drill/report-20260813-162314-8518b3.json`（41/41 绿）。

## Risks / Notes

- P95 为本地功能级下限参考，非并发压测；真实集群规模与并发基准待后续补充后再
  写入论文结果章节。文档已明确标注方法论与诚实边界。
