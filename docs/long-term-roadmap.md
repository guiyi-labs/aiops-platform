# 长远计划：把 AIOps 平台打磨到业内最高水准

- Status: Active（执行基线）
- Updated: 2026-08-09（深夜轮，对齐 `113e88b`）
- Baseline: `main` @ `113e88b`（领先 `origin/main` 13 个提交，本地未推送；仅余 `frontend/package.json` + `pnpm-lock.yaml` 两个未提交依赖变更）
- 关系：详细打磨契约见 [`docs/polish-plan.md`](polish-plan.md)；本文件是面向"长远方向 + 分阶段执行 + 已落地清单"的路由图。
- 原则：不推翻既有架构决策、安全边界与非目标；每一步都带可验证的证据与验收标准。

---

## 0. 当前已落地进度（2026-08-09 复验）

本地 `main` 领先 `origin/main` 13 个提交；`baseline-m85-20260809` / `baseline-w8w9-20260809` 指向当前 HEAD。

| 提交 | 内容 | tag |
|---|---|---|
| `591c19b` | polish: UI motion 打磨基线 + polish-plan 落库 | `baseline-polish-20260808` |
| `afa1e93` | M80 聚合治理态势视图（W4 posture） | — |
| `9077c4d` | count-up 指标滚动、aurora 登录背景、premium motion 层；新增 useCountUp + 4 条 Vitest | `baseline-ui-motion-20260808` |
| `48af578` | M84 测试强度（W8 主体）：14 个 fuzz、4 个 benchmark、核心包 ≥70% CI 门禁 | `baseline-m84-20260808` |
| `290e52e` | M81 AIOps 端到端闭环（W5）：findings → 巡检佐证（M52）→ 确定性诊断（M43）→ AI 引用（M55）→ dry-run 预览（M19） | `baseline-m81-20260809` |
| `5d01974` | M82 黄金回归发现契约（W6）：analyzer_discovery + posture/engine/diagnosis/inspection 快照，DatasetVersion → 1.1 | `baseline-m82-20260809` |
| `288d824` | M83 拓扑深化（W7）：Gateway API 只读接入 + collapse 折叠/聚合参数 + 4 条新测试 | `baseline-m83-20260809` |
| `a75d357` | W8/W9 闭环（M85）：全局覆盖率 60.03% + CI 门禁 60，Playwright 双视口 14/14 + console error=0，motion.css 统一动效 + SkeletonCard/EmptyState | `baseline-m85-20260809` / `baseline-w8w9-20260809` |
| `9fcb9a1` + `113e88b` | W10 错误码审计：静态审计全路由 writeError + `VELERO_UNAVAILABLE` 422/424 → 503（backup/restore/kubernetes 3 处）+ change-record | — |

验证状态（2026-08-09 全量复验）：
- 后端全绿：gofmt / vet / build / `go test ./...`；全局覆盖率 **60.03%**（13396/22317），CI 覆盖率门禁 50% → 60%。
- 前端全绿：typecheck / eslint / vitest（22 files · 124 tests）/ vite build / Playwright 14/14（Desktop + Mobile，console error=0）。
- **W10 已关闭**：重复 schema（`VeleroBackupList` / `EvidenceRef`）已修复，federation 缺失 schemas 与 `QueryPage`/`QueryPageSize` 参数已补齐；`pnpm typegen` 生成 `frontend/src/api/openapi.d.ts` 且幂等通过；CI 新增 `Contract typegen sync (W10)`；`insight.ts` 已消费生成类型。
- 环境提示：kind E2E（W3/W5/W7 集群行为）需 Docker/kind 运行时；本机 Docker daemon 未运行，真实集群验收证据待推送远端后补；M87–M90 需组织授权。

---

## 1. 五条可度量维度（对齐 polish-plan 五条基线）

| 维度 | 打磨目标 | 当前进度 |
|---|---|---|
| AIOps 差异化 | 聚合治理 + 端到端闭环 + 黄金回归全覆盖 | 主线（P1）：W4/W5/W6 已闭环，W7 拓扑代码完成（500 节点 fixture / 真实集群验收待补） |
| 工程卓越 | 核心包 ≥70% + 全局 ≥60% + fuzz/benchmark + 浏览器 E2E + 契约一致性 | W8/W9 已落地（实测 60.03%、14/14 E2E）；W10 错误码审计完成、类型生成待收尾 |
| 数据可靠性 | WAL 归档 + PITR + 多副本演练常态化 | P3，需组织授权 |
| 安全与身份 | 组织级 OIDC/MFA + 会话策略 | P3，需组织授权 |
| 供应链与交付 | 真实可验证签名 + 语义化发布 + 离线安装包 | P3，需组织授权 |

---

## 2. 分阶段长远路线（P0 → P3）

### Phase 0：收口与基线稳定（进行中，按天收敛）

