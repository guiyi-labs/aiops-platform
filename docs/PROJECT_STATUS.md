# 项目进度与交接状态（Project Status & Handoff）

> 生成时间：2026-08-10 · 维护人：开发 Agent
> 当前功能基线：M97 RC 本地严格供应链包与双路径生命周期已验证；Hosted GitHub Release 被 required CI 失败阻塞
> 适用场景：项目阶段性收尾，准备打包迁移到新环境继续开发。

---

## 1. 当前基线

| 项 | 值 |
|---|---|
| 仓库 | `k8s-aiops`（Go 后端 `backend/` + Vue 前端 `frontend/`） |
| 默认模块路径 | `k8s-aiops.local/backend` |
| 最新功能基线 | M97 `aiops.release-manifest/v1`、双架构 OCI 资产、Helm/Kustomize/离线包、SBOM/provenance 入口与严格校验（见 `2026-08-10-m97-release-candidate-closure.md`） |
| 里程碑范围 | **M1 – M97 + W10–W12**（M93-C 科技主题、M93-B2 登录性能预算、M94 诊断叙事/行动区/深链、M95 统一证据模型、M96 规模证据及 M97 RC 供应链） |
| 远程同步 | M97 `main`、`baseline-m97-release-candidate-tooling-20260810` 与 `v0.3.0-rc.1` 已同步；Release run `31376784927` 因既有 Backend lint 与 M96 前端规模 invariant 失败，Hosted package 被跳过且未创建 GitHub Release |
| Go / Node | go 1.26.5 / node 22（前端构建用镜像内 pnpm 11.7.0） |

---

## 2. 里程碑与文档覆盖

