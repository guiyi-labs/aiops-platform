# M115-1q：appcatalog Get/List repo、GetPlan、validRepoURL、extractCredentials、构造器

- Date: 2026-08-14
- Status: Complete
- Scope: M115 冲刺第十七片：appcatalog 多个 0% 函数清零。

## Context

appcatalog/service.go 的 NewHTTPIndexSource/FetchIndex/NewService/NewTestService/
GetRepository/ListRepositories/GetPlan 全部 0%。

## What Changed

`internal/appcatalog/service_test.go` 新增：

- `TestGetRepository_SuccessAndNotFound`（含 999 → ErrRepoNotFound）。
- `TestListRepositories_Success`（2 个 repo）。
- `TestGetPlan_SuccessAndNotFound`（Preview 后 GetPlan + missing → ErrPlanNotFound）。
- `TestValidRepoURLEdges`（https 合法 / ftp / 过短 / 600 字符非法）。
- `TestExtractCredentialsBranches`（nil / 非法 JSON / 正常解析）。
- `TestNewServiceProductConstructors`（NewService/NewTestService/NewHTTPIndexSource）。

## Verification

- `go test ./internal/appcatalog/`：全绿。
- 四个 0% 函数清零；validRepoURL/extractCredentials 分支补齐。

## Risks / Notes

- validateDeployRequest 需要 RepoID/ChartVersion/TargetNamespace/ReleaseName 全合法；
  命名空间须预先存在（fake 仅预置 default）。
- 覆盖率门禁 ci.yml 65.0 仍未改（统一上调片稍后执行）。
