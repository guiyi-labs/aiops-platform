# P1 RAG 知识库诊断 — 落地

- Date: 2026-08-16
- Status: Complete
- Scope: 按 docs/enhancement-p1-rag-diagnosis-plan.md 方案实施
- DependsOn: operator 增强（af670de 等，M115）

## What

### 1. 知识库基础（backend/internal/knowledge/）

- `model.go`：Entry / Filter / ListResponse / Repository 接口 + Severity 词汇表
- `repository.go`：GormRepository（GORM raw SQL）。Insert 带循环缺陷去重
  （rule + resource + 首条根因命中则更新最新）；ListByFilter 支持规则/
  严重级/资源类型过滤按 noted_at 倒序；Count。
- `retriever.go`：两阶段检索 Retriever——阶段1 结构化 B-tree 短名单
  （默认 Top-10，>= high 严重级），阶段2 可选 LLM 精排（Reranker 接
  口，失败静默回退短名单）；BuildPromptContext 生成 prompt 注入文本。
- `ingest.go`：DiagnosisIngester —— 把 diagnosis.KnowledgeEntryInput 转成
  knowledge.Entry 持久化（knowledge→diagnosis 单向依赖）。
- `memory_test.go`/`repository_test.go`/`retriever_test.go`/`gorm_test.go`
  （sqlmock 覆盖 GORM 分支）/`ingest_test.go`：全分支测试。

### 2. 诊断自动入库钩子（backend/internal/diagnosis/）

- `ingest.go`：KnowledgeIngester 窄接口（diagnosis 侧定义，不依赖
  knowledge 包）+ IngestResolvedIfEligible（仅 resolved 且有内容时写入，
  错误吞掉不阻断）+ truncateSummary。
- `service.go`：Service.WithKnowledgeIngester + Transition 到 resolved 后
  调 ingest（nil 安全，主链不变）。

### 3. aiexplain RAG 集成（backend/internal/aiexplain/）

- `prompt.go`：新增 BuildPromptWithHistory —— 历史案例注入 prompt 前导段
  并注册 historical:N 为可被引用证据（走现有 citation 校验防伪造）。
- `service.go`：WithKnowledgeRetriever + Generate 检索历史案例注入
  （知识库故障静默降级为原生 prompt）。
- `prompt_history_test.go`：注入/空降级/citation 校验接受与拒绝。

### 4. aiinvestigator 历史参考（backend/internal/aiinvestigator/）

- `model.go`：EvidenceKindHistoricalCase + HistoricalCaseContext。
- `prompt.go`：CaseContext.HistoricalCases 渲染为「历史参考」段——
  不进 authorized evidence / PromptHash，investigation_key 稳定。
- `historical_test.go`：渲染/不进证据集/哈希不变。

### 5. 迁移

- `backend/migrations/000050_knowledge_entries.{up,down}.sql`：
  knowledge_entries 表 + 检索索引 + 去重唯一索引。

### 6. main 接线 + 前端

- `cmd/server/main.go`：knowledge repo/retriever/ingester 构造并注入
  diagnosis + aiexplain（AI 启用时生效；精排默认关，成本为零）。
- `frontend/src/views/DiagnosesView.vue`：evidenceLabel 识别 historical:
  前缀显示「历史案例」。

## Verification

- `go test ./...` 全绿；`go test -race -p=1` 关键包全绿。
- 全局覆盖率（CI 同款命令）**70.08% ≥ 70% 门**。
- golangci-lint（同 CI 配置）**0 issues**；gofmt 空。
- sqlmock 覆盖 GORM 分支；RAG 远端降级路径（Retrieve 失败→原生 prompt）
  已测试。
- 验证边界：LLM 精排默认关闭（feature flag 结构预留），CI 无真实 LLM
  调用；集成真实 API 属后续可选。

## Risks / Notes

- knowledge_entries 去重键为 (rule_id, resource_kind, resource_name,
  root_causes[0])——同缺陷最新胜出。
- Phase 2（pgvector>1000 条 / LLM 精排启用）为方案预留扩展点，本期未启用。
