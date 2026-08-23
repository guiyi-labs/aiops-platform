# P2 旗舰深化总方案 — aiops-platform 四方向全做

- Date: 2026-08-16
- Status: Draft（待批复）
- Author: command-center 派发 / 用户决策「aiops 成为唯一深化对象，四方向全做」

## 1. 决策与目标

本仓（`guiyi-labs/aiops-platform`）成为唯一深化对象。在保持既有门禁
硬约束（覆盖率 ≥70%、race/lint 0 issues、noreply 身份、AGENTS.md 归档、
每次提交门禁 `go test ./...`）不变的前提下，四个方向**全部**落地，把仓库
打磨成「多集群 + 确定性诊断 + RAG 演示 + 工程卓越」旗舰样例。

### 现状基线（2026-08-16）

- HEAD `a889b2a`（P1 RAG 知识库 6-commit 已推送）；CI 全绿。
- 全局覆盖率 **70.1%**（CI 门禁 70.0%），核心包门禁各自 ≥70%。
- Federation 已有：Host/Member 拓扑、心跳、Register/Deregister、
  ResourceSummary fan-out、FederationEvent、Overview（M48）。
- Fleet：健康 fan-out（跨集群节点/负载抽样，带超时与截断呈现）。Globalsearch：
  跨集群固定范围搜索 + 用户私有过滤器。
- P1 RAG：知识库（knowledge 包）+ 诊断入库钩子 + aiexplain 注入 +
  aiinvestigator 历史参考（`historical:N` 可引用证据，精排默认关闭）。
- README 270 行，基线与 M115 一致但**无架构图思路、无量化证据徽章行、
  无快速上手**；CHANGELOG Unreleased 累计多里程碑。
- CI：gofmt → golangci-lint v2.12.2 → 全局覆盖率 ≥70（`go test -cover -p=1
  -count=1 ./...` + `go tool cover -func | tail -1`）→ 核心包 ≥70 → race
  （`-race -p=2`）→ Frontend Playwright → Gate B 等。**无 gosec/trivy**。

## 2. 四方向总览与阶段划分

| 阶段 | 方向 | 内容 | 依赖 | 可并行 |
|---|---|---|---|---|
| P2a | ⑤CI 门禁升级 | 覆盖率上调评估、gosec/trivy 评估、lint 规则补充 | 无 | ✅ 独立先行 |
| P2b | ⑤README/文档精修 | README 旗舰重写、快速上手、架构思路、徽章行、ADR 一致性 | 无（少量依赖 P2a 徽章数值） | ✅ 与 P2a/P2c 并行 |
| P2c | ②RAG 深化 + 演示打磨 | 示例知识库灌入脚本、e2e 演示链路、演示文档/截图 | P1 已完成（知识库就绪） | ✅ 与 P2a/P2b 并行 |
| P2d | ①P2 多集群 Federation 深化（主任务） | 跨集群诊断/运维聚合视图、跨集群巡检 | 依赖前述全绿基线及 Federation 既有基础；**不依赖 P2a-c** | 最后做（工作量最大、风险最高） |

依赖关系图：

```
P2a (CI) ─┬─> P2b (README 徽章行数值引用 P2a 提升后的覆盖率)
          │
P2c (RAG 演示) ─── 独立
P2d (Federation 深化) ─── 最后（独立于 P2a-c，仅依赖基线稳定）
```

排序理由：P2a/P2b/P2c 三方向互不阻塞、改动面小、回报快，先行兑现
「演示可讲」与「工程门禁可晒」；P2d 是最大工作量与最大求职价值，放在
最后集中攻坚，此时 CI/文档已稳定，风险隔离。

## 3. 各阶段详案

### P2a — CI 门禁升级（阶段 1，先行）

**目标**：把「覆盖率/安全/静态分析」做成可量化晒点，且不破坏现有全绿。

