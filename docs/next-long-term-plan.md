# 下一阶段开发计划：M93–M97

- Status: Active（执行入口）
- Updated: 2026-08-09
- Baseline: 本地功能基线 `main` @ `b0287e1`（M92，tag `baseline-m92-20260809`）；`origin/main` @ `181da6f`，本地归档与基线提交待统一推送
- 上位路线：[`long-term-roadmap.md`](long-term-roadmap.md)
- 归档规范：[`ARCHIVING.md`](ARCHIVING.md)

## 0. 当前判断

项目已经越过“功能是否齐全”的阶段：确定性诊断、AI 引用解释、多集群联邦、优化中心、
受控运维、真实 kind E2E、覆盖率/契约/a11y/bundle 门禁均已形成基线。M92 又补齐了登录
首屏的产品识别度。下一阶段不应继续横向堆页面，而应按以下顺序提升：

1. 先把新视觉做成可信、可测、低成本运行的正式能力。
2. 再把 AIOps 核心价值收敛成“10 秒看清根因”的产品叙事。
3. 最后用规模、性能和供应链证据证明它可以稳定交付。

## 1. M92 基线快照

| 维度 | 当前状态 | 结论 |
|---|---|---|
| 产品能力 | M1–M92 + W10–W12 | 主链路闭环，进入深度与证据阶段 |
| 前端 | 34 个视图；M91 虚拟滚动；M92 粒子网络/拓扑登录页 | 视觉完成度明显提升，需补数据真实性与专项回归 |
| 后端 | 61 个 `internal/` 模块；18 个只读分析器 | 领域能力充足，不再以新增模块数量为目标 |
| 质量 | 全局覆盖率 60.03%；核心包 ≥70%；Playwright/axe/bundle/typegen 门禁 | 基线可靠，下一目标是性能预算与关键旅程覆盖 |
| 交付 | kind E2E、SHA256SUMS、签名 fail-closed | 正式 Release / 真实 keyless / 离线包仍需收口 |
| 外部依赖 | M89 OIDC/MFA、M90 WAL/PITR/HA | 继续按组织授权推进，不阻塞本地主线 |

## 2. 执行顺序

### M93：登录页质量收口（1–2 天，立即开始）

目标：把 M92 从“视觉完成”提升为“可长期维护、数据可信、低端设备可控”。

范围：

- 明确 12 / 186 / 99 的语义：优先接入最小公开摘要接口；若安全边界不允许未认证读取，
  改为明确的演示/能力基线，不使用“实时”措辞。
- ParticleNetwork 使用 `ResizeObserver` 处理容器尺寸变化；页面隐藏时暂停 RAF，恢复时继续。
- 监听 reduced-motion 变化，用户运行时切换系统设置也能即时降级。
- 增加组件测试与 Playwright：Canvas 非空像素、桌面/移动布局、reduced-motion 静态帧、
  登录表单可用、console error=0。
- 加性能预算：桌面 60fps 目标、低端移动设备降粒子密度、登录页新增 JS/CSS 体积阈值。

验收：

- 不存在无法解释的硬编码“实时”指标。
- Desktop 1440×900、Mobile 390×844、reduced-motion 三组 Playwright 全绿。
- Canvas 像素抽样非空，页面 hidden 后 RAF 停止，resize 后粒子数量重新计算。
- `pnpm lint/typecheck/test/build/bundle:gate/test:e2e` 全绿并上传截图证据。

### M94：诊断叙事与证据时间线（4–6 天）

目标：让 SRE 在一次诊断详情里 10 秒内回答“哪里坏了、证据是什么、下一步做什么”。

范围：

- 根因卡片：主根因、影响面、置信来源、首次出现时间、当前状态。
- 证据时间线：资源状态、事件、日志、变更、告警按统一时间轴排列。
- 回放模式：按事件时间逐步重放 M81 insight runbook，不伪造 AI 结论。
- 受控动作入口：只暴露已有 dry-run/确认/幂等/审计能力，不新增任意执行面。

验收：

