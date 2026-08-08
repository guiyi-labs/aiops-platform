# 打磨计划：向业内最高水准靠拢（Polish Plan）

- Status: Draft
- Updated: 2026-08-08
- Baseline: `main` @ `4f43e98`（M1–M78 全部落地，工作树干净，与 `origin/main` 同步 0/0）
- Audience: 后续开发 Agent、评审者、发布负责人
- M33–M45 执行入口仍为 `docs/kubesphere-optimization-plan.md`；M46–M60 见
  `docs/post-m45-development-roadmap.md`。本计划是 **M78 之后的打磨路线执行合同**，
  不推翻既有架构决策、安全边界与非目标，只定义打磨优先级、验收标准与证据要求。

---

## 1. 目标定义：什么算"业内最高水准"

本项目不做功能数量对等（不与 KubeSphere / OpenCost / Trivy / Kubescape 逐项比
能力清单），而是把 **五条可度量基线** 做到接近同期业界的头部水准：

| 维度 | 业内头部参照 | 本项目当前 | 打磨目标 |
|---|---|---|---|
| 供应链与交付 | SLSA L1+、SBOM、keyless 签名可验证、语义化发布 | Cosign keyless 为结构占位、`attest-blob` fail-open、无正式 release | 真实可验证签名 + 语义化版本 + 离线安装包 |
| 安全与身份 | OIDC/MFA、最小权限、密钥不入库 | 本地账号完整，OIDC/MFA 仅离线就绪检查 | 组织批准 provider 接入 + 会话策略 |
| 数据可靠性 | WAL/PITR、HA、常态化恢复演练 | 逻辑备份恢复演练完整，无 WAL/PITR、无 HA | WAL 归档 + PITR + 多副本演练常态化 |
| AIOps 差异化 | 确定性 RCA + 引用式 AI + 质量回归 | M43/M52/M55/M56 已闭环，分析器呈孤岛 | 聚合态势 + 端到端闭环 + 黄金回归全覆盖 |
| 工程卓越 | 高覆盖率、fuzz、benchmark、浏览器 E2E | 全局覆盖率 50%、无 fuzz/benchmark/Playwright | 核心包 ≥70%、fuzz + benchmark + 浏览器 E2E 入 CI |

一句话：**把已闭环的产品链路打磨到"可发布、可验证、可规模"三层都成立**，而不是新增功能数量。

---

## 2. 现状基线（实证快照，2026-08-08 复验）

| 项 | 值 |
|---|---|
| Git | `main` @ `4f43e98`（08-03），76 commits，963 个跟踪文件，工作树干净，与 `origin/main` 0/0 |
| 里程碑 | M1–M78 全部落地；tag：`baseline-m25-20260730`、`baseline-m60-20260801` |
| 文档 | `docs/changes/` 114 份变更记录；`docs/adr/` 74 份 ADR；`CHANGELOG.md` 覆盖 M1–M78 |
| 后端 | Go 1.26.5 模块化单体，60 个 `internal/<domain>` 包；本地 `go build ./cmd/server` 通过 |
| 前端 | Vue 3 + TS + Vite，33 个视图，20 个 Vitest 测试文件；本机 pnpm/corepack 已可用 |
| 优化中心 | M61–M78 共 18 个纯只读分析器，统一 `Evaluate(clusterID, Inputs, time) → Status` 契约 |
| CI 门禁 | gofmt / vet / golangci-lint / 覆盖率≥50% / 5 个二进制 / race / ESLint / vue-tsc / Vitest / vite build / Kustomize / Compose / Helm lint / 4 类演练 |
| kind E2E | `scripts/e2e-*.ps1` 覆盖到 M60（M73/M75 扩展）；M67–M78 分析器尚无 real-cluster 证据 |

---

## 3. 差距分析（对标与证据）

