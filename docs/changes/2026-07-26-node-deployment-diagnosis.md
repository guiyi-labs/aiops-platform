# M8 Node 与 Deployment 健康诊断

- 日期：2026-07-26
- 里程碑：M8
- 范围：确定性诊断规则、Kubernetes 只读查询、诊断 API、工作负载页、测试与契约文档

## 目标

在已有 Pod/Service 诊断闭环上补充集群节点与 Deployment 健康信号，继续复用同一条证据、SLA、审计、AI 解释和受控处置链路。M8 不增加任意 Kubernetes 写入能力。

## 交付内容

1. 新增 `node.not_ready.v1`：读取 Node 的 `Ready` Condition；Condition 缺失或状态不是 `True` 时命中，保存所有 Conditions 的状态、Reason、Message 与 LastTransitionTime。
2. 新增 `deployment.replicas_unavailable.v1`：使用 Deployment 的期望、实际、Ready、Available、Updated、Unavailable 副本计数；Ready 或 Available 未达到期望时命中。
3. Kubernetes 服务新增按名称读取 Node 的只读方法，诊断 Source 扩展 Node/Deployment 查询能力。
4. HTTP 诊断入口支持 `Pod`、`Service`、`Node`、`Deployment`；Node 允许空 Namespace，其余资源仍要求 Namespace。
5. Workloads 页面为 Node 和 Deployment 增加诊断按钮，并展示 Node Ready 状态。

## 验证

- 后端规则单元测试覆盖命中、健康跳过、Ready Condition 缺失、Deployment 默认副本数。
- Kubernetes gateway 测试覆盖 Node 资源路径。
- 前端 API 测试覆盖 Node/Deployment 请求序列化。
- OpenAPI 与 Gin 路由契约继续由既有门禁校验。
- 完整 `scripts/verify.ps1` 于 2026-07-26 19:05:40 +08:00 通过：Go 1.25 Docker 工具链全包 vet/test/build、前端 typecheck、8 个 Vitest 文件/27 个测试、生产构建、Compose 三服务健康、Kustomize 16/5/7 和 HTTP 健康检查全部通过。证据为 `.artifacts/verification/verify-20260726-190540.json`。

## 风险与边界

- Node NotReady 只说明控制面观察到的 Ready Condition 异常，不替代节点日志、事件和运行时检查。
- Deployment 副本规则不直接推断具体 Pod 根因，需从证据面继续进入 Pod 诊断。
- 本阶段未修改三个既有 kind 演示故障场景；新增 Node/Deployment 规则已通过单元、契约、构建和 Compose 门禁，真实故障注入留到后续独立 E2E 扩展。
- 不扩展任意 YAML、Patch、Exec 或 WebShell；现有 rollout restart 白名单保持不变。

## 归档索引

- 实施代码：`backend/internal/diagnosis/node_not_ready.go`、`backend/internal/diagnosis/deployment_replicas_unavailable.go`
- API 契约：`docs/api/openapi.yaml`、`docs/api/README.md`
- 后续验证结果：见 `docs/development-handoff.md` 与 `.artifacts/verification/`
