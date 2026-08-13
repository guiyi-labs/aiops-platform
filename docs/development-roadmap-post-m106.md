# 后续开发路线：M107+（含前端界面优化并行轨）

- Status: Active
- Updated: 2026-08-13
- Baseline: M106（commit `d28c2b1`，tag `baseline-m106-20260813`，CI `31666531921` 15/15 全绿）
- 上位路线：[`docs/long-term-roadmap.md`](long-term-roadmap.md)（原则不变）
- 打磨合同：[`docs/polish-plan.md`](polish-plan.md)（P0–P3 优先级不变）
- 授权轨准备：[`docs/authorization-gate-prep.md`](authorization-gate-prep.md)
- 本文件取代 `docs/next-long-term-plan.md` 的 M93-B2–M102 执行序（M102–M106 已收口），
  规划 M106 之后的并行轨与主线里程碑。

## 0. 定位

项目功能基线已过 M106：事故工作空间（M98）、数据可见性（M99）、告警/巡检/信号三类
事故联动（M103–M105）、本地体验重构（M106）全部闭环。下一阶段不再单纯加页面，而是：

1. **接入并行前端优化轨**（另一 Agent 正在做），用统一验收基线吸收，避免与主线冲突。
2. **把 incident 从"创建入口齐全"推进到"可交接、可复盘、可关联"的协作闭环**。
3. **把 correlation（关联案例）接到 incident 作为第 6 来源**，完成多来源自动归并。
4. **工程卓越收口**：性能门禁从 report mode 转 fail-closed，incident 关键旅程入浏览器 E2E。
5. **刷新 RC**：M103–M110 全部纳入 v0.3.0-rc.6 离线包与供应链证据。
6. **M89/M90 生产授权轨保持 Deferred**，授权材料已备，一旦放行即按
   `docs/authorization-gate-prep.md` 执行并冲刺 GA Gate D。

## 1. 并行轨 A：前端界面优化（另一 Agent，独立工作流）

> 范围由前端 Agent 自主推进；本节只定义**衔接契约**，防止与主线互相踩踏。

### A.1 建议优化方向（按优先级，均为 M96 遗留的结构性技术债）

- **主题收敛**：`base.css` / `console-theme.css` / `motion.css` / `premium-ui.css` 四层
  覆盖顺序已多次出现特异性误匹配（M106 就修过 `section[class*="panel"]` 覆盖登录面板）；
  建议建立可审计的 CSS token 层，删除失效覆盖，规则以 M93-C 选择器数量为上限。
- **关键页面截图基线**：为 incident 列表/详情、告警、巡检、信号、工作负载、拓扑建立
  Desktop/Mobile 截图基线 + 像素容差（复用 M96 的截图基线机制）。
- **响应式审计**：35 个视图在 ≤720px 的可用性（表格横向滚动、工具栏折叠、抽屉全屏化）。
- **交互统一**：骨架屏（SkeletonCard）、空状态（EmptyState）、错误重试、toast 语义
  在全部视图落地一致；axe 双视口 0 critical/serious。
- **性能**：沿用 M93-B2 登录页预算与 M96 前端 DOM 硬上限，不破坏既有预算基线。

### A.2 衔接契约（必须满足）

- 门禁：`pnpm typecheck`、`pnpm lint`、`pnpm test`、`pnpm build` 全绿；
  Playwright 双视口回归（现有 42/42 或更新后的断言集）不回退；console error = 0。
- 只改 `frontend/`，不碰后端 API 契约；如确需新增字段/路由，先出 OpenAPI/typegen
  变更并在本文件登记，由主线 Agent 合入后端。
- 按 `docs/ARCHIVING.md` 归档：change-record + CHANGELOG + 基线 tag + 工作树干净。
- 与主线冲突规避：主线 M107 的 incident 详情 UI 以「证据时间线」组件为核心，
  前端 Agent 若也动 `IncidentsView`，先沟通再改；`console-theme.css` 的登录/侧栏
  规则（M106 刚收敛）优先保真。

## 2. 主线 M107：事故协作闭环（5–8 天）

**目标**：把 incident 从"能创建、能流转"升级为"10 秒可交接、可复盘"。

- **复盘视图（只读）**：事故解决后可查看证据、决策、动作、结果完整叙事；时间线可按
  人工/系统/来源过滤；不修改历史记录（M98 复盘视图的深化落地）。
- **SLA 仪表**：响应/解决目标、剩余时间、逾期高亮；SLA 提醒接入现有 notification
  webhook（M21+ 已具备）。
- **交接与关注者**：批量指派、关注者变化事件、交接审计落 `audit_logs`；备注与系统
  事件保持分离（不扩大敏感信息暴露面）。
- **统一证据时间线**：incident 详情页聚合五源证据（diagnosis / finding / inspection /
  alert / signal）到同一事件流，复用 M94 证据时间线与 M95 FindingDetail v2 组件。