### 3.1 交付与供应链
- 现状：版本化打包与 SHA256 清单已就绪；M59 加入 Cosign keyless 签名与 in-toto provenance，
  但为 **structural placeholder**，`cosign attest-blob` 为 `|| true` fail-open；无语义化 release。
- 差距：供应链"可验证"未闭环——无法对外声明产物可复现、签名可验、provenance 可信。
- 优先级：**P3**（需组织授权推送 tag / 创建 Release）。

### 3.2 安全与身份
- 现状：轻量集群/Namespace 授权（M35）、四角色、加密凭据、审计、CSV 安全导出、OIDC/MFA 离线就绪检查。
- 差距：生产级 OIDC/MFA、WAL/PITR、HA 明确 deferred（M26B，需组织授权）。
- 优先级：**P3**（外部身份与基础设施决策）。

### 3.3 AIOps 差异化深度
- 现状：确定性诊断（M18/M43）、AI 引用解释（M44/M55）、巡检计划（M52）、黄金回放（M56）
  均已本地闭环；优化中心 18 个分析器彼此独立、发现不聚合。
- 差距：**缺少"一眼看清集群有多健康"的聚合态势层**；优化中心与诊断/AI 链路未串联。
- 优先级：**P1（主线）**——本项目区别于通用云管的最大价值点。

### 3.4 规模与性能
- 现状：M72 拓扑采集并行化；无性能基准、无大集群实测。
- 差距：无 P95 API 延迟基准、无采集背压/分页实测、前端无虚拟滚动；大 fleet 下拓扑页会退化。
- 优先级：**P2**。

### 3.5 工程质量与文档
- 现状：CI 强、行为验证充分（40+ 包单测、kind E2E），但：覆盖率仅 50% 且为全局基线；
  无 Go benchmark/fuzz；无 Playwright 浏览器 E2E；README/PROJECT_STATUS 基线滞后
  （README 仍写 M60）；M1–M20 与 M61–M66 缺独立 change-record。
- 优先级：**P2**（先收口文档与工具链，再上强度）。

---

## 4. 打磨路线（P0 → P3，里程碑建议 M79–M90）

> 顺序原则：先 P0 收口 → P1 差异化 → P2 工程卓越 → P3 外部门禁。每个工作流独立完成并存证据，不批量合并。

### Phase 0：基线收口（P0，先做）

**W1 文档与基线对齐**
- 范围：README、PROJECT_STATUS、development-handoff 基线更新到当前 HEAD；补 M61–M66 独立 change-record（M1–M20 可选后续）。
- 验收：文档基线 == HEAD；`rg "M60" README.md` 不再有过时基线表述；每里程碑 change-record 独立成文。
- 门禁：文档-only CI 通过。

**W2 本地全量验证恢复**
- 范围：补齐 pnpm/corepack 工具链；`verify-fast.ps1 -Scope All` 与 `go test -race ./...` 本地全绿。
- 验收：后端+前端+清单三类本地全过；"仅靠 CI"降级为兜底。
- 门禁：上述命令 exit 0 为证据。

**W3 优化分析器 kind E2E（建议 M79）**
- 范围：新增 `scripts/e2e-optimization-kind.ps1`，在一次性 kind 集群中部署 fixtures，断言
  M61–M78 代表性分析器（finops/cis/deprecatedapi/netpolicy/imagepolicy/gitopsdrift/capacity
  /policy/hpaposture/pdbposture/ingressposture）在真实数据上产出预期 finding。
- 验收：每类至少 1 条 critical/warning 断言 + 1 条 clean 断言；脚本 finally 清理。
- 门禁：真实 kind 运行通过并产出脱敏 `summary.json`。

### Phase 1：差异化深度（P1，主线）

**W4 聚合治理态势视图（建议 M80）**
- 范围：后端新增聚合服务（复用 `internal/finding` 契约与 18 个分析器），输出"集群治理/优化总览"：
  按风险优先级排序的 findings 聚合 + 按集群/namespace 过滤 + 证据/修复建议；对标
  KubeSphere 集群态势与 Kubescape compliance report 形态。
