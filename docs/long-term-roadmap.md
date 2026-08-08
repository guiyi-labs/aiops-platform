# 长远计划：把 AIOps 平台打磨到业内最高水准

- Status: Active（执行基线）
- Updated: 2026-08-09
- Baseline: `main` @ `30cb507`（领先 `origin/main` 8 个提交；另有未提交的 W8 覆盖率补测与本次同步，待明日统一提交远端）
- 关系：详细打磨合同见 [`docs/polish-plan.md`](polish-plan.md)；本文件是面向"长远方向 + 分阶段执行 + 已落地清单"的路由图。
- 原则：不推翻既有架构决策、安全边界与非目标；每一步带可验证的证据与验收标准。

---

## 0. 当前已落地进度（2026-08-09）

本地 `main` 领先 `origin/main` 8 个提交（另有未提交的 W8 覆盖率补测）；已打 tags：

| 提交 | 内容 | tag |
|---|---|---|
| `591c19b` | UI motion 打磨基线 + polish-plan 落库 | `baseline-polish-20260808` |
| `afa1e93` | M80 聚合治理态势视图（W4 posture） | — |
| `9077c4d` | count-up 指标滚动、aurora 登录背景、premium motion 层；新增 useCountUp + 4 条 Vitest | `baseline-ui-motion-20260808` |
| `48af578` | M84 测试强度：14 个 fuzz、4 个 benchmark、核心包 ≥70% CI 门禁（metricshistory 79.4% / apiquery 100% / deprecatedapi 93.2% / optimization 75.9%） | `baseline-m84-20260808` |
| `290e52e` | M81 AIOps 端到端闭环（W5）：findings → 巡检佐证（M52）→ 确定性诊断（M43）→ AI 引用（M55）→ dry-run 预览（M19），新增只读 insight runbook 端点 | `baseline-m81-20260809` |
| `5d01974` | M82 黄金回归发现契约（W6）：analyzer_discovery 场景 + posture/engine/diagnosis/inspection 快照进黄金回放，DatasetVersion → 1.1 | `baseline-m82-20260809` |
| `288d824` | M83 拓扑深化（W7）：Gateway API 只读白名单 + collapse 折叠/聚合参数 + 4 条新测试 + ADR 0080 | `baseline-m83-20260809` |
| `30cb507` | M82 发现契约 change-record + 长远路线图基线同步 | — |

验证状态：
- 前端全绿：typecheck / eslint / vitest（22 files · 124 tests）/ vite build。
- 后端全绿：gofmt / vet / build / `go test ./...`；W7 相关包（httpserver/kubernetes/topology）gofmt + vet + test 单独复验通过。
- 全局覆盖率 59.1%（< 60% 目标，显式跟踪项，见 W8）；W8 覆盖率补测（correlation/diagnosis/inspection/automation/kubernetes/optimization/golden）已就绪并通过，待提交。
- 依赖提示：kind E2E 涉及 W3/W5/W7 集群行为，需要 Docker/kind 运行时；当前本机 Docker daemon 未运行，真实集群验收证据待推送远端后补。

---

## 1. 五条可度量维度（对齐 polish-plan 五条基线）

| 维度 | 打磨目标 | 当前进度 |
|---|---|---|
| AIOps 差异化 | 聚合治理 + 端到端闭环 + 黄金回归全覆盖 | 主线（P1），M80/M81/M82/M83 已落地，主线收敛为后续 UX/规模迭代 |
| 工程卓越 | 核心包 ≥70% + 全局 ≥60% + fuzz/benchmark/浏览器 E2E | M84 已落地大半，W8 增量待办 |
| 数据可靠性 | WAL 归档 + PITR + 多副本演练常态化 | P3，需组织授权 |
| 安全与身份 | 组织级 OIDC/MFA + 会话策略 | P3，需组织授权 |
| 供应链与交付 | 真实可验证签名 + 语义化发布 + 离线安装包 | P3，需组织授权 |

---

## 2. 分阶段长远路线（P0 → P3）