**验收**：三条黄金场景（Node NotReady / Deployment unavailable / OOMKilled）各带至少
一个第二来源联动，可在 incident 详情首屏完成"根因→影响→证据→下一步"判断；状态机
与审计断言齐全；Playwright 关键旅程 Desktop/Mobile 双通过。

## 3. 主线 M108：关联归一与第 6 来源（5–7 天）

**目标**：correlation 引擎的关联案例（case_key 归并、factors 合并、confidence、
FirstObservedAt/LastObservedAt 已具备）正式接入 incident。

- `incidents.source_type` 增加 `correlation`（迁移 `000045`）；`SourceRefForCorrelation`。
- 自动归并策略：同一 case_key 的 incident 创建去重（防风暴）；相关 case 在 incident
  详情提示"关联案例"入口并聚合影响面。
- `CorrelationCasesView` ↔ `IncidentsView` 双向深链（沿用 M94 深链模式）。
- 演示演练：`demo-drill.sh` 增加「Correlation → incident」断言（2 信号触发 → 归并 →
  提升事故 → 重复提升去重）。

**验收**：关联案例提升事故 2/2 信号归并正确；风暴场景（同 case 高频触发）不产生
重复 incident；OpenAPI/typegen/迁移 up+down 齐全；CI 全绿。

## 4. 主线 M109：工程卓越收口（5–7 天）

- **性能门禁 fail-closed**：M96 Gate B 连续两个稳定周期后，超阈值从 warning 转
  fail-closed（需先在 CI 记录两个稳定样本）。
- **incident 关键旅程 E2E**：创建 → 指派 → 确认 → 解决 → 复盘，Desktop/Mobile、
  axe、console error 断言入 Playwright。
- **覆盖率**：全局 60% → 65%（重点补 incident / correlation / signal / metricshistory）。
- **fuzz 扩展**：correlation engine 与 incident transition 状态机 fuzz 用例。

**验收**：Gate B 两个稳定周期记录入库；旅程 E2E 全绿；覆盖率门禁上调至 65%。

## 5. 主线 M110：RC 刷新（v0.3.0-rc.6，3–5 天）

- 把 M103–M110 全部纳入不可变 RC：双架构镜像、Helm/Kustomize/离线包、SHA256SUMS、
  SBOM、provenance、keyless Cosign 验证。
- 全新环境安装 / 升级 / 回滚 / 备份恢复演练（复用 M101/M102 轨道脚本）；离线包自包含
  （迁移入包）。
- Release run 走完整质量门（对照 `v0.3.0-rc.4` Release run `31384939856` 的流程）。

**验收**：RC-6 资产 digest 固定、签名 fail-closed；全新环境双路径安装通过；升级失败可
回滚到上一 RC；M89/M90 未完成前保持 RC，不宣称 GA。

## 6. 授权轨（Deferred，随时可启动）

- **M89 身份轨**：真实 OIDC discovery/JWKS、issuer/audience/nonce/state 校验、组→角色
  映射、MFA 声明消费、Provider 不可用 fail-closed、断供 break-glass 与审计。
- **M90 数据轨**：WAL 归档/PITR、多副本 HA、故障注入（ENOSPC/网络分区/崩溃）、
  RPO/RTO 实测（只引用实测值）。
- 完成 M89/M90/M100/M101 + 两次独立全新环境演练 + 零未解释 critical gate → **GA Gate D**。

## 7. 编排与门禁总表

| 轨 | 优先级 | 开始条件 | 完成门禁 |
|---|---|---|---|
| A 前端优化（并行） | P0 | 现在 | typecheck/lint/test/build + 双视口回归 + axe + console error=0 + 归档 |
| B M107 事故协作闭环 | P0 | 现在（避开 A 的 IncidentsView 冲突面） | 黄金场景 + 复盘/SLA/证据时间线 + 旅程 E2E |
| C M108 关联归一 | P1 | M107 后 | correlation→incident 演练 + 风暴去重 + 迁移 |
| D M109 工程卓越 | P1 | M108 后 | 性能 fail-closed + 覆盖率 65% + fuzz |
| E M110 RC-6 刷新 | P2 | M109 后 | 全资产 digest + 全新环境演练 + 签名 fail-closed |
| F M89/M90 | P3 | 组织授权 | authorization-gate-prep.md 验收清单全过 → GA |

- 每个里程碑独立归档：change-record + CHANGELOG + `baseline-m1XX-YYYYMMDD` tag +
  远端 CI 证据（对照 M103–M106 的交付节奏）。
- 本地栈（`k8s-aiops`，`admin/admin123`）作为日常回归环境；新镜像构建遇 Docker Hub
  不可达时沿用 M106 的离线重建路径（宿主机交叉编译 / 复用既有 nginx 层）。