- 验收：聚合 API 单测（排序/聚合/去重/空态）；前端新增总览视图页；真实 kind 数据点断言。
- 门禁：单测 + vitest + OpenAPI 契约测试 + kind E2E。

**W5 AIOps 闭环串联（建议 M81）**
- 范围：优化中心发现 → 自动生成巡检计划（M52）→ 确定性诊断（M43）→ AI 引用式解释（M55）
  → 受控操作 dry-run 预览（M19）串成一条可演示链路。
- 验收：端到端 kind 演示脚本 PASS；各环节保持只读/受控边界；审计追踪完整。
- 门禁：真实 kind 演示 + 审计断言。

**W6 黄金数据集覆盖分析器（建议 M82）**
- 范围：将聚合分析器与主要优化分析器的 findings 纳入 M56 golden replay 与质量报告；
  建立 discovery 契约快照（防 findings 意外漂移）。
- 验收：golden 回放含优化发现族且质量报告展示；变更需显式刷新快照。

**W7 拓扑与信号深化（建议 M83）**
- 范围：拓扑大图支持分页/折叠/聚合视图；Gateway API（Gateway/HTTPRoute）只读接入作为拓扑、SLO 与影响证据。
- 验收：500 节点 fixtures 渲染不崩溃；Gateway 只读无越权。
- 门禁：性能快照 + kind E2E。

### Phase 2：工程卓越（P2）

**W8 测试强度升级（建议 M84）**
- 范围：覆盖率门禁从"全局≥50%"提升为"核心包≥70% 且全局≥60%"；为 Quantity 解析、迁移解析、
  YAML/JSON 契约校验、OpenAPI schema 校验器补 10+ fuzz target；新增 benchmark 门禁
  （拓扑采集、metrics 序列查询、聚合分析器、registry health）。
- 进度：核心包门禁（metricshistory/apiquery/deprecatedapi/optimization ≥70%）已入 CI；
  9 个包新增 14 个 fuzz target、4 个 benchmark 并本地全绿（见
  `docs/changes/2026-08-09-m84-test-intensity.md`）。
  **剩余增量：全局覆盖率 ≥60%**（已达成：2026-08-09 实测 60.03%，CI 全局门禁同步提升到 60%，按包补测了
  automation/auth/cluster/workspace/alert/alertroute/authz/insight 等低覆盖层，共新增 8 个测试文件；见
  `docs/changes/2026-08-09-w8-coverage-closure.md`）。
- 验收：CI 新增 job 全绿；fuzz 记录随 change-record 留存；benchmark 基线写入文档。

**W9 前端质量与浏览器 E2E（建议 M85）** — 已落地（2026-08-09）：Playwright 双视口（Desktop 1280×720 / Mobile 390×844）14 项 smoke 全绿、console error=0；新增 unified motion 层（motion.css）、SkeletonCard、EmptyState。见 `docs/changes/2026-08-09-w9-playwright-e2e.md`。
- 范围：引入 Playwright（1280×720 + 390×844 两张视口），覆盖登录→工作台→详情→优化总览等关键链路，
  断言无 undefined 与 console 警告；补 a11y 基础（ARIA、键盘导航）；bundle 体积门禁。
- 验收：Playwright 关键链路 ≥5 条全绿；console error=0。
- 门禁：真实浏览器执行（本机或 CI headless）。

**W10 契约与 API 治理（M86）** — 已完成：OASdiff 破坏性变更检查在 CI 生效；全路由错误码静态审计并归一化 `VELERO_UNAVAILABLE` → 503；修复 OpenAPI 重复 schema（`VeleroBackupList`/`EvidenceRef`）与缺失 federation schemas/参数；生成 `frontend/src/api/openapi.d.ts`（`pnpm typegen` + CI sync gate）；`insight.ts` 消费生成契约类型。详见 `docs/changes/2026-08-09-w10-openapi-typesync.md`。
- 范围：全路由错误码审计（400/403/404/409/500 映射一致）；OpenAPI 破坏性检查扩展（schema 深度 diff）；
  前端类型改为从 `docs/api/openapi.yaml` 生成（或校验同步）。
