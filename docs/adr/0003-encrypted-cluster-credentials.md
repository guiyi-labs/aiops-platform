# ADR 0003: Encrypted Cluster Credentials And Restricted Import

- Status: Accepted
- Date: 2026-07-16

## Context

kubeconfig 可能包含 bearer token、客户端证书和私钥。直接保存或回显会造成高风险凭据泄漏；无限制解析 kubeconfig 中的文件路径或 `exec` 认证插件还可能让平台读取本地文件或执行命令。

## Decision

平台使用环境提供的 32 字节密钥，通过 AES-256-GCM 和随机 nonce 加密完整 kubeconfig，数据库同时保存密钥版本。API 从不返回 kubeconfig。

阶段 1 的导入器只接受绝对 HTTPS API 地址以及内嵌 token 或内嵌客户端证书。不读取 `certificate-authority`、`client-certificate`、`client-key` 文件路径，不执行 `exec`，不加载外部 auth-provider。

连接探测使用受超时约束的 HTTP 客户端访问 `/version`。进入核心资源查询阶段前再引入 client-go；当前网络无法访问 Go 模块代理，因此不在本阶段复制外部依赖或绑定本机 Kubernetes 源码目录。

## Consequences

- 数据库泄露不会直接暴露 kubeconfig 明文，密文篡改可被 GCM 校验发现。
- 平台无法直接导入依赖云厂商 exec 插件或本地文件路径的 kubeconfig，用户需要先生成最小权限、内嵌凭据的专用 kubeconfig。
- 密钥轮换需要按 `encryption_key_version` 增加重加密流程。
- 资源查询仍需在后续阶段接入 client-go，并保持当前凭据和注册表接口边界。
