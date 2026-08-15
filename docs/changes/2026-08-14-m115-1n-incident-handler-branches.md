# M115-1n：incident 处理器分支全覆盖（list/metrics/batch/follower/note/postmortem/export）

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十四片：补齐 incident handler 所有低覆盖分支。

## Context

incidents.go 部分子处理器（list 52%、metrics 73%、runbook 60%、exportPostmortem
54%）仍有未覆盖校验/错误分支；测试引擎未注册 context/export/followers/notes/
postmortem 路由。

## What Changed

- `internal/httpserver/incidents_test.go`：
  - 测试引擎注册 context/export/addFollower/removeFollower/addNote/setPostmortem。
  - `TestIncidentHandler_ListValidationBranches`（10 个非法 query → 400 + 全过滤 happy path）。
  - `TestIncidentHandler_MetricsValidationBranches`（6 个非法 query → 400 + happy path）。
  - `TestIncidentHandler_BatchAssignBranches`（空/无 assignee/超长 comment 400 + 成功 200）。
  - `TestIncidentHandler_AddFollowerBranches`（404/400/201/409）。
  - `TestIncidentHandler_RemoveFollowerBranches`（400/200/404）。
  - `TestIncidentHandler_AddNoteBranches`（404/400/409/201）。
  - `TestIncidentHandler_SetPostmortemBranches`（超长 400/200）。
  - `TestIncidentHandler_ExportBranches`（export/postmortem 404 + 200）。
  - `seedIncidentForHandler` helper。

## Verification

- `go test ./internal/httpserver/`：全绿。
- incident 子处理器低覆盖分支（list/metrics/follower/note/postmortem/export）补齐。

## Risks / Notes

- incidentPostmortemRequest.Content 无 binding:"required"，content 为空合法；
  400 分支只能触发超长。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