- 验收：契约测试全覆盖；生成物与 openapi 同步自动校验。

### Phase 3：交付与生产就绪（P3，外部授权）

- **P3-1 正式发布闭环（建议 M87）**：语义化 tag + GitHub Release 产物 + Cosign 真实通过（移除
  `|| true`，接入 `slsa-github-generator`）。外部依赖：授权推送 tag / 创建 Release / 注册 runner。
- **P3-2 生产身份（建议 M88）**：OIDC provider 接入（issuer/JWKS 由组织提供）+ MFA + 会话策略
  （短期 token + refresh rotation）。
- **P3-3 数据可靠性（建议 M89）**：WAL 归档 + 定时 PITR 演练 + backend 多副本优雅停机 + 迁移回滚演练。
- **P3-4 性能容量（建议 M90）**：500 节点/50k pod 规模基准；前端虚拟滚动与聚合缓存。

---

## 5. 执行合同（门禁与不变量）

1. **决策变更先 ADR**：任何改变范围、权限、安全边界或既有契约的实现，必须先新增或更新 ADR 再开发。
2. **每个工作流独立成里程碑**：ADR → 实现 → 单测 → `verify-fast.ps1 -Scope All` → 涉及集群行为的
   一次性 kind E2E → change-record + CHANGELOG → CI 全绿，才可完毕；不得批量合并。
3. **证据规则**：`.artifacts` 机器生成、不跨设备；失败只能报失败，不得改证据转"通过"；
   文档基线必须与实际 HEAD 一致（M1–M78 期唯一倒退的教训）。
4. **契约同步**：任何路由/命令变更必须同步 OpenAPI、前端类型、错误码映射、审计 action/target。
5. **只读优先**：分析器与聚合必须只读；任何写能力引入必须走 ADR + threat-boundary 评审。
6. **持续指标**：每次打磨后，覆盖率趋势、fuzz 时长、benchmark、E2E 通过数作为 PR 可见记录。

---

## 6. 非目标（沿用既有边界）

- 通用 PaaS / DevOps / 应用商店 / 完整 KubeSphere 功能对等。
- 任意 YAML/CRD 编辑、Pod exec / WebShell、Secret 值管理、跨集群 restore / cutover。
- 动态 OPA/Rego 规则引擎（保持 M42 确定性诊断可回放）。
- 边缘 KubeEdge、GPU 调度、自定义 PromQL/LogQL 编辑、Grafana 导入。
- 本计划明确**不**为"功能数量"突破以上边界。

---

## 7. 风险与外部依赖

- **P3 工作流**（发布、身份、数据可靠性）依赖组织授权与命名基础设施。
- 工具链（pnpm/corepack、Playwright）需在受限网络内安装；建议先跑通 W2 再进 P2。
- kind E2E 时长增长，需透明清理与超时边界。
- 覆盖率提升可能伴随适度重构，属于职责范围；保留各分析器原输出契约。
- 时间窗口：P0 ~2 天；P1 每月 1 个里程碑；P2 每月 1–2 个；P3 按授权逐个推进。

---

## 8. 一句话总结

先收尾（文档对齐 + 本地全量验证 + 分析器真实 E2E），再打差异（聚合治理态势 + AI 闭环 +
黄金回归 + 拓扑规模），再上工程（覆盖率/fuzz/benchmark/浏览器 E2E/契约治理），
最后按授权推进正式发布、生产身份与数据可靠性 —— 每一步都有可验证的验收证据，不因"打磨"破坏既有安全边界。
