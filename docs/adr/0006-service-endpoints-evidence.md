# ADR 0006: Service Endpoint Evidence Boundary

- Status: Superseded by ADR 0016
- Date: 2026-07-17

## Context

Service 没有可用后端可能来自 selector 不匹配、目标 Pod 未 Ready 或端口配置错误。单看 Service 对象无法证明当前是否存在可接收流量的地址。ExternalName 和无 selector Service 又可能不由 Kubernetes Service controller 自动管理后端，不能与普通 selector Service 使用同一判断条件。

## Decision

`service.no_ready_endpoints.v1` 只诊断带非空 selector、且不是 ExternalName 的 Service。证据读取使用同 Namespace、同名称的 core/v1 Endpoints，并统计 Ready 与 NotReady addresses；只有 Ready address 为 0 时命中。

Kubernetes Gateway 只增加固定的单资源 GET 路径，不开放任意 Endpoints 列表或透明代理。证据保存 Service 类型、selector、端口以及地址计数，不保存完整对象。

## Consequences

- 规则能稳定证明“当前没有 Ready 后端”，同时避免 ExternalName 和手工后端 Service 的明显误报。
- EndpointSlice 迁移与兼容回退已由 ADR 0016 完成；本 ADR 的 selector/ExternalName 排除和“只陈述可观察证据”边界继续有效。
- 规则不直接断言具体根因，selector、Pod Ready 状态和 targetPort 仍是需要进一步验证的假设。
