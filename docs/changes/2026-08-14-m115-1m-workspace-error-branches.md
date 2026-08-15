# M115-1m：workspace 处理器错误分支测试

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十三片：workspace handler 错误/校验分支（nil service 503、
  membership 冲突/缺失、role grant 缺失、duplicate 409）。

## Context

workspace handler 多为 36-70% 低覆盖（错误分支未测）；handlerFakeRepo 缺乏
membership/grant 错误注入。

## What Changed

- `internal/httpserver/workspace_test.go`：handlerFakeRepo 增加
  addMembershipErr/removeMembershipErr/grantErr 错误注入。
- 新测试：
  - `TestWorkspaceHandler_NilServiceReturnsUnavailable`（全部 13 路由 503）。
  - `TestWorkspaceHandler_ListWorkspacesWithDisplayNameFilter`。
  - `TestWorkspaceHandler_AddMembershipAlreadyExists`（409）。
  - `TestWorkspaceHandler_RemoveMembershipNotFound`（404）/
    `TestWorkspaceHandler_RemoveMembershipNoClusterQuery`（400）。
  - `TestWorkspaceHandler_CreateDuplicateNameReturnsConflict`（409）。
  - `TestWorkspaceHandler_GetWorkspaceNotFound`（404）。
  - `TestWorkspaceHandler_GrantRoleNotFound`（404）。

## Verification

- `go test ./internal/httpserver/`：全绿。
- workspace.go removeMembership 36.8% → 100%、其余错误分支补齐。

## Risks / Notes

- seedWorkspaceForHandler 内部会 CreateGrant，错误注入须在 seed 之后设置。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
