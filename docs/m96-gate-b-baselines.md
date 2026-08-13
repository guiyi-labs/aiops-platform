# M96 Gate B 稳定周期记录

> **Fail-closed active**（M109，2026-08-13）：两个稳定周期记录入库后翻转，CI 报告与基线均改为 fail-closed，超阈值视为回归并阻断 CI。

## 稳定周期记录

| # | CI Run | Commit | 结果 | 采集日期 (UTC) |
|---|--------|--------|------|----------------|
| 1 | 31682162681 | `d919ecd` feat(frontend): 登录页触觉细节增强（第四轮） | success（Gate B passed） | 2026-08-13 08:27:52 |
| 2 | 31683950601 | `7c06ac9` test(frontend): M109 incident E2E | success（Gate B passed） | 2026-08-13 08:51:43 |

> 两次运行均为同一主干、同一 fixture（`m96-v1`）下 success，表明阈值在 report mode 下稳定。

## 翻转执行记录

- 2026-08-13：翻转证据生产者（`pod-scale-perf-report.mjs`、`login-perf-report.mjs`、`style-audit.mjs`、`scalebench report.go`）mode → `fail-closed`；ci.yml `GATE_B_MODE=fail-closed`；`m96-gate-b.mjs` 模式参数化与 markdown 口径更新。

## 原翻转步骤（记录入上述两周期后执行）

1. 将证据生产者（`pod-scale-perf-report.mjs`、`login-perf-report.mjs`、`backend scalebench report.go`）的 `mode` 从 `'report'` 改为 `'fail-closed'`（一次发布提交）。
2. Gate B 脚本：设置 `GATE_B_MODE=fail-closed` 环境变量（或移除变量默认读 fail-closed）。
3. 本文件新增第三个条目记录首次 fail-closed 稳定运行，作为后续回归的真基线。
4. 在 CHANGELOG 新增条目；在本文件顶部更新状态为 `Fail-closed active`。

## 门禁行为差异（report vs fail-closed）

- `report`：`mode` 字段写入产物 JSON/Markdown，阈值校验仍为 hard-fail（`requireTruthy`），但文档注释说明为观测性。
- `fail-closed`：同逻辑，但产物 `mode` 改为 `fail-closed`，Markdown 备注改为「latency/heap/CSS 漂移超过阈值视为回归并阻断 CI」。

## References

- AGENTS.md: 归档铁律
- `docs/development-roadmap-post-m106.md`: M109 验收
- `scripts/m96-gate-b.mjs`: 校验逻辑
- `frontend/scripts/pod-scale-perf-report.mjs`: 前端 budget 生产者
- `backend/internal/scalebench/report.go`: 后端 budget 生产者
