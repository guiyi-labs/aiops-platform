# gofmt 全量格式化修复

- Date: 2026-08-15
- Status: Complete
- Scope: 修复 CI `Check formatting` 阻塞：14 个 Go 文件对齐空白问题（93
  行插入/删除，零语义变化）。

## Context

Operator/CRD 增强（`fa03612`）推送后远端 CI `Check formatting` 失败，
根因是 `internal/inspection/model.go` 等既有文件未通过 `gofmt -l` 校验
（Go 1.26 gofmt 对 struct 字段对齐规则比旧版本更严格，这些文件在
M113-3 `dbee872` 时引入了非标准对齐）。

## What Changed

`gofmt -w` 修复以下 14 个文件的对齐空白（仅空格调整，无语义变化）：

- `internal/aiexplain/model.go`
- `internal/aiexplain/service_test.go`
- `internal/httpserver/incidentchat.go`
- `internal/httpserver/router.go`
- `internal/httpserver/router_harness_test.go`
- `internal/incident/context.go`
- `internal/incident/context_test.go`
- `internal/incidentchat/model.go`
- `internal/incidentchat/prompt.go`
- `internal/incidentchat/provider.go`
- `internal/incidentchat/service.go`
- `internal/incidentchat/service_test.go`
- `internal/incidentchat/summary.go`
- `internal/inspection/model.go`

## Verification

- `gofmt -l backend/`：输出为空（全量格式通过）。
- `go test -count=1` 对受影响的 5 个包全绿（aiexplain / incident /
  incidentchat / inspection / httpserver）。
- 变更均为 struct 字段对齐 / 函数签名对齐，无逻辑改动。

## Risks / Notes

- 仅空白变化，不影响任何运行时行为；后续 CI 的 `Check formatting`
  job 应不再失败。