- **CHANGELOG.md**：覆盖 M1–M97 全部 `Added/Changed/Fixed` 条目（Unreleased 含 W10–W12 / M88 / M91 / M92 / M93-A / M93-B1.1 / M93-C / M93-B2 / M94 / M95 / M96 / M97）。
- **docs/changes/**：147 份变更记录 + 1 份模板。M21–M95 每个里程碑均有独立 `YYYY-MM-DD-mXX-*.md` change-record（M61–M66、M74、M75 已于 2026-08-09 补齐）；M1–M20 早期以主题文档（认证、引导、集群接入等）形式归档。
- **已知文档缺口（低风险）**：M1–M20 以主题文档形式归档（无独立 mXX 编号文件）；M61–M66/M74/M75 已补齐独立 change-record。

### 优化中心（M67–M78，纯只读分析器）
| 里程碑 | 包 | 关键规则 |
|---|---|---|
| M67 网络策略态势 | `internal/netpolicy` | 4 family / 11 code，含 3 个 critical（无后端 Service、端口不匹配、暴露且无 NetworkPolicy） |
| M68 镜像供应链 | `internal/imagepolicy` | 5 code（可变 Tag=warning，其余 info） |
| M69 GitOps 漂移 | `internal/gitopsdrift` | GITOPS_DRIFT_DETECTED=warning |
| M70 容量趋势 | `internal/capacity` | CAPACITY_SATURATION_RISK（30 天内≥80% 或短窗≥100%→critical） |
| M71 策略合规 | `internal/policy` | 4 family / 11 code（PRIVILEGED=critical） |
| M72 拓扑并行化 | `internal/topology` | worker 池默认并发 4（性能改动，非契约） |
| M73 kind E2E | `scripts/e2e-m46-m60-kind.ps1`（M75 扩至 M60） | 可弃 kind 端到端 |
| M76 HPA 姿态 | `internal/hpaposture` | HPA 缩放姿态 |
| M77 PDB 保护 | `internal/pdbposture` | PodDisruptionBudget 保护 |
| M78 Ingress 暴露面 | `internal/ingressposture` | 无 TLS / 死后端 / 通配符 host / 未显式 ingressClassName |

### 打磨期（M80–M84，polish-plan P0/P1/P2 已落地部分）
| 里程碑 | 内容 |
|---|---|
| M80 | 聚合治理态势视图（W4 posture）+ UI motion 打磨基线（aurora 登录、count-up 指标滚动、premium 动效层） |
| M81 | AIOps 端到端闭环（W5）：findings → 巡检佐证（M52）→ 确定性诊断（M43）→ AI 引用（M55）→ dry-run 预览（M19） |
| M82 | 黄金回归发现契约（W6）：analyzer_discovery 场景，DatasetVersion → 1.1 |
| M83 | 拓扑深化（W7）：Gateway API 只读 + collapse 折叠/聚合 |
| M84 | 测试强度（W8）：14 fuzz + 4 benchmark + 核心包 ≥70% 门禁；全局覆盖率 60.03%（CI 门禁 50%→60%） |
| M85 | 前端质量（W9）：Playwright 双视口 14/14、console error=0、统一 motion 层（并入 `a75d357`） |
| M86 | 契约与 API 治理（W10）：错误码审计 + OpenAPI schema 修复 + `pnpm typegen` + CI sync gate + `insight.ts` 消费生成类型 |
| M87 | W11 UX 精细打磨：premium 交互层 + axe 双视口 0 critical/serious + bundle 门禁 |
| M88 | 发布闭环本地化：release-verify 脚本 + SHA256SUMS + cosign fail-closed 签名门禁 |
| W12 | 真实集群 kind E2E 证据（诊断/联邦/全局搜索三套）+ 前端构建契约修复 |
| M91 | 前端规模性能：useVirtualList 窗口化虚拟滚动 + 6 条单测 + sticky 表头 |
| M92 | 登录页交互视觉：Canvas2D 粒子网络 + SVG 多集群拓扑 + reduced-motion / 移动端降级 |
| M93-A | 登录页数据真实性：移除 12 / 186 / 99 硬编码“实时”指标；能力卡替代；确定性 API fixture；Playwright/axe 双视口 28/28 |
| M93-B1 | 控制平面登录动效：认证状态联动、SVG 节点坐标修复、ResizeObserver/Visibility/动态 reduced-motion、移动端首屏；Playwright/axe 38/38 |
| M93-B1.1 | 全屏登录页收口：全视口粒子/拓扑场景、暗色悬浮表单、自动填充适配、移动端首屏边界断言 |
| M93-C | 科技主题控制台：登录与控制台视觉统一、侧栏持久化折叠、永久路由底板、无透明度整页转场；Playwright/axe 42/42 |
| M93-B2 | 登录页性能预算：LoginView 专属体积统计、三模式采样（desktop/mobile/reduced-motion）9 visits 0 失败、版本化基线 JSON、CI 报告模式探针 |
| M94（第一步） | 诊断叙事：根因卡 + 只读证据时间线（六类分类、SHA-256 完整性、缺失语义）、OpenAPI 增量 schema、前端时间线渲染、Playwright 44/44 |
| M94（第二步） | 行动区：类型化只读建议/受控动作（dry-run+确认）、无权限与依赖降级提示、OpenAPI `DiagnosisAction`、Playwright 46/46 |
| M94（第三步） | 深链：资源详情/工作负载与相关事件/审计入口，纯只读导航，Playwright 50/50 |
| M95 | 统一证据模型：`FindingDetail v2`（规则身份/证据引用/类型化建议/版本信息）、v1→v2 兼容层、共享严重度映射、按资源合并保留规则来源、golden DatasetVersion 1.2 + 迁移提示；前端 Posture/Optimization/Diagnosis/Inspection 共享证据组件；11 posture 分析器 + finops schema parity；前端门禁 135 单测 / 56 浏览器回归 |
| M96-A/B/C/D + Gate B | `m96-v1` 确定性规模 fixture（500 Node / 50k Pod / 100k Event）、fixture-backed 后端 report-mode 基准、前端 50k Pod DOM/交互基线（6 visits，0 failures，0 invariant failures）、认证单壳层与 active CSS layer 基线（56/56 浏览器回归）；本地 Gate B 聚合 PASS，Hosted CI 待运行 |
| M97 | `aiops.release-manifest/v1`、RC-only workflow、双架构 OCI、四份平台 SPDX SBOM、Helm/Kustomize/离线资产、严格 checksum/Cosign 与 kind 生命周期；本地 Gate C 证据已通过，M89/M90 与 Hosted GitHub Release/keyless 证据仍为 Blocked/Deferred |

> 当前执行入口：`docs/next-long-term-plan.md`。M93-B2、M94、M95、M96 本地 Gate B 与 M97 本地 Gate C 已归档；M97 远端 run `31376784927` 的失败已记录，本轮按用户要求在此暂停，不进入 M98；
> M89 生产身份与 M90 数据可靠性继续作为组织授权轨，未完成时版本保持 RC。

---

## 3. 架构与模块速览

- **后端**：模块化单体。`internal/<domain>/`（model+service+test）→ `internal/optimization/collector.go` → `internal/httpserver/` → `router.go` → `docs/api/openapi.yaml`。
- **前端**：`frontend/src`，类型与 API 客户端在 `types/optimization.ts`、`api/optimization.ts`，优化中心 11 个 tab（`OptimizationView.vue`）。
- **分析器契约**：`Evaluate(clusterID, Inputs, time) → Status`，findings 复用 `internal/finding`（code/severity/family/remediation）。

---

## 4. 构建与部署

### 4.1 构建镜像
```bash
# 后端（多阶段，产出 api/credential-reencrypt/audit-archive/identity-readiness/recovery-readiness）
docker build -f backend/Dockerfile -t k8s-aiops-backend:dev backend
# 前端（镜像内 pnpm install + build，nginx 托管 dist）
docker build -f frontend/Dockerfile -t aiops-platform-frontend:dev frontend
```

### 4.2 本地测试栈（docker compose）
```bash
docker compose up -d        # postgres + backend + frontend
# 首次启动若报密码认证失败：postgres 卷残留旧密码，执行下方重置
docker compose down -v && docker compose up -d
curl http://127.0.0.1:8080/api/v1/health/ready   # 期望 {"status":"ready"}
```
> 默认凭据（仅开发/测试）：admin / `change_me_now`，Postgres `aiops/change_me`。**生产必须替换**。

### 4.3 Kubernetes
- `deploy/kubernetes/`（kustomize）：`kubectl apply -k deploy/kubernetes/`；Secret 需基于 `deploy/kubernetes/secret.example.yaml` 在 Git 外生成。
- `deploy/helm/aiops-platform`：官方 Helm 图表（M38），Secrets 必须由外部提供。

---

## 5. 测试与质量门禁

- **CI 门禁**（` .github/workflows/ci.yml`）：gofmt / go vet / 覆盖率门禁 / 5 个二进制构建 / golangci-lint / eslint / vue-tsc / vitest / vite build，全量通过。
- **端到端**：`scripts/e2e-*-kind.ps1` 系列，使用真实 kind 集群（L3 真实环境级）。
- **本次测试环境验证（2026-08-02）**：见第 7 节。

---

## 6. 已知缺陷与本次修复

- **e2e 脚本 bug（已修复）**：`scripts/e2e-m46-m60-kind.ps1` 创建工作区时 `metadata = '{}'`（PowerShell 字符串），被序列化为 `"metadata":"{}"`，触发后端 `WORKSPACE_INVALID_INPUT`（要求 JSON 对象）。改为 `metadata = @{}`（对象）后通过。后端严格校验行为正确，非产品缺陷。
- **本地 docker 构建缓存致镜像陈旧（环境事项，非产品缺陷）**：本机 `docker compose build` 复用了旧的 `go build` 层，导致运行中的后端二进制不含最新 main 的 Inspection(M52)/Golden(M56) 路由（运行时该两服务在旧二进制里为 nil，路由未注册，返回 404；而 M57/M58/M60 正常）。CI 使用 `docker buildx` 在干净环境构建不受影响。本地验证已改用主机 Go（GOPROXY=goproxy.cn）交叉编译 linux/amd64 二进制并直接打镜像，确保二进制为最新源码。
- **M52 巡检 plan 创建 500（产品缺陷，已修复）**：`POST /api/v1/aiops/inspection/plans` 报 `INSPECTION_FAILED`，后端日志 `failed to parse field: ClusterIDs, error: unsupported data type: &[]`。
  - **根因 1**：`inspection/model.go` 的 `Plan.ClusterIDs` / `RuleCodes`（GORM 模型）为 `Int64Array`/`StringArray`，但**缺少 `gorm:"type:bigint[]"` / `type:varchar(128)[]` 字段 tag**。GORM 的 postgres dialector 对无 tag 的 slice 字段先按原生 slice 解析（校验失败并污染 `db.Error`），带显式 `type:` tag 后才直接走数组绑定。backup 模块因 `included_namespaces text[]` 带 tag 一直正常。
  - **根因 2**：`createPlan` handler 直接返回 GORM 内部 `Plan`（无 json tag）而非 `PlanView`，JSON 序列化为 `{"ID":1,...}`（大写 Go 字段名），**违反 API 小驼峰契约**（`getPlan`/`updatePlan` 均返回 `PlanView`）。
  - **修复**：① `model.go` 给 `Plan`/`Task` 数组字段加 `gorm:"type:..."` tag；② 同时新增 `Int64Array`/`StringArray` 类型（实现 `driver.Valuer`/`sql.Scanner`，基于 `lib/pq`）并全链路显式转换（`service.go`/`repository.go`/`httpserver/inspection.go`）；③ `createPlan` handler 返回 `inspection.PlanView`（小驼峰）。**已用真实 Postgres 集成测试复现并确认修复生效**。

---

## 7. 本次测试环境验证（2026-08-02）

> 环境：Windows + Docker 29.6.2 + kind v0.30.0 + kubectl v1.34.0
> 拓扑：docker-compose 起控制面（postgres+backend+frontend），e2e 自建 kind 成员集群注册验证。

| 步骤 | 结果 |
|---|---|
| 镜像重建（backend+frontend，docker compose build） | ✅ 构建成功（但命中旧 `go build` 缓存层 → 二进制陈旧） |
| compose 控制面启动 + 后端 ready | ✅ healthy |
| postgres 卷密码残留导致认证失败 → 重置卷 | ✅ 已解决（测试环境数据残留） |
| e2e 第一次：M46 因脚本 `metadata` 字符串 bug 失败 | ✅ 定位并修复脚本 |
| e2e 第二次：M52 因旧镜像缺 Inspection/Golden 路由 404 | ✅ 定位为本地构建缓存问题 |
| 改用主机 Go 交叉编译 linux/amd64 并直接打镜像（绕过容器内模块下载） | ✅ 采用 `DOCKER_BUILDKIT=0` legacy builder + 预编译二进制 |
| **M48 联邦注册 + probe（真实 kind 集群）** | ✅ `probe=ready`，PASS |
| **M52 巡检 catalog + plan 生命周期** | ✅ PASS（修复两个产品 bug，见第 6 节） |
| **M57 app-catalog plan 查询** | ✅ PASS（`{"items":[],"total":0}`，带 cluster_id） |
| 全量单元测试 `go test ./...` | ✅ 40+ 包全部通过，无回归 |

**最终验证（2026-08-02）：**
```
PASS M48 federation (cluster_id=8 probe=ready seen=1)
PASS M52 inspection catalog + plan lifecycle(id=2)
PASS M57 app-catalog plans
RESULT M48/M52/M57: PASS=3 FAIL=0
```
> 注：本地镜像构建须用 `DOCKER_BUILDKIT=0 docker build`（沙箱环境的 buildx `activity/.tmp-*` rename 被拒，Access denied）。CI 的 buildx 不受影响。

（e2e 完成后的 summary.json 路径：`.artifacts/m46-m60-kind/summary.json`）

---

## 8. 遗留 / 延期项（非阻塞）

- **M26 外部门禁未实现**（按 `docs/next-development-plan.md` 需组织授权）：
  - 生产级 OIDC/MFA（M26B）：仅有离线就绪检查，未实现。
  - 生产级 PITR / HA（M26B）：未实现。
- 上述属于"外部决策门禁"，在缺少组织授权时不阻塞项目收尾，但标注为 deferred。

---

## 9. 敏感信息安全态势

专项检查（2026-08-02）结论：**通过，无敏感信息泄露风险**。
- 仓库内无真实 `.env`、`.key`、`.pem`、`.kubeconfig` 被跟踪或历史强制提交（仅 `.env.example` 模板）。
- 工作树与全部 git 历史均未检出真实密钥 token（OpenAI `sk-`、Slack `xox`、JWT、AWS `AKIA`、GitHub `ghp_`、GitLab `glpat_` 精确正则全 0 命中）。
- 仅 7 处 `CHANGE_ME` 占位符与开发默认值（compose 默认口令）。
- 打包迁移目录（含 `.git`）不会带入密钥。

---

## 10. 新环境上手步骤

```bash
git clone <this-repo> aiops-platform
cd aiops-platform
docker compose up -d              # 本地起测试栈
# 或使用 kind 跑 e2e：
powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/e2e-m46-m60-kind.ps1
```
- 替换所有 `CHANGE_ME` / `change_me*` 默认值为真实密钥后再投产。
- 数据库迁移在后端首次启动时自动执行（`backend/migrations/`）。

---

## 11. 交付物清单（本次收尾）

1. M67–M73 里程碑 change-record 文档（7 份，`docs/changes/`）。
2. M76/M77/M78 change-record + CHANGELOG（先前已完成）。
3. e2e 脚本 `metadata` bug 修复（待提交）。
4. 本交接状态文档 `docs/PROJECT_STATUS.md`。
5. 敏感信息专项检查（第 9 节，通过）。
6. 可移植目录（含 `.git` 完整历史，见打包步骤）。
