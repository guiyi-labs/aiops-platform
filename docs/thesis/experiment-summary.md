# 实验摘要（M115 答辩基线）

> 生成日期：2026-08-15 · 源码修订：`e7daa6f`（采集时工作树基线）
> 本节所有数字均注明测量时间、环境与方法，不把本地开发环境测量表述为生产基准。
> 相关：[[demo-scripts-outline.md]]、[[defense-demo-script.md]]、[[test-matrix.md]]。

## 1. 诊断覆盖与准确率

- **确定性规则复现**：内置 `demo-kube-mock` fixture 下，3 条规则全部命中且结论稳定：
  `node.not_ready.v1`（critical）、`pod.oom_killed.v1`（critical）、
  `deployment.replicas_unavailable.v1`（high）。本次答辩截图基线会话中重新触发
  3 条诊断并全部得到预期规则 ID、严重级、根因与建议（2026-08-15）。
- **端到端演练**：M102 demo drill 报告 `report-20260813-162314-8518b3.json`
  （`.artifacts/demo-drill/`）**41/41 断言全绿**：诊断创建 → 证据/replay →
  确认 → 受控处置 → 验证 → 事故 → 告警/巡检/信号/关联四类联动 → 幂等 → 清理。
- **自动测试覆盖**：M115 后端逐包平均覆盖率 ~74%（门禁全局 ≥65%、核心包 ≥70%），
  前端 143+ 用例全绿。覆盖对象是代码路径，不是生产数据分布。
- **诚实边界**：不宣称真实集群上的精确率/召回率；规则命中率仅在确定性 fixture 上
  可复现。真实集群现场演示见 `defense-demo-script.md`。

## 2. API P95 延迟（本地开发栈实测）

测量环境：k8s-aiops 本地 Compose 栈（PostgreSQL 17 容器 + Go 后端），本机 HTTP
回环，50 次采样，2026-08-15。方法：顺序请求、`urllib` 计时（ms）；未做预热/并发
加压，属于功能级延迟下限参考，不是压测基准。

| 端点 | p50 | p95 | p99 |
|---|---|---|---|
| `GET /api/v1/diagnoses?limit=10` | 1.58 ms | 3.54 ms | 13.3 ms |
| `GET /api/v1/clusters` | 1.63 ms | 2.12 ms | 2.24 ms |
| `GET /api/v1/health/ready` | 1.70 ms | 2.08 ms | 2.41 ms |

## 3. 受控操作幂等性（现场复测）

对已确认诊断 `pod.oom_killed.v1`（诊断 #2）发起 `deployment.rollout_restart`：

1. Preview 生成计划 `fca00e11-3bd1-400d-8791-54c4ff695b55`（`awaiting_confirmation`，10 分钟 TTL）。
2. Execute（携带 `Idempotency-Key: capture-20260815-001`）→ 计划 `succeeded`。
3. **同 key 重放** Execute → 返回同一计划 ID 与 `succeeded` 状态，**不产生第二次
   Kubernetes 变更**：mock `/mock/mutations` 记录 `total=1`，且 patch 仅含
   `k8s-aiops.local/remediation-id` + `restarted-at`（2026-08-15 实测）。
4. 与 M102 demo drill 既有证据一致（`action-verify`：mock 记录 1 次 mutation、
   remediation-id 唯一），跨两次环境复现幂等语义。

## 4. 资源成本（本地栈，2026-08-15 `docker stats`）

| 服务 | 内存 | CPU |
|---|---|---|
| `k8s-aiops-postgres-1` | 55.4 MiB | 0.05% |
| `k8s-aiops-backend-1` | 33.0 MiB | 0.00% |
| `k8s-aiops-frontend-1` | 3.3 MiB | 0.00% |
| `demo-mock`（演示 fixture） | 6.5 MiB | 0.00% |

平台三服务合计约 **92 MiB 内存**（不含 postgres 数据卷与镜像存储）；
源码与镜像约束见 `compose.yaml`（未设置硬内存 limit，生产环境建议按此实测基线
配置 request/limit）。

## 局限与后续

- P95 为本地单机功能级测量；生产建议补充并发压测（`k6`/`wrk`）与真实集群规模数据。
- 诊断覆盖率/准确率需真实集群现场数据支撑后，再写入论文结果章节。
- 资源成本未包含多集群联邦规模（本实验为 1 集群 + fixture）。
