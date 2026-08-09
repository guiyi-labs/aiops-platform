# 后续长远计划：把 AIOps 平台打磨到业内最高水准

- Status: Active（待评审）
- Updated: 2026-08-09
- Baseline: `main` @ `c53bc06`（M91 已提交，尚未推送）
- 打磨契约见 [`docs/polish-plan.md`](polish-plan.md)；分阶段路线见 [`docs/long-term-roadmap.md`](long-term-roadmap.md)。
- 原则：不推翻既有架构决策与安全边界；每步带可验证证据；授权等外部依赖如实记录缺口。

## 0. 现状一句话

模块化单体 + 44 个后端 domain 包 + 34 个前端视图 + 18 个分析器，CI 全绿、真实集群 kind E2E 通过；待办集中在文档收口、m91 推送、P3 授权缺口。
## 1. 文档与基线收口（第一优先）

- 现状：m91 `c53bc06` 未推送（ahead 1）；CHANGELOG 未收录 W11/M88/W12/M91；PROJECT_STATUS 基线停在 70c314c；roadmap 文首未对齐。
- 动作：补 CHANGELOG 条目；刷新 PROJECT_STATUS 与 roadmap；打 tag `baseline-m91-20260809` 并推送。
- 验收：工作树干净，与 origin/main 同步 0/0，文档基线 = 真实 HEAD。
## 2. 里程碑后续路线（P0→P3）

### P0 已收口
W8/W9 覆盖率 60%+Playwright；W10 OpenAPI；W11 a11y+bundle；W12 kind E2E 证据；M88 发布本地化。

### P1 AIOps 差异化深耕（主线）
- 诊断友好化：根因卡片 + 证据引用 + 可回放演示叙事。
- 洞察可解释分层：规则→证据→建议 三层下钻。
- 大规模拓扑渲染：500 节点 fixture + kind 验证（与 M91 联动）。
- 每项：ADR→实现→单测→verify-fast→kind E2E→change-record。

### P2 工程卓越夯实
- 全局覆盖率 60%→70%，不改契约。
- 性能基准：100k 事件窗口、500 节点/50k pod fixture，入 CI。
- CI artifact 归档 + 覆盖率链接；E2E 超时与资源上限；go test -race 可控。
### P3 交付与生产就绪（组织授权）
- M89 生产身份：OIDC/MFA 对接（M26B 前置审批）。
- M90 数据可靠性：WAL→PITR 演练→多副本停机→回滚演练。
- M88 剩余：GitHub Release 创建 / Fulcio keyless 签名。

## 3. 五条可度量维度收尾目标

| 维度 | 目标 |
|---|---|
| 供应链交付 | 正式版本 + SHA256SUMS/签名/provenance 可验 + 离线包 |
| 安全身份 | OIDC/MFA + 会话策略（组织授权） |
| 数据可靠 | WAL + PITR + 多副本演练常态化 |
| AIOps 差异 | 聚合洞察 + 端到端闭环全场景覆盖 |
| 工程卓越 | 全局覆盖率 70%+、P95 基准入 CI |
## 4. 候选方向探索（长期）

| 方向 | 说明 | 前置 |
|---|---|---|
| 可观测性深化 | 日志/指标联动、告警降噪、SLO 扩展 | M91 |
| 供应链可验证 | SBOM（CycloneDX）入库、依赖扫描入 CI | M88 |
| 身份升级 | OIDC/JWKS、MFA 与设备会话 | 组织授权 |
| 数据韧性 | WAL/PITR 自动化、恢复报告入 CI | M89/M90 |
| 规模性能 | 500+ 节点、虚拟滚动、聚合缓存、P95 | P2-B/M91 |
| 可视化梳理 | thesis/defense 演示稿、截图更新 | P0/P1 |
## 5. 非目标

通用 PaaS/DevOps/应用商店/完整 KubeSphere；任意 YAML 编辑/Pod exec/WebShell/Secret 值管理/跨集群 restore；动态 OPA/Rego、KubeEdge、GPU 调度、自定义 PromQL/LogQL、Grafana 导入。仅为“可交付、可验证、可持续”提升，不为功能数量突破边界。

## 6. 依赖与风险

P3 依赖组织授权与命名基础设施；kind E2E 时长增长需超时边界；覆盖率提升伴随适度重构但不改契约；P1 每月一个里程碑、P2 每月一到两个、P3 按授权推进。