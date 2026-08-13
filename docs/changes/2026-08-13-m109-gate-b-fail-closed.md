# M109：Gate B 性能门禁 fail-closed 翻转

- Date: 2026-08-13
- Status: Complete
- Scope: 完成 M109 最后一项——把 M96 Gate B 性能门禁从 report 模式翻转为 fail-closed。前置（两个稳定周期入库）已在上一次 `m109-gate-closeout` 提交落地。

## Context

`docs/development-roadmap-post-m106.md` M109 要求「性能门禁 fail-closed：M96 Gate B 连续两个稳定周期后，超阈值从 warning 转 fail-closed」。上一提交已将两个稳定成功周期（Run 31682162681、31683950601）记录入库并提供 `GATE_B_MODE` 开关，本提交执行实际翻转。

## What Changed

### 证据生产者 mode → fail-closed

- `frontend/scripts/pod-scale-perf-report.mjs`：budget `mode: 'report'`→`'fail-closed'`，注释改为「超阈值视为回归、阻断 CI」。
- `frontend/scripts/login-perf-report.mjs`：budget `mode` 同样翻转，备注更新。
- `frontend/scripts/style-audit.mjs`：`mode: 'report'`→`'fail-closed'`，标注行同步。
- `backend/internal/scalebench/report.go`：报告 `Mode: "report"`→`"fail-closed"`，markdown 附注改为「fail-closed 生产门禁，违规阻断 CI」。
- `backend/internal/scalebench/scalebench_test.go`：断言 `report.Mode` 更新为 `"fail-closed"`。

### 门禁脚本与 CI

- `scripts/` 内 `m96-gate-b.mjs`：`EXPECTED.css.mode` 应随模式（`'fail-closed'`）；`summarize()` 的 `performanceThresholds` 说明按 `GATE_B_MODE` 动态输出；markdown 备注已由前次提交支持 fail-closed。
- `.github/workflows/ci.yml`：gate-b job 的 `GATE_B_MODE` 从 `report` 翻转为 `fail-closed`。

### 文档

- `docs/m96-gate-b-baselines.md`：顶部状态改为 **Fail-closed active**，追加「翻转执行记录」条目。

## Verification

- `node --check`：`pod-scale-perf-report.mjs` / `style-audit.mjs` / `m96-gate-b.mjs` 语法通过（login-perf-report.mjs 带 BOM 的 shebang 为既有特征，运行时正常）。
- `GATE_B_MODE=fail-closed node scripts/m96-gate-b.mjs`（空目录）：产出 `mode: "fail-closed"`，result failed（缺证据）——开关生效。
- `cd backend && go test ./... -short`：全绿（scalebench 断言已更新通过）。
- `go build ./internal/scalebench/`：通过。

## Risks / Notes

- 门禁翻转后依赖 CI 实测证据：首次 fail-closed 运行应在上线后记录为第三个稳定周期（真基线），见 `docs/m96-gate-b-baselines.md`。
- 阈值本身在 report 模式已是 hard-fail 校验；翻转主要改变产出语义（mode 字段/文档口径）与 CI 显式声明。
- `.qoder/` 分析工作区非交付物，不提交。
