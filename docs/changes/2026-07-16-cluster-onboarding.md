# 2026-07-16 Cluster Onboarding Baseline

## Scope

- 增加 AES-256-GCM kubeconfig 加密与密钥版本。
- 增加受限 kubeconfig 导入器，禁止本地路径、exec 和外部 auth-provider。
- 增加集群 CRUD、启停、`/version` 探测和客户端缓存失效。
- 增加 `Ready`、`CredentialValid`、`Reachable` Conditions 及迁移。
- 集群 API 接入四类角色权限；只有系统管理员能导入、启停和删除，运维管理员可探测。
- 前端增加集群列表、接入表单、探测、启停和删除入口，凭据提交后立即清空且不回显。

## Verification

- Go：`go test ./...` 和 `go build ./cmd/server` 通过。
- 后端测试覆盖 AES-GCM 随机 nonce 与往返解密、kubeconfig 校验、TLS 测试服务器探测和角色拒绝。
- 前端：`pnpm typecheck`、Vitest 和 `pnpm build` 通过。
- PostgreSQL 17 真实迁移成功；真实 API 验证创建、列表、离线探测、启用和删除均成功。
- 列表响应不包含 kubeconfig 或测试 token；数据库密文字节中不存在测试 token 明文。
- 离线 API Server 被记录为 `unreachable`，同时生成 3 个可追溯 Conditions。

## Environment Limitations

- 本机 kubeconfig 当前没有 context，kind 未安装，因此尚未完成真实 Kubernetes `/version` 成功探测。
- `proxy.golang.org` 当前不可达，client-go 留待核心资源查询阶段安装；本阶段未引入不可移植的本机源码依赖。

## Deferred

- 更新 kubeconfig 与密钥轮换。
- 使用 client-go 查询 Node、Namespace、Pod、Deployment 和 Service。
- 集群变更与连接探测审计落库。
