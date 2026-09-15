# T1 核心七包覆盖率补齐：从「结构上测不到」到全部 ≥75%

- Date: 2026-09-15
- Status: Complete
- Scope: 为课题详写的 7 个核心包（`diagnosis` / `correlation` / `signal` / `finding` /
  `knowledge` / `aiexplain` / `mcpserver`）补写单元测试，使**每个包**的语句覆盖率
  不低于 CI 的 75% 门禁线；全局语句覆盖率随之从 **70.1%** 升至 **72.3%**。
  本次**只新增 `_test.go` 文件**，未改动任何业务源码。

## Context

### 问题：核心包的覆盖率低于外围包

课题的文档边界把 7 个包划为 **T1「详写」**，其余 8 包为 T2「简述」、56 包为 T3「不写」。
但覆盖率剖面与这个声明是**反着**的：

| 包 | 层级 | 补测前 |
|---|---|---|
| `aiexplain` | **T1** | 52.9% |
| `diagnosis` | **T1** | 64.8% |
| `signal` | **T1** | 70.1% |
| `correlation` | **T1** | 70.7% |
| `monitoring` | T3（不写） | 98.0% |
| `eventstream` | T3（不写） | 94.4% |

即「核心模块」的测试强度低于「不写的模块」。这不只是数字难看：正文第 3、4 章
以这 7 个包为论证主体，终期评审时「核心模块为什么覆盖率反而更低」是可直接追问的点。

### 根因：repository 层是结构性空白

全仓未覆盖语句 **8,439 条**，其中 **repository 层占 3,291 条（39%）**，
覆盖率仅 **5.1%**。原因是全仓测试**不使用真实数据库**——没有 sqlite、没有
testcontainers、没有 pgx；而 repository 又**不能**用内存库替代，因为它们发的是
Postgres 专有 SQL：

- `date_trunc('day', NOW() AT TIME ZONE 'UTC')`（`aiexplain/repository.go`）
- `pg_advisory_xact_lock(?)`（同上，`Reserve` 的串行化点）
- `CAST(? AS JSONB)`、`FILTER (WHERE ...)`、`RETURNING id`

（`gorm.io/driver/sqlite` 虽在 `go.sum` 中，但对这些语句无解，故不采用。）
既有可用手段只有一个：`github.com/DATA-DOG/go-sqlmock` + GORM 的
`gormpostgres.New(gormpostgres.Config{Conn: db})`——仓库里已有此范式，
但**此前只有 `internal/knowledge` 和 `internal/store` 两处**在用。

## What Changed

### `backend/internal/diagnosis`（64.8% → 95.2%）

- 新增 `repository_test.go`（1,124 行）：用 sqlmock 驱动 `Save` / `List` / `list` /
  `Get` / `loadWorkflow` / `Transition` / `Assign` / `AddFeedback` / `Summary` /
  `ListByClusters` / `StatsByClusters`，含 `decodeStoredRecord` 纯函数直测；
  事务方法覆盖 `Begin/Commit` 与 `Begin/Rollback` 两条路径。
- 新增 `service_orchestration_test.go`（444 行）：以假 `Repository` 驱动
  `DiagnoseNode` / `DiagnosePersistentVolumeClaim` / `DiagnoseHorizontalPodAutoscaler` /
  `DiagnoseIngress` / `DiagnoseDeployment`，并断言 `WithKnowledgeIngester` 注入的
  ingester **确实被调用**。
- 新增 `hpa_saturated_test.go`：直测 `summarizeHPAMetric`。

### `backend/internal/correlation`（70.7% → 98.1%）

- 新增 `repository_gorm_test.go`（1,356 行）：`repository.go` 达 **100%**。
  纯函数部分（4 个 `TableName`、JSONB 自定义类型的 `Value`/`Scan`/`MarshalJSON`/
  `UnmarshalJSON`、`applyCaseFilter`、8 个领域对象↔行转换函数、
  `unmarshalEvidenceRefs`/`unmarshalInt64s`/`unmarshalStrings`）直接调用测试。
- 新增 `service_extra_test.go`（655 行）：覆盖 `WithLookback`、`CorrelateNamespace`、
  `ListCases`、`ListTimeline`、`GetCaseGraph` 与 `worker.go` 的 `NewWorker`/`runScope`。
- `UpsertResult` 的合并语义用自定义 `sqlmock.Argument` **捕获落库参数**，
  断言因子合并、置信度提升、观测窗双向扩展——而非只校验「调用发生了」。

### `backend/internal/signal`（70.1% → 96.7%）