- **覆盖率门限**（分两步）：
  - `P2a-1`：全局门禁 70.0% → **75.0%**（同步 CI yml 与 README/CHANGELOG
    描述）。需先量化：当前 70.1%，缺口约 5 个百分点 ≈ 1300+ 语句。按
    M115 经验逐包提测（knowledge/diagnosis/aiexplain/aiinvestigator 已
    高覆盖；剩余低覆盖大头：cmd/server、cluster 网关、federation）。
  - 若 75% 全局成本过高（评估见 §5），替代：*核心包列表门禁 ≥75% +
    全局保持 70%*（CI 已有核心包门禁机制，扩展列表即可）。**先做核心包
    75%**，全局 75% 视缺口量化后定。
- **gosec**：评估成本——轻量引入 `gosec` 于 CI（只 `go vet`+`gosec` 两
  个命令，不改依赖实质）；首轮扫出问题逐条修（预期集中在
  cmd/server、credential-reencryption、crypto）。若问题过多，降级为
  `gosec -severity high` 白名单（G104 等已知噪声）。
- **trivy**：评估成本——trivy 扫容器仓扫描（backend 镜像 base 层）。
  CI 现有 docker build 阶段可挂 `trivy image --exit-code 1`；若本地暂无
  镜像仓库基础设施，降级为 `trivy fs --skip-dirs .git node_modules` 扫
  源码依赖（go.sum / package-lock）。
- **lint 补充**：golangci-lint 增启 `gosec` + `govet` 已有 + `errcheck`
  + `unparam`（若噪声可控）；否则维持现状只增 gosec。

**Commit 划分**（估算 3-5 commit）：
1. 核心包 75% 门禁代码 + CI yml 调整（先核心后全局）
2. 补测提覆盖提到 75%（视缺口 2-4 commit）
3. gosec 引入 + 修复（1-2 commit）
4. trivy 评估/落地（0-1 commit）
5. lint 规则调整（可与 3 合并）

**覆盖率影响**：提测本身提高全局覆盖率（+5pp 目标）；新 CI 代码不计入
语句。全局 70.1% → 75% 需补测，见 §5 成本评估。

### P2b — README/文档精修（阶段 2，与 P2a/P2c 并行）

**目标**：README 达到「旗舰仓库」观感，文档体系自洽。

- **README 重写**（核心交付）：
  - 开头：项目一句话 + 架构图（Mermaid `flowchart`：多集群 → 网关 →
    确定性诊断 → RAG 知识库 → AI 增强 → 控制台），替代现有纯文字。
  - 量化证据徽章行：Go 1.26 / Vue 3 / K8s 1.36 / CI passing / **覆盖
    率 75%（P2a 后）** / linters 0 issues / **gosec / trivy clean**。
  - 快速上手：`make` 起集群（kind? 现有 e2e 用 docker compose）→
    `go run ./cmd/server` → 打开本地控制台 → 演示路径索引（见 P2c）。
  - 目录重整：项目简介 / 核心能力矩阵 / 快速上手 / 架构 / 演示剧本 /
    工程指标 / 路线图 / 边界与免责。
- **ADR 一致性**：检查 `docs/adr/` 与当前实现是否矛盾（重点：AI 不直接
  执行变更——P1 已遵守；门禁 75% 需同步 ADR-0003 之类覆盖门禁记录）。
- **CHANGELOG**：Unreleased 区段重整，把 P1/P2 各阶段条目压实成里程碑
  叙述风格，避免碎片化。

**Commit 划分**（估算 2-3 commit）：
1. README 旗舰重写（含 Mermaid 架构 + 快速上手 + 徽章行占位）
2. ADR/CHANGELOG 一致性修正
3. （可选）docs/ 索引与演示截图链接整理（依赖 P2c 产出）

**覆盖率影响**：零。

### P2c — RAG 深化 + 演示打磨（阶段 3，与 P2a/P2b 并行）

**目标**：让面试能直接演示「问题 → 检索 → AI 综合 → 带依据回答」闭环。

- **示例知识库灌入脚本**（`scripts/seed-knowledge.sh` 或 Go cmd）：
  向 `knowledge_entries` 灌入 6-10 条精心构造的已解决案例
  （crash_loop / OOMKilled / node_not_ready / ingress_backend /
  metric_breach / hpa_saturated 各 1-2 条），含真实感 root_causes /
  recommendations / resolved_at。脚本幂等（先清空再灌）。
