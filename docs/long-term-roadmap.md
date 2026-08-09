# 长远计划：把 AIOps 平台打磨到业内最高水准

- Status: Active（执行基线）
- Updated: 2026-08-09（对齐 `a0874e3` + W11 关闭 + 全量推送完成）
- Baseline: `main` @ `a0874e3`（与 `origin/main` 同步 0/0，工作树干净，18 提交与 9 个新 tag 已推送）
- 关系：详细打磨契约见 [`docs/polish-plan.md`](polish-plan.md)；本文件是面向"长远方向 + 分阶段执行 + 已落地清单"的路由图。
- 原则：不推翻既有架构决策、安全边界与非目标；每一步都带可验证的证据与验收标准。

---

## 0. 当前已落地进度（2026-08-09 复验）

本地 `main` 与 `origin/main` 已同步；`baseline-m85-w8w9` / `baseline-w11` / `baseline-m86` / `baseline-m87` 等 9 个新 tag 均指向当前 HEAD。

| 提交 | 内容 | tag |
|---|---|---|
| `591c19b` | polish: UI motion 打磨基线 + polish-plan 落库 | `baseline-polish-20260808` |
| `afa1e93` | M80 聚合治理态势视图（W4 posture） | — |
| `9077c4d` | count-up 指标滚动、aurora 登录背景、premium motion 层；新增 useCountUp + 4 条 Vitest | `baseline-ui-motion-20260808` |
| `48af578` | M84 测试强度（W8 主体）：14 个 fuzz、4 个 benchmark、核心包 ≥70% CI 门禁 | `baseline-m84-20260809` |
| `290e52e` | M81 AIOps 端到端闭环（W5）：findings → 巡检佐证（M52）→ 确定性诊断（M43）→ AI 引用（M55）→ dry-run 预览（M19） | `baseline-m81-20260809` |
| `5d01974` | M82 黄金回归发现契约（W6）：analyzer_discovery + posture/engine/diagnosis/inspection 快照，DatasetVersion → 1.1 | `baseline-m82-20260809` |
| `288d824` | M83 拓扑深化（W7）：Gateway API 只读接入 + collapse 折叠/聚合参数 + 4 条新测试 | `baseline-m83-20260809` |
| `a75d357` | W8/W9 闭环（M84/M85）：全局覆盖率 60.03%、CI 门禁 60%、Playwright 双视口 14/14、console error=0、motion.css 统一动效 + SkeletonCard/EmptyState | `baseline-w8w9-20260809` |
| `9fcb9a1` + `113e88b` | W10 错误码审计：全路由 writeError + `VELERO_UNAVAILABLE` → 503（backup/restore 3 处 + change-record） | — |
| `258c2e7` + `f282e96` | W10/M86 OpenAPI 契约修复：重复 schema、缺失 schemas 与参数修复 + `pnpm typegen` + CI sync gate + `insight.ts` 消费生成类型 | `baseline-m86-20260809` |
| `55e8ebd` + `3b744e1` + `a0874e3` | W11：premium 交互层（glass/发光/按压等）+ axe 双视口 0 critical/serious a11y 扫描 + bundle 体积门禁 + 文档基线同步 | `baseline-w11-20260809` / `baseline-m87-20260809`（指向同一基线） |

验证状态（2026-08-09 全量复验）：
- 后端：gofmt / vet / build / `go test ./...` 全绿；全局覆盖率 60.03%（CI 门禁 50% → 60%）。
- 前端：typecheck / eslint / vitest（22 files · 124 tests）/ vite build / Playwright Desktop+Mobile 14/14，console error=0；axe 双视口 critical/serious 为 0；bundle 门禁入 CI。
- CI：OASdiff 破坏性检查、typegen sync gate、覆盖率门禁、bundle 门禁均已接入。
- 环境：Docker daemon 29.6.2 + kind v0.30.0 + kubectl v1.34.0 就绪；`.env` 已生成（production、强随机密钥、AI/NOTIFICATION 关闭）；compose 后端 + 前端 + Postgres 尚未启动。

**真实集群证据缺口（下一步核心）**：W3（诊断 / M46 联邦 M48）、W5 / W7 关键路径的 kind 真实集群 E2E 尚未在本地产生 `.artifacts/<suite>-e2e/*.json` 证据（当前 `.artifacts/` 为空）。需要在 compose 起来后依次跑 `e2e-diagnosis-kind.ps1`、`e2e-fleet-kind.ps1`、`e2e-global-search-kind.ps1`，产出证据并归档到 change-record。

