# ADR 0002: Modular Monolith And Ordered Request Pipeline

- Status: Accepted
- Date: 2026-07-16

## Context

KubeSphere 通过微内核、扩展组件和有序 API 过滤链支撑企业级平台。项目需要清晰的模块边界和扩展性，但没有足够范围支撑完整微内核、多进程控制器和动态扩展市场。

## Decision

平台采用模块化单体。认证、集群、Kubernetes 资源、诊断、AI 和审计保持独立包，通过小接口协作。HTTP 请求统一经过 Recovery、Request ID、Request Metadata、Authentication、Authorization、Audit、Metrics/Logging，再进入 Handler。

集群通过显式 `cluster_id` 路由，不做透明反向代理。诊断规则采用注册表接口，但第一版随主进程编译部署。

## Consequences

- 单进程便于本科项目开发、测试和部署。
- 横切逻辑集中，业务 Handler 更容易测试和审计。
- 模块可在后续确有需要时拆分，不提前承担分布式复杂度。
- 不具备 KubeSphere 运行时安装扩展的能力，这是有意的范围控制。
