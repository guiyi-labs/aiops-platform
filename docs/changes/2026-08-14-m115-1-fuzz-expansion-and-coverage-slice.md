# M115-1：fuzz 扩展（automation plan/rollback + SLA monitor）与覆盖率首片提升

- Date: 2026-08-14
- Status: Complete
- Scope: M115 工程卓越冲刺第一片：fuzz 状态机扩展入 CI；incident/signal/correlation
  覆盖率首轮提升；覆盖率门禁保持 65%（70% 需累计多片后再上调）。

## Context

M115（docs/development-roadmap-post-m110.md Track F）要求：覆盖率 65% → 70%、性能基准
入 CI、fuzz 扩展（automation plan/rollback、SLA monitor 状态机纳入 CI fuzz seed）。
本片完成 fuzz 扩展 + 三个目标包的首轮覆盖提升；覆盖率门禁暂不上调，等后续切片接近 70%
时统一改 ci.yml。

## What Changed

### Fuzz 扩展（M115-3）
- `backend/internal/automation/fuzz_plan_lifecycle_test.go`（新增）：`FuzzPlanLifecycle`
  驱动 action plan 状态机（approve/cancel/expire 在任意状态上只返回文档化哨兵错误、
  不 panic）；`FuzzRollbackContract` 钉住 rollback 资格契约（仅 Deployment
  image_update/rollout_restart 可创建 rollback 计划）。种子 5 组 + 6 组，短时 fuzz
  通过（PlanLifecycle 38k execs、RollbackContract 64k execs）。
- `backend/internal/incident/fuzz_sla_monitor_test.go`（新增）：`FuzzSLAMonitorStateMachine`
  用任意 escalation 窗口（int64 ns）+ incident_id + level 驱动 EvaluateOnce，校验
  first < final 单调窗口契约、payload 合法性与 deep_link；361k execs 通过，无 panic。
- `.github/workflows/ci.yml`：fuzz seed smoke 包列表加入 `./internal/automation/`
  （本地全量 fuzz smoke 12 包全绿）。

### 覆盖率提升（M115-1，首片）
- `backend/internal/signal/`：63.9% → 70.1%。
  - `normalizer_test.go`：`TestChangeSignalIDAllKinds`（maintenance/restore/失败分支）、
    `TestMapSeverityUsesPolicyMappingsThenFallback`、`TestAlertNormalizer_FallsBackToUpdatedAt`。
  - `slo_burn_normalizer_test.go`：`TestSLOBurnSink_WithLoggerSetsNonNil`、
    `TestSLOBurnSink_IngestFailureLoggedNotPropagated`（Warn 分支 + failingUpsertRepo）。
  - `service_test.go`：`TestBuildOccurrence_RejectsZeroClusterAndMissingObservedAt`、
    `TestBuildOccurrence_FallsBackToObservedAtForFreshness`、
    `TestService_OverviewRejectsRecentChangesError`。
  - `diagnosis_drain_test.go`：`TestDiagnosisDrainDefaultsForZeroConfig`。
- `backend/internal/correlation/`：67.2% → 70.7%。
  - `merge_test.go`（新增）：`TestCorrelateMergesSameCaseKeyFromMultipleTriggers`
    （Correlate 合并路径：同 rule/同 case_key 的第二个 trigger 合并 factors/links）、
    `TestCorrelateEmptyTriggerSkips`。
  - `logic_test.go`：`TestClassifyCompletenessAllBranches`、`TestConfidenceRankDefaultForUnknown`、
    `TestSortTriggerSignalsTieBreakBySignalID`、`TestDiagIndexLookupBoundaries`、
    `TestComputeFactorsFreshnessAndCoverageBranches`、`TestComputeFactorsVeryStaleBranch`。
  - `provider_test.go`：三个 provider 错误传播测试 + `TestToEvidenceEmptySlicesReturnNil`。
- `backend/internal/incident/`：57.0% → 61.7%（GormRepository 224 stmt 需真实
  Postgres 才能测，包内可达上限约 69.6%，后续切片再补 service/markdown 剩余分支）。
  - `metrics_test.go`：`TestServiceMetricsUsesDefaultWindowAndClusterFilter`、
    `TestServiceMetricsClampsWindowAndRespectsClusterFilter`（Service.Metrics 0% → 90.9%）。
  - `service_test.go`：`TestServiceResponseCatalogReturnsIndependentClone`、
    `TestServiceAssignValidatesAssignee`、`TestServiceFollowerValidation`、
    `TestServiceNoteValidation`、`TestServiceExportErrors`。

## Verification

- `go test -run '^Fuzz' -count=1` ×12 包（含新增 automation）：全绿。
- `FuzzPlanLifecycle` fuzztime=5s：38754 execs PASS；`FuzzRollbackContract`：64166 execs PASS。
- `FuzzSLAMonitorStateMachine` fuzztime=8s：361231 execs PASS。
- `go test -cover -p=1 -count=1 ./internal/signal/`：70.1%；`./internal/correlation/`：70.7%；
  `./internal/incident/`：61.7%。
- `go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...` + `go tool cover -func`：
  全局 65.6%（上片基线 65.3%）。

## Risks / Notes

- 覆盖率门禁 65.0 暂未改动；ci.yml 的 65→70 上调与核心包门禁扩展留待后续切片（M115-1b/c/d）
  达成全局 ~70% 时一并提交。
- incident 包 GormRepository 无 sqlite driver，只能做 Postgres 集成测试；包级 70% 门禁
  若引入需先建 DB-backed 测试（不在本片范围）。
- FuzzSLAMonitor 失败语料曾写入 testdata/fuzz/，已删除；种子以代码内 f.Add 为准。