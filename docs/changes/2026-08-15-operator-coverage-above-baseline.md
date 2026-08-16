# Operator 包单测补强：全局覆盖率推过 70% 门

- Date: 2026-08-15
- Status: Complete
- Scope: CI `Backend Test and coverage baseline` 最后缺口 —— coverage 69.9%
  差 0.1% 未过 70% 门禁，根因是新增 operator 包拉低整体覆盖率。

## Context

`d23dc8b`（lint 修复）后 CI 覆盖率门禁仍失败：`Coverage 69.9% is below
70.0% baseline`。分析确认 operator 包（ControlledOperation CRD +
controller，179 stmts）仅 72.1% 覆盖，50 stmts 未覆盖拖累全局。

## What Changed

`backend/internal/operator/` 测试补强（fake dynamic client + reactor）：

- `executor_test.go`（新增）：
  - rollout_restart 分支：patch 落盘 + 注解验证
  - cronjob.suspend 分支：spec.suspend=true 验证（新增 AsCronJobGVR）
  - dry-run：用 `PatchActionImpl.GetPatchOptions().DryRun` 捕获补丁选项
    验证 `dryRun=All` 透传（fake client 不模拟 dryRun 语义，断言选项
    本身才是正确可观察量）+ dry-run=false 无选项
  - unsupported target kind / unknown action 拒绝
- `util_test.go`（新增）：IsNotFound（真实 NotFound 错误）、KeyFor /
  NamespacedNameFor、ControlledOperationList DeepCopy（嵌套指针隔离 +
  nil receiver）、List Unstructured round-trip、Enqueue tombstone /
  非 meta 对象错误路径
- `controller_test.go`：新增 AsCronJobGVR helper

## Verification

- `go test ./internal/operator/`：全部用例通过。
- operator 包覆盖率：**72.1% → 88.8%**（无 0% 函数）。
- 全局覆盖率（CI 同款命令）：
  `go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...` →
  tool cover 尾行 = **70.0%**，awk 门禁输出 "Coverage 70.0% meets
  70.0% baseline"，exit 0。
- golangci-lint（同 CI 版本配置）0 issues；gofmt 空。

## Risks / Notes

- 测试引用真实分支逻辑（action 枚举、dryRun 透传、幂等、finalizer），
  无凑数断言；dry-run 断言基于 patch 选项捕获而非 fake 落盘行为
  （fake client 不实现 dryRun 语义，那是正确的可观察量）。