---

## 1. 五条可度量维度（对齐 polish-plan 五条基线）

| 维度 | 打磨目标 | 当前进度 | 长远收尾方向 |
|---|---|---|---|
| AIOps 差异化 | 聚合治理 + 端到端闭环 + 黄金回归全覆盖 | W4/W5/W6 已闭环；W7 拓扑代码完成（500 节点真实集群验证待补） | 聚合洞察、"给定集群 10 秒内看清根因"的叙事级演示稿（thesis/defense） |
| 工程卓越 | 核心包 ≥70%、全局 ≥60%、fuzz/benchmark、浏览器 E2E、契约一致性、a11y/bundle 门禁 | W8/W9/W10/W11 全落地（实测 60.03%、Playwright 14/14、axe 0 critical） | 全局覆盖率 → 70%+；性能基准（P95 延迟、采集背压）入 CI |
| 数据可靠性 | WAL 归档 + PITR + 多副本演练常态化 | 逻辑备份/恢复演练完整；无 WAL/PITR 与 HA | M90 前先做 PITR 步骤化脚本 + 演练报告模板 |
| 安全与身份 | 组织级 OIDC/MFA + 会话策略 | 本地账号、离线就绪检查、加密凭据、审计完备 | 生产身份（外部 OIDC/JWKS）对接前置准备 |
| 供应链与交付 | 真实可验证签名 + 语义化发布 + 离线安装包 | Cosign keyless 结构占位 + release 工作流存在；无正式 GitHub Release、cosign 真实通过待验证 | 正式版本、Checksums、Cosign 签名验证、离线安装文档 |

---

## 2. 分阶段长远路线（P0 → P3）

### P0：环境启动与真实集群证据收口（本周 · 2–3 天）

- [x] 推送全部提交 + tag（`origin/main` 18 提交、9 个新 tag 已同步）。
- [x] 生成 `.env`（production、强密钥、AI/NOTIFICATION 关闭）、Docker daemon 就绪。
- [x] **启动 compose**：`docker compose up -d --build`（Postgres + backend + frontend），`/api/v1/health/ready` → `{"status":"ready"}`；修好前端构建缺陷后全栈 healthy。
- [x] **真实集群 E2E 证据**（kind v0.30.0 / k8s v1.34.0，脚本自建自清理，证据见 `.artifacts/<suite>-e2e/*.json`）：
  - `scripts/e2e-diagnosis-kind.ps1`（W3 诊断链路真实集群行为）
  - `scripts/e2e-fleet-kind.ps1`（M48 联邦注册 + probe 等多集群场景）
  - `scripts/e2e-global-search-kind.ps1`（M53 global search 跨集群）
- [x] 将 `.artifacts` 证据摘要写进 change-record（`docs/changes/2026-08-09-w12-real-cluster-e2e.md`）。

### P1 — AIOps 差异化深耕（已闭环，续推即深化）

- [x] 聚合治理态势（W4/M80）、端到端闭环（W5/M81）、黄金回归发现契约（W6/M82）、拓扑深化（W7/M83）。
- [ ] **W1 续深化候选**（以 A/B 优先级，每项带验收与证据）：
  - 3.1 诊断友好化：以"根因卡片 + 证据引用 + YouTube 风格回放"把 M81 闭环再包装成可交付的演示叙事。
  - 3.2 洞察可解释分层：在聚合态势页为每类分析器给出"规则 → 证据 → 建议"三层可下钻 UI。
  - 3.3 大规模拓扑渲染：500 节点 fixture + kind 验证，必要时引入虚拟滚动（与 M91 联动）。
- [ ] 每个深化项：ADR → 实现 → 单测 → `verify-fast.ps1 -Scope All` → kind E2E（涉及集群行为时）→ change-record + CHANGELOG → volatile 不合并。

### P2 — 工程卓越夯实（W8/W9/W10 已收；再往上补强度）

- [x] 覆盖率门禁 / Playwright / OpenAPI 契约 / typegen / a11y / bundle 均入 CI。
- [ ] **A 全局覆盖率 60% → 70%**：按 CI 报告逐包补充核心逻辑测试，只影响已存在分析器，不改契约。
- [ ] **B 性能与容量基准**（可与 M91 合并执行）：
  - 新增 `httptest` 基准：100k 事件 metricshistory 窗口、500 节点 / 50k pod 拓扑 fixture 渲染。
  - 在 CI 加性能回归（阈值宽松、失败提示 review）。
