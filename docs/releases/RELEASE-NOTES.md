# Release Notes — v0.3.0-rc.6（RC）

> 定位：**发布候选（RC）**。未完成 M89（生产 OIDC/MFA）与 M90（WAL/PITR/HA 组织级验收）
> 前不宣称 GA；本版本为展示与部署评估基线。
> 基线：M115 工程质量冲刺 · `baseline-m115-20260815` · 关联 Issue #16 / #17 / #18。

## 这个版本提供什么

多集群可观测、证据型诊断、事故响应与受控运维平台（Day 2 Kubernetes 运行期）：

- **多集群与多租户**：三层控制台（平台 → 集群 → 工作区）+ 集群/Namespace 2D 授权，未授权资源返回 404。
- **全栈可观测**：监控大盘、日志探索、事件驾驶舱（M114）、SLO burn 总览与告警降噪（M114）、7 天精确指标历史 + 30 天下采样归档（M114）。
- **证据型诊断**：确定性规则 + 不可变证据快照，AI 仅引用式解释增强（`AI_ENABLED=false` 时确定性降级）；智能巡检趋势与规则命中覆盖率（M113）。
- **优化中心闭环**：18 个只读分析器 + finding→runbook 预览导航 + 容量感知 dry-run 预览（M113）。
- **事故响应**：工作空间、SLA/MTTA/MTTR、升级通知、Runbook、复盘 Markdown 导出、三联动与关联归一。
- **受控运维**：固定操作目录（rollout restart / scale / image update / rollback / cronjob suspend-resume），统一 dry-run + 确认 + 幂等 + 审计。
- **质量基线**：后端逐包平均覆盖率 ~74%（全局 ≥65%、核心包 ≥70%）；前端 typecheck/lint/test/build、axe 双视口、截图基线 verify、`pnpm ui:gate` 4/4。

## 兼容范围

| 组件 | 支持版本 | 验证方式 |
|---|---|---|
| Kubernetes | 1.34 – 1.36 | kind 集群（1.34.0/1.36.0）+ real-kind E2E（CI） |
| PostgreSQL | 17（pgvector 0.8.1） | Compose/Helm 部署 + 备份恢复演练 |
| 部署路径 | Docker Compose / Helm 3 / Kustomize | CI 校验 + 离线安装演练 |
| 浏览器 | 现代 Chrome / Edge | 前端双视口 + axe |
| 镜像平台 | linux/amd64、linux/arm64 | 多架构 OCI + SBOM + Cosign |

## 升级说明（从 rc.5 / rc.5-replay → rc.6）

1. **先备份**：逻辑备份（`pg_dump`）或启用 WAL/PITR 归档；演练记录 RPO≤2s、RTO≈1.2–2.7s（本地观测值，非生产声明）。
2. **替换镜像**：后端/前端指向 rc.6 digest；`helm upgrade` 或 Kustomize 滚动替换。
3. **校验**：`/api/v1/health/ready` 返回 `version`；审计 marker（数据面标记）保持不变；关键旅程冒烟（登录、资源浏览、诊断）。
4. 双环境演练已覆盖跨 digest 升级（`dev → rc.5-replay` 全链路 PASS），审计标记保持。

## 回滚说明

- 回滚到上一稳定基线镜像 digest；`/api/v1/health/ready` version 复原，审计/持久化 marker 保持。
- 升级后如遇数据契约不兼容（迁移失败），先恢复备份再回滚；本地双环境演练验证过备份恢复路径。

## 已知限制（发布前须知）

- 生产 OIDC/MFA 与组织级 WAL/PITR/HA 未授权验收（M89/M90，GA Gate D 前置）；项目保持 RC 口径。
- `demo-kube-mock` 仅用于演示/演练 fixture，不承载真实工作负载验证。
- 集群资源实时查询 API Server，不落库；无网络时相关视图降级。

详细变更历史见 [`CHANGELOG.md`](../../CHANGELOG.md)，验收清单见
`docs/authorization-gate-prep.md` 与 `docs/release-candidate-operations.md`。
