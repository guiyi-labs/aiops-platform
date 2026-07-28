# 2026-07-17 Platform Audit Trail

## Scope

- 新增 `000006_audit_trail`，扩展操作者快照、HTTP 状态、来源和查询索引。
- 实现 audit 领域模型、PostgreSQL Repository、查询 Service 和受控列表 API。
- 统一中间件记录认证、集群与诊断写操作的 success、failure、denied 结果。
- 登录/刷新成功后补充实际操作者；处理器只补充资源名称和数字标识。
- 审计列表限制为系统管理员和安全审计员。
- 新增审计中心页面，支持集群、操作、结果筛选和详情抽屉。
- 删除集群后保留操作者、资源、请求 ID 和集群 ID 快照。

## Verification

- `go test ./...` 与服务端构建通过，审计测试覆盖 action、结果映射、拒绝记录和读请求排除。
- `pnpm typecheck`、6 个 Vitest 文件共 12 项测试、`pnpm build` 通过。
- PostgreSQL 真实应用 `000006`，四个新增字段和迁移记录存在。
- API 产生 5 条核心验证记录：3 条 success、1 条 failure、1 条 denied；浏览器会话刷新另追加 1 条 success。
- 成功登录的操作者为 System Administrator；无令牌集群创建为 HTTP 401 denied；非法 kubeconfig 为 HTTP 400 failure。
- 成功创建并删除测试集群后，删除审计的 cluster_id 外键为空，`details.cluster_id` 保留原 ID。
- 浏览器验证全部记录、结果筛选、详情中的请求 ID、HTTP 状态、来源、User-Agent 和非敏感 JSON。
- 测试完成后清理审计记录、测试资源和临时服务。

## Boundaries

- 审计不保存请求体、凭据、Cookie、Authorization 或诊断备注。
- 当前为同步追加但非跨业务事务原子；后续强一致方案采用事务 outbox。
- 审计中心的安全 CSV 导出已由同日后续变更完成；保留策略和签名校验尚未实现。
- 平台业务仍不执行自动 Kubernetes 修复。

## Deferred

- 审计 CSV 导出已在同日后续变更完成；保留周期、归档分区和完整性签名仍待实现。
- 事务 outbox 与审计写入失败告警。
- 诊断批量操作和通知；负责人转派与 SLA 已在后续变更 `2026-07-17-diagnosis-sla-assignment.md` 完成。
- AI Provider 与引用式解释已在后续变更 `2026-07-17-cited-ai-explanations.md` 完成。
