# 后续开发路线：M110 收口 + M111–M115（含并行前端轨收口与授权轨）

- Status: Active（执行基线）
- Updated: 2026-08-13
- Baseline: M109 封口 + M110 RC-6 发布预检完成（`docs/m110-release-preflight.md` 15 项全过），
  RC tag 推送待用户授权；本地 main 领先 origin 6 个提交（登录页 #login-ambience 第 9–13 轮）
- 上位路线：[`docs/long-term-roadmap.md`](long-term-roadmap.md)（原则不变）
- 打磨合同：[`docs/polish-plan.md`](polish-plan.md)（P0–P3 优先级不变）
- 授权轨准备：[`docs/authorization-gate-prep.md`](authorization-gate-prep.md)
- 已知限制：[`docs/testing/known-limitations.md`](testing/known-limitations.md)
- 本文件取代 `docs/development-roadmap-post-m106.md` 的 M107–M110 执行序（M107–M109 已封口、
  M110 待发布），规划 M110 之后的并行轨与主线里程碑。

## 0. 定位

项目功能基线已过 M109，M110（v0.3.0-rc.6 刷新）本地预检全部通过，剩余动作是**用户授权的
发布动作**而非开发工作。事故协作闭环（M98 工作空间 + M103–M105 三类联动 + M107 协作 +
M108 关联归一 + M109 收口）已经可用。下一阶段不再堆功能数量，而是三条线并行：

1. **M110 收口**：把已就绪的发布与前端轨成果推出去（用户决策项），完成 rc.6 全链路证据。
2. **产品线（M111–M114）**：把 incident 从"闭环可用"推进到"可运营、可度量、可解释"，
   并把已有能力（runbook 目录、AI 引用、受控动作、SLO/信号）织成更深的闭环。
3. **工程线（M115 + 常续）**：覆盖率 65% → 70%、性能基准入 CI fail-closed、fuzz 扩展；
   M89/M90 授权轨保持 Deferred，材料已备，放行即执行并冲刺 GA Gate D。

约束沿用：规则诊断是确定性主链路、AI 仅解释增强、受控操作目录固定（不新增任意命令）、
未授权资源返回 404、所有写路径 dry-run + 确认 + 幂等 + 审计。未完成 M89/M90 前保持 RC，
不宣称 GA。

## 1. Track 0：M110 收口（用户决策项，1–2 天）

> 本轨唯一阻塞点是用户授权 push RC tag 触发远端 Release；其余均为本地可执行项。

- [ ] **推送本地提交**：main 领先 origin 6 个提交（登录页 #login-ambience 第 9–13 轮），
  先复验前端门禁（`pnpm typecheck` / `pnpm lint` / `pnpm test` / `pnpm build` + Playwright
  双视口回归）后 `git push origin main`。
- [ ] **用户授权后发布**：`git tag v0.3.0-rc.6 && git push origin v0.3.0-rc.6`，触发
  `.github/workflows/release.yml`（validate → quality 复用 ci.yml → package：双架构 OCI、
  SBOM、Helm/Kustomize、离线包、keyless Cosign、GitHub prerelease）。发布前按
  `docs/m110-release-preflight.md` 清单核对。
- [ ] **发布后演练**：`scripts/offline-install-drill.sh`（`APP_VERSION=v0.3.0-rc.6`）全新
  环境安装 10/10；`scripts/dual-env-compose-drill.sh` 跨 digest 升级/回滚/备份恢复；
  `scripts/release-verify.ps1 -Version v0.3.0-rc.6` 核对资产 digest 与签名。
- [ ] **封口**：RC-6 资产 digest 固定、签名 fail-closed 确认后打
  `baseline-m110-rc6-YYYYMMDD`，更新 `docs/PROJECT_STATUS.md`。

**门禁**：Release run 全绿；演练报告入 `.artifacts/`；RC 资产可校验；M89/M90 未完成前
版本保持 RC。

## 2. Track A：前端优化轨收口（并行 Agent 工作流，持续）

> 范围由前端 Agent 自主推进；本节只定义**衔接契约**与收口目标，防止与主线互相踩踏。

