# 2026-07-17 Administrator Password Reset

## Scope

- 新增 `POST /api/v1/users/{user_id}/password-reset`，仅系统管理员可调用。
- 新增 `users.auth_version` 迁移，并将版本写入访问令牌。
- 密码重置事务同时保存 bcrypt 摘要、递增凭据版本并撤销全部刷新会话。
- 认证实时比较令牌版本和数据库版本，使旧访问令牌立即失效。
- 禁止管理员通过管理接口重置自身密码；新密码要求 12–128 字符。
- 新增 `user.password.reset` 审计映射，不保存密码或请求体。
- 用户管理页增加“重置密码”入口、长度约束、会话失效说明和结果反馈。
- 新增可跨会话维护的 `docs/development-handoff.md`。

## Verification

- Go 聚焦测试覆盖凭据版本、旧版本拒绝、bcrypt 重置摘要、自我保护和审计路由。
- 前端类型检查通过；7 个 Vitest 文件共 20 项测试通过，新增密码重置请求契约测试。
- 真实 PostgreSQL/API 验证：重置后 `auth_version` 从 1 增至 2；旧访问令牌与旧刷新会话返回 401；旧密码登录返回 401；新密码登录返回 200；管理员自我重置返回 409。
- 审计查询确认一条 success/200 与一条 failure/409 的 `user.password.reset` 记录。
- 最终全量测试和生产构建全部通过。

## Boundaries

- 没有用户主动改密、邮件找回、一次性重置链接、强制首次改密、MFA 或 SSO。
- 管理员必须通过安全的带外渠道把临时密码交给用户；平台不发送通知。
- 密码策略目前只约束长度，不做字符类别或泄漏密码库检查。
