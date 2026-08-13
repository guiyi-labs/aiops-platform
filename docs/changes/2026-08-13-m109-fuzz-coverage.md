# M109：工程卓越起步 — correlation/incident fuzz 扩展 + 重点包覆盖率提升

- Date: 2026-08-13
- Status: Complete
- Scope: M109 工程卓越收口的首个可验证块：为 correlation 引擎与 incident 状态机新增 fuzz 用例（路线图指定目标），并为 incident/correlation/signal 三个重点包补测（路线图 65% 覆盖率目标的前置铺垫）。

## Context

M108 关联归一已全量验收（demo-drill 41/41）。按 `docs/development-roadmap-post-m106.md` 进入 M109 工程卓越：性能门禁 fail-closed、incident E2E、覆盖率 60%→65%、fuzz 扩展。本轮先落地可本地验证的 fuzz 扩展与覆盖率提升，覆盖基线从 60.6% 升至 60.9%，重点包：incident 40.0%→43.1%、correlation 64.7%→67.2%、signal 56.4%→63.9%。

## What Changed

### Fuzz 扩展（M109 路线图指定目标）

- `backend/internal/correlation/fuzz_engine_test.go`（新）：`FuzzEngineCorrelate` 以结构化随机输入（信号/变更/拓扑边/诊断，含垃圾值）驱动确定性关联引擎，断言永不 panic、结果 rule_id 必在目录、confidence 合法、case 必 active、case_key 非空。
- `backend/internal/incident/fuzz_transition_test.go`（新）：`FuzzCanTransition` 钉死状态机真值表（合法边 + 非法状态拒绝）；`FuzzTransitionSequence` 走 Service.Transition + CAS 镜像内存仓库，断言每次成功必版本 +1、失败仅限 `ErrInvalidTransition`/`ErrVersionConflict`。

### 覆盖率补测（重点包）

- `backend/internal/incident/model_test.go`：`TestSourceRefs` 补 `SourceRefForCorrelation`。
- `backend/internal/incident/service_test.go`：新增 `TestService_ListAndSummary`、`TestAssignFailureCode`（四分支）。
- `backend/internal/incident/sla_monitor_test.go`：新增 `TestSLAMonitorRunLifecycle`（Run 立即评估 + 上下文取消退出）。
- `backend/internal/signal/service_test.go`：新增 `TestService_GetNotFoundWithNopRepository`、`TestService_IngestBatchCountsSuccesses`、`TestNopSourceReaderDefaults`。
- `backend/internal/signal/diagnosis_drain_test.go`：新增 `TestDiagnosisDrainRunLifecycle`（Run 立即 drain + 取消退出）。
- `backend/internal/correlation/logic_test.go`：新增 `TestComputeReasonCode`（五分支）。

## Verification

- `cd backend && go test ./... -short`：全绿。
- fuzz 引擎实测：`FuzzEngineCorrelate` 15s ≈ 189K execs 无失败；`FuzzCanTransition` 10s ≈ 1.2M execs；`FuzzTransitionSequence` 15s ≈ 1.5M execs。
- 覆盖率：全局 `go test -cover -p=1 -count=1 -coverprofile=... ./...` = **60.9%**（此前 60.6%）；重点包 incident 43.1% / correlation 67.2% / signal 63.9% / metricshistory 79.3%。
- `gofmt -l`（改动包）：干净。

## Risks / Notes

- 65% 全局门禁未达成（当前 60.9%）：大头是 GormRepository（DB 绑定，现无测试基建）与 httpserver（2872 未覆盖语句）；需要 postgres 测试基建或系统性 handler 测试，属 M109 后续块。
- CI 的 `Fuzz seed + benchmark smoke` 步骤列表尚未纳入 incident/correlation（该 job 位于并行 Agent 未提交的 `ci.yml` 改动中）；本轮不触碰该文件，fuzz 目标仍以 seed 模式随 `go test ./...` 执行。并行轨收口后应把 `./internal/incident/ ./internal/correlation/` 追加进 fuzz smoke 列表。
- 性能门禁 fail-closed 与 incident 旅程 E2E 未在本轮范围内，见路线图 M109 后续。
