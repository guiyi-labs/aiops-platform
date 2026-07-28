# ADR 0008: Sanitized Append-only Audit Trail

- Status: Accepted
- Date: 2026-07-17

## Context

平台写操作涉及认证、集群凭据、连接探测和诊断处置。仅依赖应用日志无法稳定按操作者、资源和请求 ID 查询；直接保存请求体又会把密码、token 或 kubeconfig 复制到审计库，扩大敏感数据暴露面。

## Decision

使用统一 HTTP 中间件识别固定写路由，在请求结束后同步追加结构化审计记录。审计字段来自白名单上下文：操作者 ID/名称、集群与资源引用、action、结果、请求 ID、HTTP 状态、来源地址、User-Agent、方法和路由模板。

结果语义固定为 success、failure、denied。审计表不提供更新或删除业务接口；用户和集群删除时外键置空，但名称、资源和数字 ID 快照保留。查询权限只授予系统管理员和安全审计员。

禁止审计请求体、Authorization、Cookie、密码、token、kubeconfig、Secret 内容和诊断备注。操作代码由服务端固定映射，不接受客户端自定义 action。

## Consequences

- 关键操作可按请求 ID、操作者、集群、action 和结果追溯。
- 敏感输入不会因审计而产生第二份持久化副本。
- 权限拒绝和业务失败与成功操作使用同一查询模型。
- 当前审计追加与业务事务不是原子提交；写入失败会记录结构化错误。需要严格原子保证时引入与业务事务同库的 outbox，再由投递器生成审计视图。