- 新增 `gorm_repository_test.go`（762 行）：覆盖 `TableName`、`NewGormRepository`、
  `NopRepository` 五个方法、`GormRepository` 的 `Upsert`/`Get`/`List`/
  `CountBySignal`/`DeleteExpired`，以及 `occurrenceToRow`/`rowToOccurrence`。
- 断言覆盖：返回字段映射、`limit` 钳位、`ErrRecordNotFound` → `ErrSignalNotFound`
  的翻译、各类 DB / JSON 错误向上透传。

### `backend/internal/aiexplain`（52.9% → 98.4%）

- 新增 `repository_test.go`、`service_extra_test.go`、`provider_extra_test.go`。
- `Reserve` 的预算控制被完整覆盖：`900+200+100 > 1000` → `ErrBudgetExceeded`
  且**断言事务回滚**；`dailyBudget == 0` 时跳过预算校验。
- 纯函数 `feedbackResult` / `feedbackSummary` / `helpfulRate` / `nullableID` 直测。

### 测试基础设施

- 三个包各自引入 `sqlmock.QueryMatcherFunc` 片段匹配器（空白归一化后做子串匹配），
  替代 `regexp.QuoteMeta` 整条 SQL 钉死。**动机**：断言应聚焦行为，
  钉死整条语句会让测试在无关的 SQL 格式调整中无谓失败。
- 统一 `mustExpectationsMet(t, mock)` 辅助函数，确保期望被消费完。

## Verification

- `go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...`
  → `total: (statements) 72.3%`（补测前 70.1%），**0 失败**；87 包中 81 包 `ok`，
  6 包无测试文件（5 个 `cmd/*` + `internal/buildinfo`）。
- 逐包复核（CI 口径 `go test ./internal/<pkg>/ -count=1 -cover`）：

  | 包 | 补测前 | 补测后 |
  |---|---|---|
  | `aiexplain` | 52.9% | **98.4%** |
  | `diagnosis` | 64.8% | **95.2%** |
  | `signal` | 70.1% | **96.7%** |
  | `correlation` | 70.7% | **98.1%** |
  | `knowledge` | 92.4% | 92.4%（未改动） |
  | `mcpserver` | 97.3% | 97.3%（未改动） |
  | `finding` | 97.6% | 97.6%（未改动） |

- CI 五个核心包门禁复核：`metricshistory` 76.2%、`apiquery` 100.0%、
  `deprecatedapi` 93.2%、`optimization` 82.5%、`knowledge` 92.4% —— 全部 ≥75%。
- `go test -race -count=1` 四个改动包：全部 `ok`。
- `gofmt -l .`：无输出（合规）。`go vet` 四个包：通过。
- `go build ./...`：通过。
- 规模：新增 9 个测试文件、205 个顶层测试函数；全仓测试文件 271 → **280**，
  测试函数 2,388 → **2,593**。

## Risks / Notes

- **未改一行业务源码**：本次全部为新增 `_test.go`。可用
  `git status --porcelain | grep -v '_test\.go$'` 复核为空。
- **残余不可覆盖点**（均为死代码或不可注入路径，未强测以免写出无行为断言的用例）：
  - `correlation/service.go` 中 `s.engine.Correlate` 的错误分支——`Engine.Correlate`
    只有 `return results, nil`，**永不返回 error**，是死代码。
  - `aiexplain/prompt.go` 的 `if valid == nil` 分支——`BuildPrompt` 始终 `make(map…)`。
  - `aiexplain/service.go` 的 `crypto/rand.Read` 错误分支、`provider.go` 的
    `json.Marshal` 失败分支——无依赖注入点，不可注入。
  - `signal/gorm_repository.go` 的 `occurrenceToRow` 内 `json.Marshal` 错误分支——
    `Attributes` 是 `map[string]string`、`Evidence` 仅含 string/int64，编码不可能失败。
- **顺带发现一处既有缺陷（本次未修，仅记录）**：
  `internal/signal/gorm_repository.go:202` 的
  `Delete(&signalRow{}).Limit(batchSize)`——`Limit` 在 `Delete` **之后**调用，
  而 `Delete` 是终结方法、语句已执行，故 **`batchSize` 被静默忽略，删除无上限**。
  注意：把 `Limit` 移到 `Delete` 之前**并不能修好**，因为 Postgres 的 `DELETE`
  不支持 `LIMIT` 子句（那是 MySQL 语法），会直接语法报错。正确修法是子查询形式
  `DELETE ... WHERE id IN (SELECT id ... LIMIT ?)`。**留给后续单独处理**。
- 覆盖率分母是「代码路径」，不是「生产数据分布」——对外引用时须写明测量命令与日期。
