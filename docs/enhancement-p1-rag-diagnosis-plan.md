# P1 RAG 知识库诊断 — 技术方案（落盘待批复）

- Status: Draft（等中枢批复后开工）
- Author: Guiyi Labs
- Date: 2026-08-16
- Scope: `docs/enhancement-p1-rag-diagnosis-plan.md`（不推送）

---

## 1. 背景与动机

现有诊断链路为「确定性规则 → 诊断记录 → AI 解释（aiexplain）/AI 调查
（aiinvestigator）」。每次 AI 调用从零生成——无法利用平台自身已积累的
历史诊断经验。当规则命中一个新问题时，AI 只能基于当前证据推理，无法参考
「同规则在其他集群/时间的已解案例」来提升置信度和推荐质量。

**RAG 的差异化价值**：让诊断系统能够从自身历史诊断记录中检索相似案例，
将「经验证的根因 + 有效处置措施」作为额外上下文注入 AI，形成「问题 →
检索知识库 → LLM 综合 → 带历史依据的回答」闭环。这是基于使用量正向飞轮
的差异化能力：越多历史记录，诊断越准确。

**现有基础设施约束**：
- provider 层：纯 HTTP 调用 OpenAI-compatible `/responses` 端点，无 SDK
- 无 embedding/向量存储能力（go.mod 零相关依赖）
- PostgreSQL 为主要持久化（aiexplain 的 explanation 表 / diagnosis 的 record 表）
- CI 门禁：全局覆盖率 ≥70%，race detector 通过，golangci-lint v2.12.2
- 功能冻结例外：P1 被中枢授权为下一阶段核心差异点

## 2. 知识库来源

**不新建数据源，用既有诊断记录构建知识库**——这是最少耦合、最高 ROI
的路径。

| 知识库内容 | 来源 | 规模预估 | 用途 |
|---|---|---|---|
| 已完成的诊断记录 | `diagnosis.Record`（RootCauses + Recommendations + Evidence + Summary） | 随系统运行增长，每条含结构化根因/建议 | 相似案例检索（主） |
| 已完成的 investigation | `aiinvestigator.Investigation`（Hypotheses + RecommendedRunbookID + Citations） | 与 diagnosis 1:1 | 验证过的处置建议检索（辅） |
| 规则定义 | `diagnosis.Rules`（crash_loop / deployment_replicas / hpa_saturated 等） | 固定集，~10-20 条 | 规则语义描述索引（辅） |
| AI 生成的解释 | `aiexplain.Explanation`（Analysis + RecommendedActions + Feedback） | 与 diagnosis 1:1 | 质量反馈信号（长期） |

**边界**：不包含用户对话、聊天历史或非诊断类数据。知识库仅限平台自身
产生的结构化诊断输出。

## 3. 技术方案

### 3.1 核心思路：结构化检索 + LLM 上下文注入（不引入向量数据库）

考虑到：
- 平台初期诊断记录量有限（<1000 条），向量检索优势不明显
- 运维场景规则性强（rule_id + severity + resource_kind 已构成高区分度索引）
- 避免引入 pgvector / FAISS / 外部向量服务的运维负担

**采用结构化检索（PostgreSQL 原生）+ LLM 二次精排的两阶段方案**：
1. 第一阶段：用 rule_id + severity + resource_kind 从 diagnosis 表检索
   Top-K 候选（K=10，PostgreSQL B-tree 索引，延迟 <5ms）
2. 第二阶段：将候选列表摘要输入 LLM，由 LLM 从候选中挑选最相关的
   N 条（N=3），并生成「检索到的历史案例」上下文段落

这比纯向量检索更适合运维场景：**规则命中本身就是强信号**，不需要
语义相似度兜底。未来若诊断记录量级达到数万、或需要跨规则相似度检索，
可平滑升级为 pgvector 方案（PostgreSQL 原生扩展，不改架构）。

### 3.2 模块设计

#### 新增包：`backend/internal/knowledge`

```
knowledge/
├── model.go          # KnowledgeEntry / SearchResult /检索结果类型
├── repository.go     # PostgreSQL 存储（CRUD + B-tree 检索）
├── retriever.go      # 两阶段检索逻辑（结构化查询 + LLM 精排）
├── retriever_test.go # 纯单元测试（mock DB + mock provider）
├── service.go        # 暴露给 aiexplain/aiinvestigator 的检索接口
├── service_test.go
├── provider.go       # Embedding provider 接口（预留，Phase 2）
└── provider_test.go
```

**核心接口**：

