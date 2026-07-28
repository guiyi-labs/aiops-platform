# 10 分钟答辩演示脚本

目标：在 10 分钟内证明“真实多集群数据读取 -> 可解释规则诊断 -> 人工确认 -> 受控处置 -> 审计追溯”的主链路，而不是逐页介绍全部功能。

## 演示前准备

1. 运行 `.\scripts\verify.ps1 -SkipComposeBuild`，确认 Compose 三服务 healthy。
2. 确认 `kubectl --context kind-aiops-test get nodes` 成功。
3. 运行 `.\scripts\demo-up.ps1`，准备并保留真实集群、三条诊断和处置历史。
4. 浏览器打开 `http://localhost:18080`，使用本地开发管理员登录；不要在投屏或文档中展示密码。
5. 准备 `.artifacts/demo` 和 `.artifacts/e2e-kind` 的最新脱敏 JSON 作为网络波动时的备用证据。
6. 演示完成后运行 `.\scripts\demo-down.ps1`；需要释放 kind 资源时增加 `-CleanupDemoResources`。

## 时间安排

| 时间 | 操作 | 要表达的结论 |
|---:|---|---|
| 0:00-0:40 | 展示登录页并进入 Dashboard | 平台是可运行系统，角色和会话由后端数据库管理 |
| 0:40-1:30 | 打开集群页，展示 Ready、Reachable、CredentialValid | kubeconfig 加密保存；状态不是单一“在线”，而是 Kubernetes Condition 风格 |
| 1:30-2:30 | 进入 `aiops-demo` Pod/Service 列表 | 资源来自真实 Kubernetes API，不是写死或复制到平台数据库的数据 |
| 2:30-4:10 | 对 ImagePullBackOff Pod 发起诊断并打开详情 | 展示规则 ID、严重级别、Event/状态证据，解释“事实证据”和“可能根因”的边界 |
| 4:10-5:10 | 快速展示 CrashLoopBackOff 和 Service 无端点历史 | 同一引擎覆盖三类可复现异常，CrashLoopBackOff 使用上次终止状态，Service 优先读 EndpointSlice |
| 5:10-6:10 | 将 ImagePullBackOff 诊断流转为 confirmed | 规则结果不可变；人工状态、负责人、反馈以追加历史保存 |
| 6:10-7:50 | 请求 Deployment rollout restart 预演并二次确认 | 服务端先 dry-run，只允许固定动作；执行受 token、resourceVersion、TTL 和幂等键保护 |
| 7:50-8:30 | 刷新处置历史并说明同键重放 | 同一幂等键返回同一计划，不产生第二次 Kubernetes patch |
| 8:30-9:20 | 打开审计页筛选本次诊断/处置 | 操作者、资源、结果、请求 ID 可追溯，凭据和请求体不进入审计 |
| 9:20-10:00 | 展示测试矩阵和最新 E2E 证据摘要 | 结论来自自动测试和真实 kind 验收，AI 关闭或失败不影响规则主链路 |

## 讲解主线

建议始终围绕三个问题回答：

1. **怎么发现问题**：平台按 `cluster_id` 访问目标 Kubernetes API，采集当前资源状态、Event、日志或 EndpointSlice。
2. **为什么可信**：确定性规则先产出版本化结论，每条结论绑定可追溯证据；AI 只做引用式解释增强。
3. **怎么避免误操作**：平台 RBAC 与目标集群 RBAC 双重限制，处置必须 confirmed、dry-run、二次确认、幂等执行并审计。

## 异常预案

- kind 短时不可达：展示最近一次 `.artifacts/e2e-kind/*.json` 和 `docs/changes` 记录，但明确说明这是历史证据。
- 故障 Pod 尚未进入目标等待状态：等待控制器重试，不临时修改规则或伪造页面数据。
- ImagePullBackOff 无法通过 restart 修复：这是预期现象。rollout restart 用于证明受控写路径，不声称它能修复错误镜像地址。
- AI Provider 不可用：直接展示确定性诊断；这正是“规则为主、AI 为辅”的降级设计。
- 时间不足：优先保留 ImagePullBackOff 诊断、受控处置和审计三段，跳过逐页浏览。