### Phase 0：收口与基线稳定（已在推进，2 天内）

- 文档基线对齐实际 HEAD：README/PROJECT_STATUS 更新到当前提交后的真实状态；本文件已同步 W7。
- 补齐 M1–M20 与 M61–M66 缺失的独立 change-record（含本次 M82 的 change-record 待补）。
- 全局覆盖率从 59.1% 渐进拉向 ≥60%（重点：cmd、repository、store、migrations 等低覆盖层）。

### Phase 1：AIOps 差异化（主线，按月里程碑推进）

- [x] **W5/M81 AIOps 端到端闭环**：优化中心 findings → 巡检计划（M52）→ 确定性诊断（M43）→ AI 引用解释（M55）→ dry-run 预览（M19），一条链路可点击、可回放，只读不扩安全边界。
- [x] **W6/M82 黄金回归全覆盖**：把聚合治理与主要分析器 findings 纳入 M56 黄金回放 + 质量报告（DatasetVersion 1.1，discovery 契约快照）。
- [ ] **W7/M83 拓扑深化**：Gateway API（Gateway/HTTPRoute）只读接入 + collapse 折叠/聚合参数；500 节点 fixture 渲染验证待补（需真实集群环境）。

### Phase 2：工程卓越（P2，每月 1–2 里程碑）

- [x] **W8/M84 测试强度**：核心包 ≥70% 门禁 + fuzz + benchmark 已落地；**剩余增量**：全局覆盖率 ≥60%（显式跟踪项）、CI 门禁 50%→60%。
- [ ] **W9/M85 前端质量 + 浏览器 E2E**：Playwright（1280×720 + 390×844），关键链路 ≥5 条、console error=0、补 a11y 基础、bundle 体积门禁。
- [ ] **W10/M86 契约与 API 治理**：全路由错误码审计（400/403/404/409/500 映射一致）、OpenAPI 深度 diff、前端类型从 `docs/api/openapi.yaml` 生成+同步校验。
- [ ] **UX/动效持续打磨**（用户重点方向）：Dashboard/Posture/Topology 补充节流、骨架屏、空态与过渡动效、主题切换，形成统一 motion 语言与设计 token。

### Phase 3：交付与生产就绪（P3，需组织授权）

- [ ] **M87 正式发布闭环**：语义化 tag + GitHub Release + Cosign 真实通过（移除 `|| true`，接入 slsa-github-generator）。
- [ ] **M88 生产身份**：OIDC provider（issuer/JWKS 组织提供）+ MFA + 会话策略（短期 token + refresh rotation）。
- [ ] **M89 数据可靠性**：WAL 归档 + 定时 PITR 演练 + 后端多副本优雅停机 + 迁移回滚演练。
- [ ] **M90 性能容量**：500 节点 / 50k pod 规模基准；前端虚拟滚动与聚合缓存。

---

## 3. 建议的下一步（短期收敛，建议按序执行）

1. **全局覆盖率 ≥60%**——按包渐进补测，CI 门禁从 50% 提到 60%，补齐 W8 剩余增量。
2. **文档基线对齐（README/PROJECT_STATUS + 缺失 change-record）**——稳定 P0 收口。
3. **W9/M85 Playwright + UX/动效打磨**——前端真实浏览器证据 + 用户最看重的 UI 细节。


> 依赖提示：kind E2E（涉及 W3/W5/W7 集群行为）需要 Docker/kind 运行时；当前本机 Docker daemon 未运行，相关验收证据需在推送远端并具备 kind 环境后补。

---

## 4. 非目标（沿用既有边界）

- 通用 PaaS / DevOps / 应用商店 / 完整 KubeSphere 对等。
- 任意 YAML/CRD 编辑、Pod exec / WebShell、Secret 值管理、跨集群 restore / cutover。
- 动态 OPA/Rego 引擎、KubeEdge、GPU 调度、自定义 PromQL/LogQL、Grafana 导入。
- 本计划不为"功能数量"突破这些边界；只提升"可交付、可验证、可规模"三层成立度。