- 黄金数据集至少覆盖 Node NotReady、Deployment unavailable、OOMKilled 三类场景。
- 规则结论可追溯到原始证据；AI 文本必须带引用，引用失效时显式降级。
- 三条场景均有 Playwright 旅程、API 契约测试和 change-record。

### M95：洞察可解释分层（4–6 天）

目标：把 18 个分析器从列表结果统一为“规则 → 证据 → 建议”的三层模型。

范围：

- 统一 finding detail schema，避免各分析器独立拼装字段。
- posture / optimization / diagnosis / inspection 使用同一证据抽屉与严重度语义。
- 建议必须区分“只读建议”和“可执行受控动作”，默认不自动执行。
- 增加跨分析器聚合、去重与同一资源的相关 finding 分组。

验收：

- 18 个分析器全部通过 schema parity 测试。
- 任一 finding 可在三次交互内定位到原始对象与规则来源。
- 黄金回放 DatasetVersion 升级并保留向后兼容说明。

### M96：规模与性能证据（5–7 天）

目标：用可重复数据证明大规模 fleet 下 API、采集器和前端都不会退化失控。

范围：

- 500 节点 / 50k Pod / 100k 事件 fixture；覆盖拓扑、工作负载、全局搜索与历史窗口。
- 后端记录 P50/P95/P99、内存峰值、goroutine 数与超时/背压行为。
- 前端记录首屏、交互响应、长任务、内存和虚拟列表节点数。
- 性能基准入 CI：先 warning，稳定两周后再 fail-closed。

验收：

- 关键 API P95 与内存阈值写入版本化基线；超阈值产生可读报告。
- 50k Pod 场景 DOM 节点数量有上限，滚动无明显跳变。
- 结果以 artifact + change-record 归档，不只保留聊天或终端输出。

### M97：正式发布与供应链收口（3–5 天 + 授权等待）

目标：产出可验证、可离线安装、可回滚的正式版本。

范围：

- GitHub Release、语义化版本、SHA256SUMS、SBOM、provenance 与 cosign keyless verify。
- 离线镜像包、Helm/Kustomize 双路径安装、升级/回滚演练。
- 发布证据集中到单一 manifest，用户可用一条命令完成校验。

验收：

- 在全新环境完成离线安装、健康检查、升级和回滚。
- 签名/校验任一步失败时发布流程 fail-closed。
- M89/M90 未获授权部分明确标注，不把就绪检查写成生产能力完成。

## 3. 第一迭代任务板（M93）

| 顺序 | 任务 | 产物 | 门禁 |
|---|---|---|---|
| 1 | 决定展示数字的数据契约 | ADR/契约说明 | 未认证信息泄露审查 |
| 2 | ParticleNetwork 生命周期增强 | 组件 + 单测 | reduced-motion / hidden / resize |
| 3 | 登录页 Playwright 专项 | 双视口截图与像素断言 | console error=0 |
| 4 | 低端设备性能预算 | benchmark 报告 | 粒子密度与 bundle 阈值 |
| 5 | 归档与基线 | change-record / CHANGELOG / tag | 工作树干净、CI 全绿 |

## 4. 每个里程碑的 Definition of Done

- 行为、API、安全边界和非目标写清楚；涉及架构变化时先 ADR。
- 代码、测试、OpenAPI/typegen、前端契约同步完成。
- 最小门禁通过；集群行为补 kind E2E，用户旅程补 Playwright。
- 性能或视觉声明有可复现 artifact，不只写“已验证”。
- `docs/changes/YYYY-MM-DD-<slug>.md`、CHANGELOG、PROJECT_STATUS 同步。
- 提交、基线 tag 与远端状态一致；未获授权项明确 deferred。

## 5. 非目标与边界

- 不扩展为通用 PaaS、应用商店或完整 KubeSphere 替代品。
- 不增加任意 YAML 编辑、Pod exec/WebShell、Secret 值管理或跨集群 restore/cutover。
- 不用动态 OPA/Rego、GPU 调度、自定义 PromQL/LogQL 作为本阶段目标。
- 不用更多装饰动画代替产品价值；M93 后前端主线转向诊断叙事与证据效率。
