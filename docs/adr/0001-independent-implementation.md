# ADR 0001: Build An Independent Implementation

- Status: Accepted
- Date: 2026-07-16

## Context

参考目录中的 KRM 和 Ratel 仓库只包含文档、截图、部署 YAML 和容器镜像地址，不包含可构建的 Go/Vue 应用源码。Ratel 已停止维护，部分认证方式和 Kubernetes API 已过时。

## Decision

建立独立的 Go/Vue 工程。参考项目只用于提炼多集群资源管理、RBAC、资源操作和页面交互需求。平台的认证、凭据管理、诊断、AI、审计和测试由本项目自主实现。

## Consequences

- 可以使用当前依赖和安全实践，形成清晰的原创提交记录。
- 无法直接继承参考项目的完整功能，需要严格限制 MVP。
- 论文和项目文档必须明确列出参考来源，不把截图或镜像功能描述为自主实现。