```go
// Retriever 查找与当前诊断上下文相似的历史案例。
type Retriever interface {
    Retrieve(ctx context.Context, query Query, limit int) ([]KnowledgeEntry, error)
}

// Query 是检索查询——基于规则 + 资源特征。
type Query struct {
    RuleID       string
    Severity     string
    ResourceKind string
    SummaryHint  string  // 可选：当前诊断摘要（用于 LLM 精排）
}

// KnowledgeEntry 是一条知识库条目（来源于 diagnosis record）。
type KnowledgeEntry struct {
    ID             int64
    RuleID         string
    Severity       string
    ResourceKind   string
    RootCauses     []string
    Recommendations []string
    Summary        string
    ResolvedAt     *time.Time
    Score          float64  // 检索得分（0-1）
}
```

#### 知识库写入：诊断完成时自动入库

在 `diagnosis` 包的 StateTransition（resolved → done）路径中，若诊断记录
包含非空 RootCauses/Recommendations，自动写入 `knowledge.Entry` 表。

写入逻辑：
- 触发时机：`diagnosis.Record.Status` 从 active/confirmed → resolved
- 写入内容：RuleID + Severity + ResourceKind + RootCauses + Recommendations +
  Summary（截取前 200 字）
- 去重策略：相同 (RuleID, ResourceKind, ResourceName, RootCauses[0]) 的
  记录只保留最新一条（防止重复注入）
- 不影响现有诊断写入路径（新表，非侵入）

### 3.3 与现有 AI 模块的集成点

#### 集成点 1：aiexplain（解释增强）

当前：diagnosis → aiexplain.Prompt（system + input）→ LLM → Explanation
RAG：diagnosis → knowledge.Retriever.Retrieve → **将 Top-3 历史案例注入
Prompt 的 input 段落** → LLM → Explanation（含历史依据引用）

具体改动：
- `aiexplain/service.go` Generate 方法开头：先调用 retriever.Retrieve
- 将检索结果格式化为 Prompt 输入段落：
  ```
  ## 历史相似案例
  [1] RuleID=crash_loop_backoff | Severity=high | RootCause=image_pull_backoff |
      Recommendation=检查镜像仓库凭据 + Resolved 2h ago
  [2] RuleID=crash_loop_backoff | Severity=high | RootCause=OOMKilled |
      Recommendation=调大 limits.memory + Resolved 1d ago
  ```
- aiexplain.Citation 新增 `Source: "historical_case"` 类型（与现有
  `source_type: "diagnosis"` 并列）
- Prompt 注入位置：input 段落的「诊断上下文」之后、「请分析」之前

#### 集成点 2：aiinvestigator（调查增强）

同理：case → knowledge.Retriever → Top-3 历史案例注入 Investigator prompt
的 EvidenceRefs。检索结果作为 `EvidenceKindHistoricalCase` 新证据类型
（与现有 signal_occurrence/topology_edge 等并列）。

#### 集成点 3：前端呈现（只读）

- aiexplain 的 Explanation.Citations 新增 `historical_case` 来源标签
- 前端 FindingEvidencePanel / IncidentCockpit 展示「历史相似案例」
  链接（只读深链到历史诊断记录详情页）
- 不需要新的前端组件，复用现有 Citation 渲染逻辑

### 3.4 检索策略细节

**Phase 1（结构化检索，本期）**：

```sql
-- 两阶段检索：先用结构化索引缩小范围，再用 LLM 精排
-- 第一阶段：B-tree 索引，<5ms
SELECT * FROM knowledge_entries
WHERE rule_id = $1
  AND severity >= $2  -- 严重级别不低于当前（防止低级噪音）
  AND resource_kind = $3
  AND resolved_at IS NOT NULL
ORDER BY resolved_at DESC
LIMIT 10;

-- 第二阶段：LLM 从 10 条中选 3 条最相关的
-- （输入 10 条摘要 + 当前诊断上下文，输出排序后的 Top-3 + 理由）
```

**Phase 2（向量检索，远期，当记录量 >1000 时评估）**：

- 在 knowledge_entries 表加 `embedding vector(1536)` 列
- 用 OpenAI `text-embedding-3-small` 生成嵌入（延迟 <200ms）
- pgvector HNSW 索引 + cosine similarity 检索
- 检索结果与 Phase 1 结构化结果融合（RRF 或加权分数）

### 3.5 测试策略与覆盖率影响

| 测试类型 | 覆盖内容 | 文件 |
|---|---|---|
| 单元测试（mock provider + mock DB） | retriever 两阶段逻辑、去重策略、空结果降级 | `retriever_test.go` |
| 单元测试（mock DB） | repository 写入/查询/去重 | `repository_test.go` |
| 集成测试（真实 DB，CI skip） | 写入→检索→LLM 精排完整链路 | `integration_test.go` |
| aiexplain 改动测试 | Prompt 注入后结构正确、Citation 类型合法 | `aiexplain/prompt_test.go` 增量 |
| 覆盖率影响 | 新增包 ~300 行，预计贡献 +1-2%（已过 70% 门禁，余量充足） | — |

