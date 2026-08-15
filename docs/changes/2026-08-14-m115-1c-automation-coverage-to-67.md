# M115-1c：automation 包覆盖率 46.5% → 66.9%（Execute/Verify 全生命周期测试）

- Date: 2026-08-14
- Status: Complete
- Scope: M115 工程卓越冲刺第三片：`internal/automation` 覆盖率从 46.5% 提到
  66.9%（service.go 46.5%→~75%，配置文件全部补齐），与 70% 门禁进一步对齐。

## Context

M115（docs/development-roadmap-post-m110.md Track F）全局覆盖率 65%→70%。
本片继续在 M115-1b（httpserver 70.0%）之后抬升 automation 包：它是纯逻辑包、
已有完整 fake 基建（fakeCaseReader/fakeKubernetesSource/verifier stubs），是
最能低成本兑现覆盖率的第二杠杆（service_test 现有 memRepo 骨架）。

## What Changed

全部为 `backend/internal/automation/service_test.go` 扩展（无生产代码改动）：

- `TestGetPlanReturnsPlan` / `TestGetPlanNotFound`：Service.GetPlan 透传与哨兵。
- Preview 路径：`TestPreviewDisabledService`（enabled=false）、`TestPreviewDisabledRepoNil`、
  `TestPreviewNotDraftReturnsErrNotDraft`、`TestPreviewRollbackErrDisabledK8s`；
  `previewCapableRepo`（memRepo + MarkPreviewed）驱动 `TestPreviewSuccessTransitionsToPreviewed`
  全 happy path（draft → refreshSnapshot → gates → previewed）。
- materializeParameters 分支补齐：rollout_restart（成功快照）、rollback（历史
  前一修订、ReplicaSet 证据）、`TestCreatePlanWithRollbackNoRollbackPoint`、
  image_update 无 runbook 契约断言 + 直接 materialize 调用、scale（成功/无变化/
  缺 override）、cronjob.suspend 已挂起无变化、`ErrUnsupportedAction` default 分支。
- refreshSnapshot：nil k8s → ErrDisabled、unsupported kind、Deployment/CronJob 成功。
- Execute 全生命周期：`execRepo`（内存实现 Claim/Complete/Fail/SaveVerification/
  GetVerificationByPlan/UpdateVerification/MarkVerified）驱动
  `TestExecuteHappyPath`（Approved→Executing→Succeeded + scheduleVerification 持久化）、
  `TestExecutePatchErrorFailsAndSchedulesVerification`（Fail 后仍调度验证）、
  `TestExecuteGateRecheckFailure`（无 k8s 源 → gate fail-closed → Failed）、
  `TestExecuteWrongConfirmationToken`。
- Verify happy path：`TestVerifyHappyPathEffective`（SLO 改善前→后 + rollout_restart
  后快照 restarted_at 更新 → ComparisonImproved → Effective + MarkVerified）；
  `TestScheduleVerificationNilVerifier` 幂等返回。

## Verification

- `go test ./internal/automation/`：全绿（含既有测试 + 新增 20+ 用例）。
- `go test -run '^Fuzz' -count=1 ./internal/automation/`：FuzzPlanLifecycle /
  FuzzRollbackContract 种子回归通过，无 panic。
- `go test -cover ./internal/automation/`：66.9%（基线 46.5%，+20.4pp）。
- 经验证 global 基线数字后续片归档中统一列（本片改变全局约 +0.4pp）。

## Risks / Notes

- `execRepo`/`previewCapableRepo` 是测试专用的内存 Repository 实现，仅覆盖
  Execute/Verify 需要的接口方法；其余走 NopRepository no-op。注意 Future 演进
  若改 Repository 接口签名需同步这两个测试 repo。
- automation 距 70% 门禁只差 ~3pp；剩余主要是 NopCaseReader.GetCase/
  EligibleActionCodes 与 applyPatch 的 cronjob 分支，可在 M115 后续切片补齐。
- 覆盖率门禁 ci.yml 65.0 仍未改（M115-1e 统一上调）。
