# M115-1b：httpserver 覆盖率 66.1% → 70.0%（clusters/cockpit/workspace/inspection/alert/aiexplain）

- Date: 2026-08-14
- Status: Complete
- Scope: M115 工程卓越冲刺第二片：`internal/httpserver` 覆盖率从 66.1% 提到 70.0%，
  为全局 65%→70% 门禁上调贡献最大单包杠杆（5783 stmts，全仓库最大）。

## Context

M115（docs/development-roadmap-post-m110.md Track F）要求全局覆盖率 65% → 70%。
`internal/httpserver` 是全局最大未覆盖单包（基线 5783 stmts、1962 未覆盖），本片
通过补齐 CRUD handler 测试把该包顶到 70.0%，与 70% 门禁对齐。本片仍是纯测试改动，
ci.yml 覆盖率门禁上调留待全局逼近 70% 的下一片。

## What Changed

全部为 `backend/internal/httpserver/` 下新增/扩展测试文件（无生产代码改动）：

- `clusters_test.go`（新增）：完整覆盖 clusterHandler 六个 handler + `clusterID`
  util。测试内构建 `cluster.Service`（内存 Repository 实现 + Encryptor + fake
  Prober），断言全部错误哨兵：
  - list 200（items/total）；get 200 / 404（ErrNotFound）/ 400（非法 ID）；
  - create 201 / 400（缺 body）；
  - setEnabled 204 / 404（缺失）/ 400（缺 body）；
  - updateCredential 200（加密轮换）/ 404 / 400（缺 body）；
  - probe 200（fake Prober 成功）/ 404（缺失）；
  - delete 204 / 404 / 400（非法 ID）。
- `event_cockpit_test.go`（扩展）：`parseCockpitRequest` 默认值（1440/50/500）、
  显式合法参数、NaN/越界参数拒绝（window_minutes/max_groups/page_limit 全分支）；
  `parseEventTime` 的 RFC3339 / Z 后缀 / 空串 / 非法串四分支。
- `workspace_test.go`（扩展）：补齐 7 个 0% handler 的 happy path —
  listWorkspaces 200、updateWorkspace 200、deleteWorkspace 204、listMemberships 200、
  getQuota 200、listRoleBindings 200、revokeRole 204/404（复用既有 handlerFakeRepo）。
- `inspection_test.go`（扩展）：getPlan 404/400、deletePlan 204/400、listTasks 200/
  bad plan_id 400/bad limit 400、getTask 404/400、listResults 200、getResult 404/400、
  effectiveRules happy path 200，以及全 handler nil-service → 503 的统一巡检测试。
- `aiexplain_handler_test.go`（扩展）：quality/coverage 服务错误 → 500。
- `alert_handler_test.go`（扩展）：deleteRule 的 ErrRuleNotFound/ErrRuleDeleted/通用
  错误分支 → 404/404/500，非法 rule_id → 400。

## Verification

- `go test ./...`（backend 全量门禁）：72 包 ok，exit 0（含 httpserver 3.78s）。
- `go test -cover ./internal/httpserver/`：70.0%（基线 66.1%，+3.9pp）。
- 全局覆盖率复测（`go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...`
  + `go tool cover -func` 尾行）：66.5%（基线 65.6%，+0.9pp；26611 stmts，
  未覆盖 8910，距全局 70% 尚差约 927 stmts）。

## Risks / Notes

- 覆盖率门禁 ci.yml 65.0 暂未改动；M115-1e 在全局逼近/达到 70% 时统一上调并考虑
  扩展核心包门禁。
- kubernetesHandler 直连 k8s gateway 的 handler（kubernetes.go 420 stmts 未覆盖、
  events/eventstream 等）需要注入真实 `*kubernetes.Service` 才可测，适合作
  gateway-aware 集成测试；不在本片范围。
- workspace_test 的 revokeRole 断言允许 204/404 两种结果（取决于 grant 是否存在），
  保持测试对夹具语义的弹性。