**降级策略**：知识库不可用（DB 错误/检索超时）→ 静默降级，退化为现有
aiexplain 行为（从零生成），不阻断诊断链路。

## 4. Commit 划分（预计 5-6 个提交）

| # | Commit | 内容 | 预估行数 |
|---|---|---|---|
| 1 | `feat(knowledge): add knowledge entry model and repository` | model.go + repository.go + repository_test.go + 迁移脚本 | ~200 |
| 2 | `feat(knowledge): add structured retriever with LLM rerank` | retriever.go + retriever_test.go + service.go + service_test.go | ~300 |
| 3 | `feat(knowledge): auto-ingest on diagnosis resolution` | diagnosis 状态转换钩子 + 去重逻辑 | ~100 |
| 4 | `feat(aiexplain): inject historical cases into prompt` | aiexplain service 改动 + prompt 模板更新 + 增量测试 | ~150 |
| 5 | `feat(aiinvestigator): historical case evidence kind` | aiinvestigator 改动 + EvidenceKindHistoricalCase | ~100 |
| 6 | `feat(frontend): display historical case citations` | 前端 Citation 渲染增强（只读深链） | ~100 |

每次 commit 都独立可测试、不破坏现有链路，CI 全绿。

## 5. 验收标准

### 5.1 功能验收

- [ ] **给定一个新诊断（rule_id=crash_loop_backoff），检索能返回同规则
  的历史已解决案例**
- [ ] **检索结果注入 aiexplain prompt 后，生成的 Explanation.Citations
  包含 `source: "historical_case"` 类型引用**
- [ ] **无历史案例时，aiexplain 正常降级为现有行为（不报错）**
- [ ] **诊断完成（resolved）后，知识库自动写入新条目**
- [ ] **前端展示历史案例引用链接（只读，点击跳转到历史诊断详情页）**

### 5.2 质量验收

- [ ] `go test ./...` 全绿，覆盖率 ≥70%
- [ ] `go test -race` 无 data race
- [ ] golangci-lint（同 CI 配置）0 issues
- [ ] 知识库不可用时静默降级（注入测试：mock DB 返回 error → aiexplain
  正常生成 → Citation 不含 historical_case）

### 5.3 验证边界标注

- **LLM 调用是真实的**：复用现有 `aiexplain.ResponsesProvider`（HTTP 调用
  OpenAI-compatible 端点）。CI 中 LLM 精排部分可 mock（retriever_test
  用 mock provider），集成测试用真实 API（需 API key，CI 环境可选启用）。
- **embedding 不在本期**：Phase 1 纯结构化检索，不引入 embedding 模型调用。
- **数据规模**：验收基于模拟数据（10 条知识库条目），不做大规模性能测试。

## 6. 预计工作量

| 阶段 | 工作量 | 说明 |
|---|---|---|
| P1-1 知识库基础（model/repository/retriever） | 2-3 个 commit | 核心检索能力 |
| P1-2 诊断写入钩子 + aiexplain 集成 | 2 个 commit | 串联完整链路 |
| P1-3 aiinvestigator 集成 + 前端展示 | 1-2 个 commit | 补全双模块 |
| 测试/修复/CI 通过 | 1-2 个 commit | 覆盖率/lint/flaky |
| **合计** | **~6-8 个 commit** | 每个 commit 独立可测 |

## 7. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| LLM 精排成本（每次诊断 +1 次 API 调用） | token 预算增加 | 只对 severity≥high 的诊断启用精排；低级诊断跳过 |
| 知识库冷启动（初期无历史记录） | 检索为空，降级为现有行为 | 不影响；随系统运行自然积累 |
| 检索质量（rule_id 相同但根因不同） | 误导 AI | Phase 1 用 severity+resource_kind 交叉过滤；Phase 2 用向量语义兜底 |
| 与 aiexplain/aiinvestigator 的耦合 | 改动影响面 | 独立包 + 接口注入（不硬编码），可独立开关 |

## 8. 与「复用 XHZhishu」的关系

XHZhishu 项目有知识图谱/RAG 技术栈。P1 方案**不直接复用 XHZhishu 的
基础设施**（避免外部依赖），而是从零构建与之理念一致的轻量知识检索能力。
若后续 XHZhishu 需要与 aiops 集成，`knowledge.Retriever` 接口可扩展为
适配器模式对接 XHZhishu 的向量检索后端——这是 Phase 2 的扩展点，不阻塞
当前交付。
