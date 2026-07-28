# ADR 0004: Bounded Read-Only Kubernetes Gateway

- Status: Accepted
- Date: 2026-07-17

## Context

阶段 2 需要尽早验证“加密集群凭据 → 真实 API 请求 → Pod/Event/日志 → 前端展示”的纵向链路。当前环境无法访问 Go 模块代理，无法可靠安装 client-go；绑定本机 Kubernetes 源码或复制 vendor 会让项目不可移植。

## Decision

先实现范围严格受限的只读 HTTP Gateway，仅支持服务端固定构造的 core/v1 Node、Namespace、Pod、Service、Event、Pod log，apps/v1 Deployment 和 `/version` GET 请求。Gateway 复用集群注册表中的 TLS 与认证配置，并限制超时、User-Agent、路径格式和响应体大小。

不提供任意 URL、任意方法或透明反向代理。受控变更、watch/informer 和完整 Kubernetes 类型兼容进入下一阶段时，优先替换为 client-go；领域 Service 与 HTTP Handler 保持接口隔离，减少替换成本。

## Consequences

- 可以在不引入不可移植依赖的情况下完成首条真实资源闭环。
- 当前结构体只覆盖页面与诊断所需字段，不是 Kubernetes API 的完整类型定义。
- 列表在 API Server selector 过滤后做本地名称过滤和分页，不适合大规模集群。
- 在进入写操作前必须完成 client-go 依赖接入和 fake client 测试，不能扩展该 Gateway 为通用代理。