- [ ] **C CI 稳健性**：截图/证据 artifact 上传与归档；覆盖率报告链接汇总到 change-record。
- [ ] **D 无回归门禁**：`go test -race ./...` 已入 CI 时确保运行时长可控；E2E 加超时与资源上限。

### P3 — 交付与生产就绪（优先本地可执行部分）

- [x] **M88（正式发布闭环，本地化部分已完成）**：
  - 语义化 tag / Release 流程接入真实校验；cosign key-less 移除 `|| true` 做真实 verify（需要 GitHub token / 组织授权）。
  - 产出 `SHA256SUMS` + signature + provenance 可验证；离线安装包（`docker save` + 迁移 tar）与文档。
  - 授权缺口已记录在 `docs/changes/2026-08-09-m88-release-loop.md`：GitHub Release 创建 / Fulcio keyless 签名待 runner 与授权。
- [ ] **M89 生产身份（组织授权）**：OIDC/MFA 对接（M26B 前置审批）。
- [ ] **M90 数据可靠性（组织授权）**：WAL 归档 → PITR 演练 → 多副本优雅停机 → 迁移回滚演练。
- [ ] **M91 前端虚拟滚动 / 大规模性能（本地可做，M90 语义保持兼容）**：拓扑/工作负载列表虚拟滚动、聚合缓存、P95 延迟基准。

---

## 4. 建议的下一步顺序（本周每天最多一件事）

1. `docker compose up -d --build` → health ready。
2. 跑 3 个 kind E2E 脚本并归档 `.artifacts` 证据 + change-record。
3. 完成 M88 可本地化部分（release 脚本 / 校验文档 / CI 门禁），组织授权缺口明确记录。
4. 刷新文档（README / PROJECT_STATUS / change-record / CHANGELOG / tags）并提交 + 推送。
5. （可选）P2 A/B 补齐覆盖率与性能基准。

> 依赖提示：kind E2E 与发布校验需要 Docker/kind 与组织授权；无授权时记录证据缺口而不是假装通过。

---

## 5. 方向探索（长期候选，不承诺范围）

| 方向 | 说明 | 前置 | 价值 |
|---|---|---|---|
| 可观测性深化 | 日志/指标联动、告警降噪告警、SLO 错误预算扩展到更多工作负载 | M91（规模与性能链路） | 提升"健康度一眼可见" |
| 供应链可验证 | SBOM（CycloneDX）入库、依赖合规扫描入 CI、镜像成分展示 | M88 发布闭环 | 供应链透明 |
| 身份升级 | OIDC/JWKS、MFA 与设备会话 | 组织授权 | 生产安全 |
| 数据韧性 | WAL/PITR 演练自动化、恢复演练报告入 CI 证据 | M89/M90 数据链路 | 生产可靠性 |
| 规模与性能 | 500+ 节点拓扑、虚拟滚动、聚合缓存、P95 基准 | P2-B / M91 | 规模验证 |
| 可视化梳理 | thesis/defense 演示稿、截图更新、test-matrix 自动比对 | P0/P1 完成 | 对外叙事 |

---

## 6. 非目标（沿用既有边界）

- 通用 PaaS / DevOps / 应用商店 / 完整 KubeSphere 对齐。
- 任意 YAML/CRD 编辑、Pod exec / WebShell、Secret 值管理、跨集群 restore / cutover。
- 动态 OPA/Rego 规则引擎、KubeEdge、GPU 调度、自定义 PromQL/LogQL 编辑、Grafana 导入。
- 本计划不为"功能数量"突破这些边界；只提升"可交付、可验证、可持续"三方层级。

---

## 7. 附：当前环境与下一步执行入口

```powershell
cd C:\BS\aiops-platform
docker compose up -d --build
curl http://127.0.0.1:8080/api/v1/health/ready
$env:AIOPS_ADMIN_PASSWORD = (Get-Content .env | Where-Object { $_ -like 'BOOTSTRAP_ADMIN_PASSWORD=*' } | ForEach-Object { $_ -replace '^BOOTSTRAP_ADMIN_PASSWORD=','' })
.\scripts\e2e-diagnosis-kind.ps1
.\scripts\e2e-fleet-kind.ps1
.\scripts\e2e-global-search-kind.ps1
```