- **演示链路打通**：
  - 起本地栈（现有 docker compose）→ 制造一个 crash_loop 诊断 →
    确认 resolved → 自动入库（P1 钩子已验证）→ 再触发同规则新诊断 →
    aiexplain 出「历史相似案例」段落（**mock 模式**，见 §边界）。
  - 后端新增 `GET /api/v1/aiops/knowledge`（只读）便于演示巡检与验证。
  - 前端「历史案例」引用渲染已就绪（P1 commit 6）——补一个演示用
    诊断详情页截图路径。
- **演示文档**：`docs/demo-rag-diagnosis.md` —— 分步剧本 + 预期输出
  （含 prompt 注入段与 citation 截图占位）、常见故障排查。
- **演示截图**：`docs/screenshots/` 补 RAG 演示 2-3 张（前端诊断详情 +
  历史引用块 + 知识库只读页）。

**边界如实**：演示用 **mock 或本地 OpenAI-compatible stub**（现有
ResponsesProvider 打点可指向本机 stub），**不依赖真实 LLM key**；CI 不跑
演示（无 key、无浏览器）。精排保持关闭（结构化相位已够演示）。

**Commit 划分**（估算 3-4 commit）：
1. knowledge 只读查询端点（Handler + 测试）
2. seed 脚本 + 幂等测试
3. 演示文档 + 截图
4. （可选）前端知识库只读页

**覆盖率影响**：新 Handler 提测后全局 +0.1~0.3pp（正贡献）。

### P2d — 多集群 Federation 深化（阶段 4，主任务，最后）

**目标**：在既有 Federation（M48）之上，补「跨集群诊断/运维聚合」这层
最缺的能力，打造「单一视图诊断全舰队」的旗舰卖点。

**现状清楚**：Federation 已有 Host/Member 拓扑、心跳、ResourceSummary
（按 GVR 计数 fan-out）；Fleet 已有健康 fan-out；Globalsearch 跨集群
搜索。缺口：**跨集群诊断聚合**（一个 cluster 出问题 → 一键看舰队内同类
资源/同类规则的全部诊断）、**跨集群巡检**（批量触发 诊断/巡检 →
聚合结果）。

**设计**（两阶段降级策略，边界如实）：
- **P2d-1 聚合视图（真联但受控）**：
  - 复用现有 `federation.ResourceSummary` / `fleet` 的有界并发 fan-out
    模式（MaxClusters=20、PerClusterTimeout=4s、截断显式呈现）。**真联**
    到各集群的只读 Kubernetes 网关（ADR 0004 有界网关）——这是「真实
    能力」而非模拟桩；CI 中该层用 mock 网关（与 M48 一致：federation
    测试走 mock ClusterLister，无真实集群）。
  - 新端点：`GET /api/v1/federation/diagnoses` —— 聚合舰队内诊断记录
    （按 rule_id/severity/status 过滤 + 跨集群时间线）；`GET /api/v1/
    federation/inspect` —— 跨集群批量巡检（触发现有 diagnosis rules，
    聚合结果按 cluster + rule 分组）。
  - 前端：舰队诊断视图（表格：cluster | rule | severity | status |
    resource | resolved_at + 深链到单集群诊断详情）。
- **P2d-2 跨集群巡检（真触发，无副作用保证）**：巡检只读诊断，不触发
  变更执行；dry-run 语义沿用现有 operator 的 dryRun=All 透传。

**边界如实**：
- Federation 是**跨集群真联**（经有界只读网关），不是模拟；但演示环境
  通常只有 1-2 个 kind/compose 集群，多集群场景在 CI 用 mock 网关覆盖
  逻辑，真联路径在本地演示 2 集群时验证。
- 若砸成本过高（每集群真实对账耗时），轻量替代：**聚合视图读平台侧
  diagnosis 表 + federation 拓扑**（不实时 fan-out 到各集群），把「跨
  集群一次查询」做成平台级聚合；实时 fan-out 保留为 Phase-2 增强。

