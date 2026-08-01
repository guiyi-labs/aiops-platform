# KubeSphere Reference Analysis

- Status: Accepted
- Reviewed: 2026-07-16
- Source: `<local-refs>/kubesphere-master`
- License: Apache License 2.0

## Valuable Patterns

### Ordered Request Filter Chain

KubeSphere 将请求解析、认证、授权、多集群分发、审计、指标和访问日志组织为明确的过滤链。授权属性来自统一请求信息，审计也复用同一上下文。这种设计能避免每个 Handler 重复解析资源和集群信息。

本项目采用：请求 ID、请求元数据、认证、授权、审计、指标/日志、Handler 的固定顺序。

### Cluster-Aware Request Context

KubeSphere 从 URL 提取 cluster、workspace、namespace、resource、verb 等信息，并把它放入 context。多集群转发、授权和审计都使用该信息。

本项目采用精简模型：request ID、actor、cluster ID、namespace、resource type、resource name、action。MVP 不引入 workspace 和多租户资源层级。

### Cluster Conditions

KubeSphere 的 ClusterStatus 使用 Kubernetes Condition 风格表达 Ready、AgentAvailable、证书即将过期等状态，并保留 reason、message 和 transition time。

本项目采用 Ready、Reachable、CredentialValid 三类 Condition，后续可增加 MetricsAvailable。数据库保留当前状态和最近变化时间，前端展示具体原因。

### Cached Cluster Clients

KubeSphere 按集群缓存 rest.Config、typed client、runtime client、Transport 和服务端版本，在集群配置改变时失效缓存，并设置 QPS/Burst 默认值。

本项目采用并发安全的 ClientRegistry。第一版缓存 rest.Config、ClientSet、Discovery 和 Metrics client；停用、删除或凭据变更时立即失效。每个请求禁止重复解析 kubeconfig 和创建 ClientSet。

### Unified Resource Query

KubeSphere 对分页、排序、字段过滤、label selector 和 field selector使用统一 Query 类型，并通过 ResourceManager 抽象 CRUD。

本项目采用统一的 ListQuery 与 ListResponse，但不会实现任意 CRD 的通用 CRUD。Node、Pod、Deployment、Service 和 Event 使用显式适配器，以保留清晰的权限和诊断语义。

### Authentication, Authorization And Audit Separation

KubeSphere 把认证、授权和审计放在独立包与过滤层中，包含 token、OIDC、RBAC、审计后端和相关测试。

本项目第一版只实现本地账号、JWT 和平台 RBAC，但保持 Authenticator、Authorizer、AuditSink 小接口，避免后续接入 OIDC 时修改业务 Handler。

### Engineering Quality

KubeSphere 使用 staging 模块、生成 API/client、vendor、单元测试、E2E、格式检查、依赖和许可证校验。其规模不适合直接复制，但质量门禁值得采用。

本项目门禁：Go fmt/vet/test/build、Vue typecheck/test/build、关键 API 集成测试、kind 场景测试、迁移校验和依赖许可证清单。

## Explicit Non-Goals

- 不复制 KubeSphere 源码或 API 兼容层。
- 不实现完整微内核、扩展市场、动态反向代理和 JS bundle 注入。
- 不实现 workspace、多租户层级、应用商店、DevOps 和 Service Mesh。
- 不使用 CRD 保存平台用户和 kubeconfig；MVP 继续使用 PostgreSQL 与应用层加密。
- 不实现跨集群 API 透明代理；前端和 API 显式传递 `cluster_id`，降低误路由风险。

## Influence On The Roadmap

在认证和集群接入之前，先完成请求 ID、统一错误、请求上下文与访问日志；集群接入阶段实现 Condition 和客户端注册表；资源阶段先实现统一查询；审计阶段捕获请求结果与耗时；MVP 稳定后再评估诊断规则插件接口。
