# M109：httpserver handler 覆盖率补测冲刺 — 全局 60.9% → 65.16%，达成 65% 门禁

- Date: 2026-08-13
- Status: Complete
- Scope: M109 工程卓越的覆盖率 65% 门禁达成。在 fuzz+重点包补测（60.9%）基础上，系统性为 httpserver 各 handler 补足错误分支（writeError 哨兵全表）、校验路径与成功/失败分支，将全局语句覆盖率抬至 65.16%。

## Context

M109 路线图要求全局覆盖率 60%→65%。此前 fuzz/重点包块升至 60.9%，剩余大头集中在 `internal/httpserver`（大量 handler 的解析校验、service 错误哨兵映射与 writeError 分支未覆盖）以及 `auth.go` / `incidents.go` 等文件。本轮以 handler 表测与分支测为主，覆盖 auth、servicemesh、topology、grants、incident 五个 handler 面，达成 65% 门禁。

## What Changed

### 测试文件（新增/扩展，全部在 `internal/httpserver/`）

- 首批 handler 表测（新文件）：`gitops_handler_test.go`、`aiexplain_handler_test.go`、`users_handler_test.go`、`maintenance_handler_test.go`、`copyops_handler_test.go`、`backup_handler_test.go`、`restore_handler_test.go`、`alert_handler_test.go`、`promotion_handler_test.go`、`remediation_handler_test.go`、`diagnosis_handler_test.go`、`slo_handler_test.go`——覆盖各文件 writeError 哨兵全表、校验路径与成功/错误分支。
- `auth_handler_test.go`（新）：复用 `userRepositoryStub`，覆盖 `auth.go` 全分支——login 成功/缺参/凭据错误/用户禁用/最后登录失败、refresh 缺 cookie/成功/禁用/失效、logout 成功/回写失败、me、changePassword（成功/当前密码错/未变化/过短新密码走 gin binding、默认错误）、sessions（成功/回读失败）、revokeSession（非法 id/0/成功/404/409 protected/409 required/默认 500）、revokeOtherSessions（成功/required/500）；另覆盖 `withAuthentication`（无 header/非法 token/用户不存在/禁用/auth_version 不匹配/成功）与 `requireRoles`（拒/放行）。
- `service mesh_test.go`（扩展）：动态 CRD stub 覆盖 VS/DR 列与详情（成功、Istio 未装 404、CRD 错误 500）、traffic-metrics（坏 window、top_k、成功、fail-soft）、`writeServiceMeshError` 全哨兵表。
- `topology_collapse_test.go`（扩展）：新增 `topologyRepoStub`（5 方法）+ `newTopologyRouter`，覆盖 `getTopologyGraph`（缺 cluster/坏 cluster/缺 namespace/坏 limit/负 limit/service 错/collapse 成功）、`listChangeEvents`（缺 cluster/坏 cluster/坏 start/坏 end/坏 limit/成功/service 错）。
- `grants_test.go`（扩展）：stub 增 `listClusterErr`/`listNamespaceErr`，覆盖 `listNamespaceGrants`（空/成功/500）、`listClusterGrants` 500、`myGrants` 两个错误分支、`deleteNamespaceGrant` 坏 cluster、`createNamespaceGrant` 空 namespace（gin binding 拦截）。
- `incidents_test.go`（扩展）：完成 `incidentRepoStub` 的 Transition/AddFollower/RemoveFollower/AddNote/SetPostmortem/Summary 并加错误注入，覆盖 summary 成功/500、transition 成功/非法状态/404/版本冲突/repo 500、addFollower 成功/404/500、removeFollower 成功/坏 user id/404/500、addNote 成功/404/空内容/500、setPostmortem 成功/404/500、export 成功/404。
- `users_handler_test.go`（扩展）：`userRepositoryStub` 增加 `findErr`/`rotateErr`/`revokeErr`/`changeErr`/`sessionsErr`/`revokeForUserErr`/`revokeOthersErr` 等注入字段供 auth/incident 复用（方法转到真实 service 语义）。

### 度量

- 全局语句覆盖率：**60.9% → 65.16%**（`go test -cover -p=1 -count=1 ./...`，25052 语句中 16323 covered）。
- 新增覆盖代表文件：auth.go、servicemesh.go、topology.go、grants.go 的未覆盖哨兵分支、incidents.go 的 transition/assign/follower/note/postmortem/summary/export 错误分支。

## Verification

- `cd backend && go test ./internal/httpserver/ -count=1`：全绿。
- `cd backend && go test ./... -count=1 -short`：无失败。
- `gofmt -l internal/httpserver/*_test.go`：无输出。
- 覆盖率：`go test -cover -p=1 -count=1 -coverprofile=/tmp/cov.out ./... && go tool cover -func=/tmp/cov.out | tail -1` = `total 65.2%`。

## Risks / Notes

- 65% 门禁虽达成，仍有明显可提点：GormRepository 各包（DB 绑定，现无 postgres 测试基建，Gorm 方法约 0%）、`cmd/server`（21.8%）、`internal/automation`（50%）等。建议后续引入 postgres 测试基建后再冲 70%+。
- 本轮未触碰并行 Agent 游离文件（`.github/workflows/ci.yml` 性能门禁等）；fuzz smoke 列表纳入 `./internal/incident/ ./internal/correlation/` 仍在并行轨收口清单。
- 性能门禁 fail-closed 属 M109 剩余块，依赖 CI 两个稳定周期样本，不在本轮。
