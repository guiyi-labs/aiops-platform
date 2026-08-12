# M100-C：敏感字段静态扫描与日志/审计脱敏门禁

- Date: 2026-08-12
- Status: Complete
- Scope: M100 第三步——按路线“审计完整性、导出脱敏、敏感字段静态扫描和日志脱敏门禁”落地：全仓库敏感字段静态扫描门禁（CI + 本地）、请求日志/审计条目/审计导出的脱敏契约测试，并以运行时证据验证 Secret/密码/token 不进入日志与审计导出。

## Context

M100 验收要求“Secret 值、kubeconfig、token、Cookie 和密码不进入 API、审计、日志、artifact 或前端状态”。存量实现已具备良好基础（请求日志只记录 method/path/status 等路由元数据、审计条目只存 actor/action/resource 元数据、cluster 凭据加密存储、前端 access token 仅存内存 Pinia store、`.env` gitignore、审计 CSV 与事故 CSV 均做公式中和），但缺少两件事：一是**没有静态扫描门禁**来持续防止密钥/凭据混入仓库；二是**没有契约测试**把这些脱敏行为固化为门禁，防止未来改动回归。

## What Changed

### 敏感字段静态扫描门禁

- `scripts/scan-sensitive-fields.sh`：扫描全部 git-tracked 文件，检出并 fail-closed 以下类别：
  - 私钥块（`BEGIN ... PRIVATE KEY`）；
  - 内联 kubeconfig 凭据数据（`client-certificate-data`/`client-key-data`/`certificate-authority-data` 后跟 base64 值）；
  - 云/API token 格式（`AKIA…`、`ghp_…`、`xox[baprs]-…`、`sk-…`、`AIza…`）；
  - JWT（三段式 `eyJ…`）；
  - 被跟踪的凭据/密钥文件（`.pem`/`.key`/`.p12`/`.pfx`/`.jks`/`.keystore`）；
  - 非占位符的 `PASSWORD=` 赋值（占位符/运行时变量/模板引用豁免：`change_me*`、`example`、`***`、`${{…}}`、`${…}`、`$(…)`、`$Var`、`<…>` 等）。
- `scripts/scan-sensitive-fields.allowlist`：按 `path:lineno` 或裸 `path` 的精确豁免清单（带注释说明），当前为空模板（现有树 0 命中）。
- `.github/workflows/ci.yml`：新增 `sensitive-scan` job，任何变更（含文档-only）都跑扫描门禁。
- 负向自测：临时注入私钥/kubeconfig/token/JWT/密码样例均被正确检出（含 `sk-ant-…` 连字符格式），真实树 1167 个跟踪文件 0 命中。

### 日志/审计脱敏契约测试（backend/internal/httpserver/log_redaction_test.go）

- `TestRequestLoggerOmitsSensitiveMaterial`：请求日志只允许 method/path/status/duration/response_size/client_ip/request_id 路由元数据；带 `?token=`/`?password=` query、`Authorization: Bearer`、`aiops_refresh_token` Cookie、JSON body 的请求，日志断言不出现任何凭据值与敏感字段名。
- `TestAuditTrailEntriesNeverCarryRequestCredentials`：审计中间件对 `auth.password.change` 记录条目断言——body 密码、Bearer token 不进审计条目，`Details` 是固定闭集（method/path_template/cluster_id）。
- `TestAuditCSVExportRedactsAndNeutralizesSensitiveCells`：`GET /audit-logs/export` 全链路断言 CSV 不含任何凭据值（公式中和由 audit 包既有测试覆盖）。

### 存量检查结论（无改动项）

- 前端 access token 仅存内存 Pinia store，localStorage 只存 UI 偏好；refresh token 为 HttpOnly Cookie。
- 请求日志不记录 query string；无 `GetRawData`/body 日志；OIDC 初始化只记 issuer/client_id（不记 client_secret）。
- kubeconfig 解析错误不回显原文；cluster 凭据加密存储（M28 起）。

## Verification

- Backend 门禁：`go test ./...`（httpserver 新增 3 个脱敏契约测试）、`go vet ./...`、`golangci-lint run ./...` 全绿（0 issues）。
- 扫描门禁：`scripts/scan-sensitive-fields.sh` 真实树 clean（1167 tracked files）；负向样例 5 类全部检出并退出码 1。
- 运行时（重建 backend 镜像，容器 healthy）：
  - M100-B 角色变更失效在运行时复验：viewer 登录 → 管理员改角色 `operations_admin` → 旧 access token `/me` 401 → 重新登录 200（此前旧 token 会继续生效）。
  - 脱敏验证：`docker compose logs backend` 对冒烟密码/管理密码 0 命中；`/api/v1/audit-logs/export`（190 行）0 命中；请求日志仅含路由元数据。
  - 冒烟用户已禁用清理。

## Risks / Notes

- 扫描器是启发式匹配：新出现的敏感格式需要补充 pattern；豁免请走 allowlist 并注明理由。
- 密码赋值检测只覆盖 `PASSWORD=` 形式（含 `export` 前缀）；`SECRET=`/`TOKEN=` 形式暂不纳入，避免误报面过大，后续如需要可加。
- 日志脱敏为“不记录”策略（结构性安全）而非“记录后打码”；若未来引入 body 日志，必须同步更新本契约测试。
- 下一步：M100-D（依赖漏洞、许可证、镜像基础层与 SBOM 差异报告；critical 默认 fail-closed）。