**Commit 划分**（估算 5-7 commit）：
1. federation 聚合查询 service（跨集群诊断/巡检聚合）+ 测试
2. handler + 路由 + 只读端点
3. 前端舰队诊断视图
4. 巡检触发 + 结果聚合 + 测试
5. mock 网关夹具扩展 + e2e（可选）
6. 文档/CHANGELOG/截图
7. （缓冲）门禁修复

**覆盖率影响**：每个 commit 自测 `go test ./...` 全绿；新 service 代码
要求 ≥75%（测试用 mock），全局维持在 70.1%+（新代码正贡献或持平）。

## 4. 总工作量与里程碑

| 阶段 | 估算 commit | 工作量（相对） | 里程碑 |
|---|---|---|---|
| P2a CI 升级 | 4-6 | 中（提测量大但机械） | `baseline-p2a-YYYYMMDD` |
| P2b 文档精修 | 2-3 | 小 | 可与 P2a 同 tag 或 `p2b` |
| P2c RAG 演示 | 3-4 | 小-中 | `baseline-p2c-YYYYMMDD` |
| P2d Federation | 5-7 | 大（主任务） | `baseline-p2d-YYYYMMDD`（旗舰收官） |

合计约 **14-20 commit**。执行顺序：P2a/P2b/P2c 并行开工（三方向互不
阻塞）→ 完成并推送 → P2d 最后攻坚。每阶段结束回报 commit SHA +
验收证据（CI 全绿截图/链接、覆盖率数值、演示文档链接）。

## 5. 覆盖率门禁影响评估（保持 ≥70% 不破）

- **P2a 提测**：目标全局 75%。当前 70.1%，缺口约 5pp。按 M115 经验，
  提测大头在 cmd/server（main 接线分支多、难测）与 cluster/kubernetes
  网关。若 cmd/server 占缺口主要部分，采用**核心包 75%** 方案即可兑现
  （CI 已有核心包门禁机制），全局保持 70% 不再上调——**如实标注**。
- **P2c/P2d 新代码**：每个 service/handler 配 mock 单测，要求新包覆盖
  ≥75%，不影响全局门禁；新代码语句约 300-600 条，全局覆盖率微升。
- **门禁防回退**：每次 commit 前跑 CI 同款 `go test -cover -p=1 -count=1
  ./...` + `go tool cover -func | tail -1` 快照验证 ≥ 现行门限，与
  M115 阶段流程一致。

## 6. 边界与轻量替代汇总（如实）

| 方向 | 主方案 | 轻量替代 | 本期决策边界 |
|---|---|---|---|
| CI 覆盖率 | 全局 75% | 核心包 ≥75% + 全局保持 70% | 先核心 75%，全局视缺口 |
| 安全扫描 | gosec + trivy image | trivy fs（无镜像基建时） | gosec 必做；trivy 视基建 |
| RAG 演示 | mock/stub LLM + seed 脚本 + 文档 | 无 | **不依赖真实 LLM key** |
| Federation | 真联各集群（有界只读网关） | 平台侧聚合（读 diagnosis 表 + 拓扑） | 真联受控；CI 用 mock 网关 |
| AI 执行变更 | 不直接执行（既有纪律） | — | 保持 dry-run/确认链 |

## 7. 验收清单（每阶段回报）

- [ ] 门禁：`go test ./...` 全绿、race 全绿、golangci-lint 0 issues、
      覆盖率 ≥ 现行门限
- [ ] change-record 每 commit 配套 + CHANGELOG 更新
- [ ] noreply 身份；不碰 docs/thesis；未验证不写「已验证」
- [ ] P2a：CI 覆盖率门限（核心 75%）+ gosec 接入证据
- [ ] P2b：README 旗舰重写 + 徽章行数值 + 架构图
- [ ] P2c：seed 脚本 + 演示文档 + 截图、知识库只读端点
- [ ] P2d：跨集群诊断聚合 + 巡检 + 前端视图 + 演示