- [x] 文档基线对齐当前里程碑：W8/W9 change-record 与 CHANGELOG（M80–M85）已补齐；README/roadmap 已同步。
- [x] W8 覆盖率增量：全局 59.1% → 60.03%，CI 门禁 50% → 60%；M61–M66/M74/M75 独立 change-record 补齐。
- [x] W10 收尾（2026-08-09 完成）：OpenAPI schema 修复（重复 key + 缺失 federation schemas/参数）→ 生成 `openapi.d.ts` → `pnpm typegen` + CI sync gate → `insight.ts` 消费生成类型 → change-record 与文档基线刷新。

### Phase 1：AIOps 差异化（主线里程碑，已完成待深化）

- [x] **W4/M80 聚合治理态势**：Namespace/配额/网络/策略/合规等 posture 聚合视图。
- [x] **W5/M81 端到端闭环**：findings → 巡检佐证 → 确定性诊断 → AI 引用解释 → dry-run 预览，一条链路可点击、可回放，只读不扩安全边界。
- [x] **W6/M82 黄金回归全覆盖**：聚合治理与主要分析器 findings 纳入黄金回放与质量报告（DatasetVersion 1.1，discovery 契约）。
- [x] **W7/M83 拓扑深化**：Gateway API（Gateway/HTTPRoute）只读接入 + collapse 折叠/聚合；**剩余**：500 节点大规模 fixture 渲染验证（需真实集群/大 fixture，Docker 可用后补）。

### Phase 2：工程卓越与前端体验（P2，每月 1–2 里程碑）

- [x] **W8/M84 测试强度**：核心包 ≥70% 门禁、fuzz、benchmark；全局覆盖率 ≥60%（实测 60.03%）。
- [x] **W9/M85 前端质量 + 浏览器 E2E**：Playwright 双视口 7 条关键链路全绿（14/14）、console error=0、统一 motion 层。
- [x] **W10/M86 契约与 API 治理**：错误码审计 + OpenAPI schema 修复 + 前端类型从 OpenAPI 生成 + `pnpm typegen` + CI sync gate + `insight.ts` 消费生成类型。
- [ ] **UX/动效持续打磨**（用户当前最关注方向）：
  - Dashboard/Posture/Topology 补齐节流、骨架屏、空态与过渡动效；统一 motion 语言与设计 token（基于 motion.css 已有基础）；
  - 主题切换（亮/暗/自定义）、a11y 深化（Playwright axe 断言）、bundle 体积门禁（vite build + CI 阈值）。

### Phase 3：交付与生产就绪（P3，需组织授权）

- [ ] **M87 正式发布闭环**：语义化 tag + GitHub Release + Cosign 真实通过（移除 `|| true`，接入 slsa-github-generator 或等价实现）。
- [ ] **M88 生产身份**：OIDC provider（issuer/JWKS 由组织提供）+ MFA + 会话策略（短期 token + refresh rotation）。
- [ ] **M89 数据可靠性**：WAL 归档 + 定时 PITR 演练 + 后端多副本优雅停机 + 迁移回滚演练。
- [ ] **M90 性能与容量**：500 节点 / 50k pod 规模基准；前端虚拟滚动与聚合缓存。

---

## 3. 建议的下一步顺序（W10 → UX → P3）

1. **W10 已完成**：OpenAPI schema 修复 + typegen + CI gate + change-record + 文档基线刷新（本轮提交后推送）。
2. **UX/动效持续打磨（可并行，用户重点方向）**：骨架屏/空态/过渡动效/主题切换/设计 token；随后追加 a11y 与 bundle 门禁。
3. **P3 授权项**：M87–M90 依赖 Docker/kind + 组织授权，逐个推进并保持证据与验收标准。

> 依赖提示：kind E2E（W3/W5/W7 集群行为）需要 Docker/kind 运行时；当前本机 Docker daemon 未运行，真实集群验收证据待推送远端并具备环境后补。

---

## 4. 方向探索（后续可选题，不承诺范围）

- **可观测性深化**：日志检索与指标联动、告警降噪与聚合、SLO 错误预算扩展到更多工作负载类型。
- **规模与性能**：500+ 节点拓扑渲染、pod 级虚拟滚动、聚合缓存与后台任务限流。
- **供应链可验证**：SBOM（CycloneDX）生成与入库、依赖合规扫描接入 CI、关键镜像软件成分展示。
- **身份升级**：OIDC/JWKS 对接、MFA 与设备会话管理（M88 前置）。
- **数据韧性**：开放 WAL/PITR 演练脚本、定期恢复演练报告纳入 CI 证据库（M89 前置）。

---

## 5. 非目标（沿用既有边界）

- 通用 PaaS / DevOps / 应用商店 / 完整 KubeSphere 对齐。
- 任意 YAML/CRD 编辑、Pod exec / WebShell、Secret 值管理、跨集群 restore / cutover。
- 动态 OPA/Rego 规则引擎、KubeEdge、GPU 调度、自定义 PromQL/LogQL 编辑、Grafana 导入。
- 本计划不为"功能数量"突破这些边界；只提升"可交付、可验证、可持续"三层层级。
