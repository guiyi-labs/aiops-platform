# 变更归档：M7 确定性诊断规则扩展

- 日期：2026-07-26
- 阶段：M7 Diagnosis Rule Expansion
- 结果：实现完成，定向测试通过

## 目标

参考 KRM/Ratel 的资源状态展示能力和 KubeSphere 的可观测性方向，在不开放新的 Kubernetes 写操作、不改变既有诊断证据模型的前提下，补充两类高价值 Pod 故障。

## 交付内容

- 新增 `pod.pending.v1`：匹配 Pod `status.phase=Pending`，保存 Pod 状态、PodScheduled 条件和 FailedScheduling Event。
- 新增 `pod.oom_killed.v1`：匹配当前或上一次容器终止 reason 为 `OOMKilled`，保存退出码、信号、重启次数、终止时间和内存相关 Event。
- Pod 规则优先级调整为 Pending、OOMKilled、ImagePullBackOff、CrashLoopBackOff，避免 OOMKilled 同时进入 CrashLoopBackOff 时丢失更具体的根因线索。
- Workloads 页面新增 Pending/OOMKilled 诊断入口。
- 更新 API 文档、架构说明、测试矩阵、测试策略、交接文档和总体开发计划。

## 验证

- Go 1.25 容器定向执行 `go test ./internal/diagnosis` 通过。
- 新增 Pending 命中、Pending 非命中、OOMKilled 命中测试。
- 完整 `scripts/verify.ps1` 已通过：Go 1.25 容器全包 vet/test/build、前端 typecheck、8 个 Vitest 文件/26 个测试、生产构建、Compose 健康、Kustomize 16/5/7 和 HTTP 检查。证据为 `.artifacts/verification/verify-20260726-182546.json`。

## 范围边界

- M7 仍然是按需诊断，不引入后台 Watch 或定时扫描。
- M7 不增加通用 YAML 写入、Pod 删除、Pod Exec、任意 Patch 或新的受控处置动作。
- 真实 kind 演示场景继续保持三条稳定演示规则；Pending/OOMKilled 由单元和集成规则测试覆盖，避免改变既有演示环境清理契约。

## 后续建议

下一阶段优先接入资源指标和持续扫描，再扩展 NodeNotReady、Deployment 副本不足、PVC Pending 等规则。指标接入完成前，OOMKilled 的“内存压力”仍属于基于容器终止状态的可验证假设，不应在界面上表述为已经观测到的使用率。