- **主题收敛**：`base.css` / `console-theme.css` / `motion.css` / `premium-ui.css` 四层
  建立可审计的 CSS token 层，删除失效覆盖（M106 修过 `section[class*="panel"]` 覆盖登录
  面板；M93-C 选择器数量为上限）。
  ✅ 两轮均落地：`scripts/audit-css-tokens.mjs`（有效值解析 + MATCHED/ORPHAN 分类 +
  `--apply`/`--check` 门禁）+ 第一轮 112 处精确值迁移 + 第二轮 ~240 处 orphan
  旧调色板字面量语义收敛（`#5a6672`→`var(--text-muted)` 等），四层 `replaceable=0`，
  orphan 仅剩刻意设计值（登录氛围/阴影/通知）；基线整体重建后 `--verify` 全绿。见
  `docs/changes/2026-08-14-css-token-layer.md`、`docs/changes/2026-08-14-css-token-round2-migration.md`。
- **关键页面截图基线**：全平台前台视图
  Desktop/Mobile 截图基线 + 像素容差（复用 M96 截图基线机制；登录页 15 轮成果一并纳入）。
  ✅ 已全量落地：`scripts/capture-ui-baselines.mjs`（capture/`--verify`）+ 
  `docs/ui-baselines/`（Desktop 1440×900 / Mobile 375×812，像素 diff+掩码+确定性随机），
  基线 **62 条 / 31 视图**（登录页 + 控制台 + 6 高价值视图 + 第三批 20 视图），`--verify`
  全绿；audit-logs 因实时追加内容仅纳入 axe 不纳入像素基线。首批见
  `docs/changes/2026-08-13-ui-baseline-screenshots.md` / `2026-08-13-ui-baseline-console-pages.md`，
  第三批见 `docs/changes/2026-08-14-ui-baseline-batch3.md`。
- **响应式审计**：31 个视图在 ≤720px 的可用性（表格横向滚动、工具栏折叠、抽屉全屏化）。
  ✅ 全量落地：31/31 视图 375px 审计 `overflowX=false`、无可点击元素出屏，见
  `docs/changes/2026-08-14-a11y-fixes-batch3.md`。
- **交互统一**：SkeletonCard / EmptyState / 错误重试 / toast 语义在全部视图落地一致；
  axe 双视口 0 critical/serious，console error = 0。
  ✅ 门禁脚本已落地并全量收口：`scripts/audit-a11y-axe.mjs`（CDP+axe-core，双视口，
  0 critical/serious/0 app errors），覆盖 **32 视图**（含 audit-logs），
  62 条截图基线 `--verify` 全绿；并修复 Workloads Tab / 用户页 / audit-logs 对比度、
  select 无障碍、缺 cluster_id 400 缺陷。
  ✅ **CI 衔接**：`scripts/ui-gate.mjs` + `pnpm ui:gate` 一键串联四件套门禁
  （CSS tokens → baselines → axe → bundle），`PASS: 4/4`。
  见 `docs/changes/2026-08-14-a11y-axe-audit.md`、
  `2026-08-14-ui-baseline-batch3.md`、`2026-08-14-a11y-fixes-batch3.md`、
  `2026-08-14-ui-gate-ci-integration.md`。
- **性能**：沿用 M93-B2 登录页预算与 M96 前端 DOM 硬上限，不破坏既有预算基线。

**衔接契约（必须满足）**：门禁 `pnpm typecheck` / `pnpm lint` / `pnpm test` / `pnpm build`
全绿；Playwright 双视口回归不回退；只改 `frontend/`，不碰后端 API 契约；确需新增字段/路由
时先出 OpenAPI/typegen 变更并登记，由主线合入后端；按 `docs/ARCHIVING.md` 归档
（change-record + CHANGELOG + 基线 tag + 工作树干净）。

## 3. 主线 Track B：M111 事故响应深化（5–8 天）

**目标**：incident 从"能协作"升级为"可运营、可度量、可交接"，全部复用既有领域组件。

**执行进度（2026-08-14）**：M111 已完成 KPI 基础层、事故详情 Runbook 关联、模板与严重级矩阵、SLA 升级链和复盘导出：新增真实时间戳派生的
`GET /api/v1/incidents/metrics`，以及复用 M81 Insight 的
`GET /api/v1/incidents/{incident_id}/runbook`；新增 `GET /api/v1/incidents/templates` 和
`GET /api/v1/incidents/{incident_id}/postmortem/export`。全部契约已同步 OpenAPI/typegen，待全量门禁和阶段 tag 封口。

