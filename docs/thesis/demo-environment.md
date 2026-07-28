# Defense Demo Environment

更新时间：2026-07-26

此流程用于答辩排练、最终截图和现场演示。它复用真实 kind E2E 主链路，不引入静态假数据或绕过平台 API 的数据库写入。

## Preparation

```powershell
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-up.ps1
```

准备脚本会先删除旧的 `demo-kind-*` 平台集群，再执行完整 E2E：应用 `aiops-demo` 故障资源和最小 RBAC、创建一小时 short-lived credential、接入并探测真实 kind、生成三条诊断、确认 ImagePullBackOff 诊断、执行并幂等重放 rollout restart。仅当全部步骤成功时保留平台集群和关联历史。

准备完成后：

- Web UI：`http://localhost:18080`。
- 集群页存在一个 `demo-kind-<timestamp>`，状态为 Ready。
- 诊断历史包含 ImagePullBackOff、CrashLoopBackOff 和 Service 无就绪端点。
- ImagePullBackOff 记录为 confirmed，并带 succeeded remediation 历史。
- 脱敏证据位于 `.artifacts/demo` 和 `.artifacts/e2e-kind`。

生成论文/答辩截图：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\capture-thesis-screenshots.ps1
```

截图保存到 `docs/thesis/screenshots`。采集器使用系统 Edge/Chrome，结束后删除 `.artifacts` 下的临时浏览器 profile。

凭据原文只在脚本内存和 HTTPS 请求期间存在。平台数据库保存 AES-256-GCM 密文，证据和文档不记录 kubeconfig、CA、token、密码、访问令牌或 Cookie。由于 ServiceAccount token 有效期为一小时，正式演示前应重新运行准备脚本。

## Cleanup

仅删除平台中的演示集群、级联诊断和处置记录，保留 kind 资源供再次准备：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-down.ps1
```

同时删除 `aiops-demo` Namespace 和目标集群 RBAC：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-down.ps1 -CleanupDemoResources
```

Cleanup 只匹配 `demo-kind-*`，不会删除用户手工接入的其他集群。平台集群删除通过正式 API 完成，因此加密凭据、诊断证据和处置计划按数据库外键级联清理，审计快照继续保留且不包含凭据。

## Failure behavior

- `demo-up` 启动时先清理上一次遗留的演示平台记录，避免重复数据。
- E2E 失败时，即使启用了保留模式，也会在 `finally` 删除本次临时平台集群。
- `KeepPlatformCluster` 与 `CleanupDemoResources` 不能同时使用。
- 准备成功后必须使用 `demo-down` 结束演示；不要依赖 token 自然过期代替凭据清理。
