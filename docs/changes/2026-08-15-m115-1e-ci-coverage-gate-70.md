# M115-1e：CI 全局覆盖率门禁 65% → 70%

- Date: 2026-08-15
- Status: Complete
- Scope: M115 验收项：覆盖率 70% 门禁上调（此前 CHANGELOG 明确"暂保持，随
  后续切片累计完成后一并修改 ci.yml"）。

## Context

全局覆盖率经 M115-1a…1ac 切片累计实测 70.02%（26611 stmts，未覆盖 7979），
超过 70% 门槛，达到上调条件。

## What Changed

`.github/workflows/ci.yml` "Test and coverage baseline"：

- 门禁基线 65.0 → 70.0（awk 比较与报错文案同步更新）。
- 注释更新为 M115。

## Verification

- `go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...` +
  `go tool cover -func` 尾行：total 70.02%（> 70.0 门禁）。
- `go test -p=1 ./...`：72 包全绿。

## Risks / Notes

- 若后续切片新增被测代码导致总 stmts 上升而百分比回落至 70% 以下，门禁会
  如实报错——这正是 fail-closed 预期行为；核心包门禁（70%）不变。