- **Runbook 关联（已完成）**：incident 详情页挂接 M81 Insight 的诊断/巡检/AI 解释/dry-run
  候选，只读展示且对人工来源、源记录缺失和跨域不确定性 fail-closed；不新增任意操作，写路径
  仍走既有受控动作目录。M44 automation 与 AI investigator 的具体执行状态延后到后续协调查询阶段。
- **MTTA/MTTR 事故 KPI**：由时间线时间戳派生（created → first_assigned → first_ack →
  resolved），新增事故 KPI 视图与聚合大盘；OpenAPI + typegen + 迁移。
- **事故模板与严重级矩阵（已完成）**：创建事故支持版本化模板，severity → SLA 目标通过 `INCIDENT_SLA_TARGETS` 可配置并落到事故快照。
- **SLA 升级链**：超时未响应/未解决 → 经现有 notification webhook 逐级升级（M107 SLA
  提醒的深化），升级事件落审计。
- **复盘导出（已完成）**：postmortem Markdown 导出包含证据时间线、决策、动作、结果叙事，是 M107
  复盘视图的可携带版本。

**验收**：三条黄金场景（Node NotReady / Deployment unavailable / OOMKilled）带第二来源
联动仍成立；runbook 关联只读且引用校验通过；KPI 由真实时间戳派生（无伪造）；升级链
通知 + 审计齐全；Playwright 关键旅程 Desktop/Mobile 双通过。

## 4. 主线 Track C：M112 AI 协调查询与解释深化（5–7 天）

**目标**：把 AI 从"单次调查请求"升级为"事故上下文中的可追问解释"，严守引用纪律。

- **会话式调查**：incident 上下文中连续提问，输出经 M44 同款 provider/citation/runbook
  校验（引用缺失/不一致 fail-closed；`AI_ENABLED=false` 时降级为确定性摘要）。
- **AI 事故摘要**：确定性阶段门 + 引用校验的自动摘要（根因/影响/证据/下一步），不伪造、
  无来源不生成结论。
- **解释覆盖率大盘**：基于 `aiexplain` quality feedback 基线，展示解释可用率/引用率/
  降级率（只读，纯展示）。

**验收**：引用校验 0 泄漏（不生成无来源结论）；Provider 故障/关闭时确定性降级路径可用；
黄金 fixture 回放一致；门禁全绿。

## 5. 主线 Track D：M113 优化中心闭环与巡检深化（5–7 天）

**目标**：把只读优化中心与既有受控动作目录织成"发现 → 预览 → 人工执行"闭环。

- **finding → runbook 预览导航**：posture/optimization finding 一键跳转对应
  `insight` runbook（dry-run 预览 + 可执行 runbook 入口），全程只读导航，不新增写路径。
- **巡检趋势与覆盖率度量**：plan → findings 时间序列、规则命中覆盖率、计划调度改进
  （M52 巡检深化）；数据可见性沿用 M99-D 的显式覆盖度展示约定。
- **（可选，需契约评审）** 受控操作目录扩展提案：如 PDB/HPA 创建建议 → dry-run 预览 →
  人工确认执行；涉及 `remediation` 目录与 ADR 变更，需单独评审，不默认纳入。

**验收**：预览与实际 dry-run 结果一致；无任何绕过审计/确认的新写路径；巡检度量有真实
数据且 fail-closed 无样本不视为健康。

## 6. 主线 Track E：M114 可观测性深化（5–7 天）

- **SLO burn 扩展与告警降噪**：更多 SLO 来源信号化（M99-A 管道扩展）、关联驱动的告警
  去重/聚合展示（复用 correlation 引擎）。
- **指标历史下采样**：metricshistory 现有 7 天精确序列，扩展 30 天下采样归档
  （有界查询 + 前端渲染预算内，沿用 M96 预算机制）。
- **事件流/日志探索增强**：时间有界、筛选、深链（M50/M51 基础上的收口）。

**验收**：所有查询有界（时间窗/条数上限）；前端 50k Pod DOM 预算不回退；无新增全量
写入平台数据库的路径（资源仍实时查 API Server）。

## 7. 主线 Track F：M115 工程卓越冲刺（5–7 天 + 常续）

