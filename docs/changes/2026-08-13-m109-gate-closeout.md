# M109：工程卓越收口 — 覆盖率门禁 65% + fuzz smoke 扩展 + Gate B 性能基线记录与模式开关

- Date: 2026-08-13
- Status: Complete
- Scope: M109 剩余项收口：把覆盖率门禁从 60% 上调至 65%（与已达成 65.16% 匹配）、fuzz smoke 列表补入 incident/correlation、Gate B 记录两个稳定周期并引入 fail-closed 模式开关，合并并行 Agent 的归档机械门禁（见下）。

## Context

M109 路线图验收要求：Gate B 两个稳定周期记录入库；旅程 E2E 全绿（已交付）；覆盖率门禁上调至 65%。并行 Agent 已提交「归档铁律机械门禁」（`scripts/check-change-record.sh`、pre-commit 钩子、ci.yml change-record job + CHANGELOG 条目）待本块统一入库。本块将其并入同一交付。

## What Changed

### 覆盖率门禁 60% → 65%（ci.yml）

- `backend Test and coverage baseline` 步骤的全局门禁基线从 `60.0` 改为 `65.0`，与全局实测 65.16% 匹配；注释由 M84 更新为 M109。
- fuzz seed + benchmark smoke 列表追加 `./internal/incident/ ./internal/correlation/`（本仓库新增的 `FuzzEngineCorrelate` / `FuzzCanTransition` / `FuzzTransitionSequence` 首次纳入 CI fuzz 门禁）。

### Gate B 性能门禁 fail-closed 预备

- 新增 `docs/m96-gate-b-baselines.md`：记录两个稳定成功周期（CI Run 31682162681 `d919ecd`、31683950601 `7c06ac9`，均 success / Gate B passed），作为 M109 验收「Gate B 两个稳定周期记录入库」的凭据；附 report→fail-closed 翻转步骤与行为差异说明。
- `scripts/m96-gate-b.mjs`：支持 `GATE_B_MODE` 环境变量读取模式（默认 `report`）；`fail-closed` 时 `summarize()` 输出 `mode`、Markdown 备注改为「latency/heap/CSS 视为生产门禁，回归阻断 CI」。阈值校验本为 hard-fail（`requireTruthy`），模式主要驱动产出语义与文档口径。
- `.github/workflows/ci.yml` gate-b job 增加 `GATE_B_MODE: report` 环境变量与注释，指向基线文档；两周期入库后翻转即可 fail-closed。

### 归档机械门禁（并行 Agent，合入本块）

- `scripts/check-change-record.sh`：`--base <ref>`（CI）与 `--staged`（pre-commit）两种模式；改动含非文档代码文件时必须存在合规 change-record，否则非零退出。
- `scripts/git-hooks/pre-commit`：本地归档门禁钩子。
- `docs/changes/2026-08-13-archive-gate-change-record.md`：并行 Agent 的 change-record。
- `ci.yml`：新增 `change-record` job（push/PR/dispatch 运行）并纳入 `result` job 必填集。
- 本 change-record 与并行 Agent 记录的 CHANGELOG 条目已合入并统一更新。

## Verification

- `node --check scripts/m96-gate-b.mjs`：语法通过。
- `GATE_B_MODE=report` 与 `GATE_B_MODE=fail-closed` 空目录运行脚本：分别产出 `mode: "report"` / `mode: "fail-closed"`，result 均 failed（缺证据）——模式开关生效。
- `./scripts/check-change-record.sh --base HEAD~1`：已归档提交通过。
- `bash -n scripts/check-change-record.sh scripts/git-hooks/pre-commit`：语法通过。
- fuzz 种子模式：`go test -run '^Fuzz' -count=1 ./internal/incident/ ./internal/correlation/` 全绿。
- 覆盖率：`go test -cover -p=1 -count=1 ./...` = 65.16%（此前提交已达成，本块未改测试）。

## Risks / Notes

- Gate B 模式在 CI 仍为 `report`：两个稳定周期记录已入库后，下一步按 `docs/m96-gate-b-baselines.md` 翻转步骤执行 fail-closed 并记录第三个 fail-closed 周期。
- GitHub Actions artifacts 下载需认证 token，本地未拉取产物体，基线凭据以 CI run 身份/结论 + 成功 commit 为准。
- `.qoder/` 为并行 Agent 的分析工作区（317 文件，4.7M），非交付物，不提交。
