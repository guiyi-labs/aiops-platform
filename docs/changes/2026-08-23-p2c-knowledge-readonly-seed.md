# P2c：RAG 案例库只读端点 + 幂等 seed 脚本 + 演示剧本

- Date: 2026-08-23
- Status: Complete
- Scope: 收尾 P2c 方向——补齐知识库只读查询端点（`GET /api/v1/aiops/knowledge`、`/knowledge/stats`）、幂等的 `seed-knowledge` 灌库命令与毕设演示剧本，并把游离的 P2c 改动按 AGENTS.md 归档入库。

## Context

P1 已完成 RAG 知识库（model / repository / 两段检索 / 诊断结案自动蒸馏 / aiexplain 引用注入）。P2c 的目标是把「可演示的闭环」补齐：

1. 提供一个**只读** HTTP 查询面，便于演示巡检与验证（写入仍只走诊断结案钩子，杜绝经 HTTP 误写）；
2. 提供一条**幂等**的种子脚本，向 `knowledge_entries` 灌入 10 条精心构造的已解决案例，让演示/答辩直接跑通「问题 → 检索 → AI 综合 → 带依据回答」；
3. 补齐演示剧本与截图占位，对齐毕设论文实验章节。

此前工作区已有游离的 `knowledge.go` / `knowledge_test.go` 与 `router.go`/`main.go`/`memory.go`/`openapi.yaml` 改动，但缺 change-record、缺 CHANGELOG 更新、**且打破了权限矩阵契约测试**（`TestPermissionMatrixMatchesCommittedDocument`），属未完成状态。

## What Changed

### 新增知识库只读端点（backend）

- `backend/internal/httpserver/knowledge.go`：新增 `knowledgeHandler`，暴露两个只读路由：
  - `GET /api/v1/aiops/knowledge` —— 按 `rule_id` / `severity` / `min_severity` / `resource_kind` / `limit` 过滤；`severity` 与 `min_severity` 互斥校验；`limit` 钳制到 [1,100]，截断经响应信封披露（与检索器一致）。
  - `GET /api/v1/aiops/knowledge/stats` —— 返回案例库总数（`total`），用于演示巡检。
  - 仓库为 nil 时返回 `503 KNOWLEDGE_UNAVAILABLE`（优雅降级，不泄露内部状态）。
- `backend/internal/httpserver/knowledge_test.go`：路由契约测试（200 / 503 / 400 互斥 / 非法 severity / 非法 limit）。`knowledgeNopRepo` 满足 `knowledge.Repository` 接口（编译期断言）。
- `backend/internal/httpserver/router.go`：当 `Options.KnowledgeRepository != nil` 时注册上述两条路由；`Options` 新增 `KnowledgeRepository` 字段并注明「写只走诊断结案钩子」。
- `backend/cmd/server/main.go`：将既有 `knowledgeRepository` 注入 `Options.KnowledgeRepository`。

### knowledge 仓库增强

- `backend/internal/knowledge/memory.go`：新增 `NopRepository`（no-op 实现），用于路由契约测试与 nil 安全替换；补充 `var _ Repository = NopRepository{}` 编译期接口断言。

### 幂等种子脚本（新增 cmd）

- `backend/cmd/seed-knowledge/catalog.go`：10 条已解决案例目录（crash_loop / oom_killed / image_pull_backoff / pending / node.not_ready / service.no_ready_endpoints / ingress.backend_unavailable / hpa.saturated / pvc.pending / deployment.replicas_unavailable），含 realistic root_causes / recommendations，并错位 `noted_at` 保证确定性排序。
- `backend/cmd/seed-knowledge/main.go`：
  - 先跑迁移门禁（`migrations.Apply`），再 upsert 名为 `seed-knowledge-demo` 的禁用集群（按 name `ON CONFLICT` 复用）；
  - 按 `seed:` 前缀精确清除本工具拥有的诊断行（知识条目经 `source_diagnosis_id` 外键级联）+ 防御性删除孤儿知识条目；
  - 每条案例写入一条 `resolved` 诊断，再经 `knowledge.NewGormRepository.Insert` 蒸馏入库；
  - `-dry-run` 不触库，仅列出将写入条目；`-timeout` 控制总超时。
- `backend/cmd/seed-knowledge/catalog_test.go`：hermetic 目录测试（计数、必填字段、无重复 key、确定性排序、时间错位、marker 非空）。

### API / 权限契约

- `docs/api/openapi.yaml`：新增 `/api/v1/aiops/knowledge`（`KnowledgeEntryList` / `KnowledgeEntry` schema）与 `/api/v1/aiops/knowledge/stats`（`{total}`）。
- `docs/security/permission-matrix.md`：**重新生成**（运行 `TestPermissionMatrixMatchesCommittedDocument -update`），纳入两条新路由的 scope/audit 元数据，修复此前被打破的契约测试。

### 演示文档（毕设私有，不推送）

- `docs/thesis/rag-demo-script.md`：P2c 演示剧本（前置 / 主线 3-4 分钟 / 截图占位 / 排障）。用 mock 本地 stub，不依赖真实 LLM key。

## Verification

- `go test ./internal/httpserver/ ./internal/knowledge/ ./cmd/seed-knowledge/`：**全绿**（含权限矩阵契约测试恢复）。
- `go build ./...` / `go vet ./...`（改动包）：**0 issue**。
- `go run ./cmd/seed-knowledge -dry-run`：列出 10 条将写入案例，未触库。
- 权限矩阵契约：`TestPermissionMatrixMatchesCommittedDocument` 由红转绿；`docs/security/permission-matrix.md` 已含两条 `knowledge` 路由（`aiops.knowledge.list` / `aiops.knowledge.stats.read`）。

## Risks / Notes

- 种子脚本依赖可达 PostgreSQL 且已跑迁移；CI 中不执行（无数据库、无 key），仅 `catalog_test.go` 走 hermetic 逻辑测试。
- 知识库**写入**仍只通过诊断结案钩子（`diagnosis.KnowledgeIngester`），HTTP 面保持只读——符合 ADR 0004「AI 不直接执行变更 / 写入受控」的边界。
- `seed-knowledge` 写入的集群为 `disabled` 状态，仅作 FK 锚定，不进入任何多集群探测/联邦扇出。