- **覆盖率 65% → 70%**：重点补 incident / correlation / signal / metricshistory /
  httpserver 核心分支（对照 `long-term-roadmap.md` P2-A，CI 报告逐包推进）。
- **性能基准入 CI**：P95 关键 API 延迟（report → 两稳定周期后 fail-closed，复用 M96
  Gate B 模式开关）；100k 事件 metricshistory 窗口与 500 节点/50k Pod fixture 渲染基准
  （对照 `long-term-roadmap.md` P2-B）。
- **fuzz 扩展**：更多状态机（automation plan/rollback、SLA monitor）纳入 CI fuzz seed；
  race 门禁保持（`go test -race`）。

**验收**：覆盖率 70% 门禁上调；性能基准两个稳定周期记录入 `docs/m96-gate-b-baselines.md`
后 fail-closed；fuzz 全绿且不 panic。

## 8. Track G：授权轨（Deferred，随时可启动）

- **M89 身份轨**：真实 OIDC discovery/JWKS、issuer/audience/nonce/state 校验、组→角色
  映射、MFA 声明消费、Provider 不可用 fail-closed、断供 break-glass 与审计。
- **M90 数据轨**：WAL 归档/PITR、多副本 HA、故障注入（ENOSPC/网络分区/崩溃）、以实测
  RPO/RTO 验收。
- 完成 M89/M90 + M100/M101 本地轨 + 两次独立全新环境演练 + 零未解释 critical gate →
  **GA Gate D**（执行顺序与验收清单见 `docs/authorization-gate-prep.md`）。

## 9. 编排与门禁总表

| 轨 | 优先级 | 开始条件 | 完成门禁 |
|---|---|---|---|
| 0 M110 收口 | P0 | 现在（push rc.6 需用户授权） | Release 全绿 + 全新环境/升级回滚/备份恢复演练 + digest 签名可校验 + `baseline-m110-rc6` |
| A 前端优化（并行） | P0 | 现在 | typecheck/lint/test/build + 双视口回归 + axe + console error=0 + 归档 |
| B M111 事故响应深化 | P0 | Track 0 后（避开 A 的 IncidentsView 冲突面） | runbook 关联 + KPI + 升级链 + 复盘导出 + 旅程 E2E |
| C M112 AI 协调查询 | P1 | M111 后 | 引用校验 0 泄漏 + 确定性降级 + 黄金 fixture 一致 |
| D M113 优化中心闭环 | P1 | M112 后 | finding→预览闭环 + 巡检度量 + 无新写路径 |
| E M114 可观测性深化 | P1 | M113 后 | 有界查询 + 渲染预算不回退 |
| F M115 工程卓越 | P1 | 与 B–E 并行可推进 | 覆盖率 70% + 性能基准 fail-closed + fuzz/race 全绿 |
| G M89/M90 授权轨 | P3 | 组织授权 | authorization-gate-prep.md 验收清单全过 → GA |

- 每个里程碑独立归档：change-record + CHANGELOG + `baseline-m1XX-YYYYMMDD` tag +
  远端 CI 证据（对照 M103–M109 的交付节奏）。
- 本地栈（`k8s-aiops`，`admin/admin123`）作为日常回归环境；新镜像构建遇 Docker Hub
  不可达时沿用 M106/M109 的离线重建路径（宿主机交叉编译 / 复用既有 nginx 层）。
- 发布/授权类动作（push rc tag、触发远端 Release、M89/M90 接入）均为用户决策项，
  本路线只提供清单与顺序，不自动执行。

## 10. 非目标（沿用既有边界）

- 通用 PaaS / DevOps / 应用商店 / 完整 KubeSphere 对齐；任意 YAML/CRD 编辑、Pod exec /
  WebShell、Secret 值管理、跨集群 restore / cutover。
- 动态 OPA/Rego 规则引擎、KubeEdge、GPU 调度、自定义 PromQL/LogQL 编辑、Grafana 导入。
- AI 不直接执行集群变更；受控操作目录不因本路线扩大执行面（新增写路径必须走
  remediation 目录 + ADR 评审）。
- 集群实时资源通过 API Server 查询，不全量写入平台数据库。
- 本路线不为"功能数量"突破这些边界；只提升"可运营、可度量、可解释、可持续"四层。
