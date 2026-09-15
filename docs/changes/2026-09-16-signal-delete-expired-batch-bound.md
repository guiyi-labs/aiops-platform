# 修复 signal.DeleteExpired 的无界删除：batchSize 被静默忽略

- Date: 2026-09-16
- Status: Complete
- Scope: 修复 `internal/signal` 的 `GormRepository.DeleteExpired`——`batchSize` 参数
  被 GORM 静默丢弃，导致「有界清理」实际退化为**单条无界 DELETE**，
  违背 `Repository` 接口的明文契约。改为 CTE + 子查询，把界写进语句内部。

## Context

### 缺陷的发现路径

在 T1 覆盖率补齐轮（见 `2026-09-15-t1-core-package-coverage.md`）为
`internal/signal` 补测时，子代理报告了这处可疑代码，随后**独立复核确认**。

### 契约说「有界」，实现是「无界」

`internal/signal/repository.go:28-30` 的接口注释：

> `DeleteExpired` removes rows whose `expires_at <= now`. Returns the count
> removed. **Bounded by an internal batch size to avoid long transactions.**

`internal/signal/service.go:23` 的调用侧注释：

> `retentionBatch` **bounds** `DeleteExpired` so cleanup transactions stay short.

而实现（修复前）：

```go
result := r.db.WithContext(ctx).
	Where("expires_at IS NOT NULL AND expires_at <= ?", now).
	Delete(&signalRow{}).Limit(batchSize)
```

`Limit` 在终结方法 `Delete` **之后**调用，此时语句已经执行完毕，所以 `batchSize`
从未进入 SQL。

### 用探针把「怀疑」变成「事实」

怀疑不够，实测才算数。用一个临时探针（sqlmock 自定义 `QueryMatcherFunc` 捕获
GORM 实际发出的语句）比对三种写法：

| 写法 | 实际发出的 SQL |
|---|---|
| A 现状：`Delete(...).Limit(n)` | `DELETE FROM "signal_occurrences" WHERE expires_at IS NOT NULL AND expires_at <= $1` |
| B 天真修法：`Limit(n).Delete(...)` | `DELETE FROM "signal_occurrences" WHERE expires_at IS NOT NULL AND expires_at <= $1` |

**两种写法都没有 `LIMIT`。** 这推翻了一个先前写进文档的错误判断
（原记录称「把 `Limit` 前移会生成 `DELETE ... LIMIT`，Postgres 会语法报错」）。
真相是：**GORM 对 Postgres 方言直接丢弃 `DELETE` 上的 `Limit`**，
所以天真修法不会报错，而是**同样静默失效**——比报错更难发现。

结论：修法不能靠 GORM 的链式 API，只能把界写进语句本身。

### 同仓库已有正确实现

`internal/metricshistory/repository.go:170` 的同类方法**写对了**：

```go
result := r.db.WithContext(ctx).Exec(`WITH expired AS (
    SELECT id FROM metric_collection_runs WHERE expires_at < ? ORDER BY expires_at ASC, id ASC LIMIT ?
) DELETE FROM metric_collection_runs WHERE id IN (SELECT id FROM expired)`, before, limit)
```

即：本项目内部已有可照抄的范式。这也说明该缺陷是**单点疏漏**，不是全局设计问题。

## What Changed

### `backend/internal/signal/gorm_repository.go`

- `DeleteExpired` 改为 CTE + 子查询形式，沿用 `metricshistory` 的既有范式：

  ```sql
  WITH expired AS (
      SELECT id FROM signal_occurrences
      WHERE expires_at IS NOT NULL AND expires_at <= ?
      ORDER BY expires_at ASC, id ASC
      LIMIT ?
  ) DELETE FROM signal_occurrences WHERE id IN (SELECT id FROM expired)
  ```

- 保留原有语义：`batchSize <= 0` 时回落到 500 默认值；`expires_at` 为 NULL 的行不删。
- `ORDER BY expires_at ASC, id ASC` 让每批的选取**确定性**（否则「取 500 条」取哪 500 条
  由物理顺序决定，重跑结果不可预期）。
- 附带效果：改为单条 `Exec` 后**不再需要 GORM 的隐式事务**，少一次 `BEGIN`/`COMMIT` 往返。
  单语句 DELETE 本身就是原子的。
- 补注释说明**为什么不能用 `Delete(...).Limit(n)`**——这是防止该缺陷复发的第一道防线。

### `backend/internal/signal/gorm_repository_test.go`

- 三个既有用例原先**写成了匹配缺陷行为**（`WithArgs(now)` 只断言一个参数，
  没有 batch 参数；并期待 `ExpectBegin`/`ExpectCommit`）——等于把 bug 锁死。
  已改为断言新的有界语句与两个参数。
- 新增 `TestGormRepositoryDeleteExpiredBoundsBatchInSQL`：用非默认 `batchSize = 7`
  断言它**确实到达 SQL**。这是本缺陷的回归测试——修复前该用例必然失败。

## Verification

- `go test ./internal/signal/ -count=1 -cover` → `96.7%`，0 失败。
- 全量：`go test -cover -p=1 -count=1 -coverprofile=coverage.out ./...` →
  `72.3%`（与修复前一致），**0 失败**。
- `gofmt -l .` 无输出；`go vet ./internal/signal/` 通过；`go build ./...` 通过。
- 探针文件已删除，未进入版本库。

## Risks / Notes

- **这是行为变更**，不是纯测试改动：`DeleteExpired` 现在最多删 `batchSize` 行，
  而此前会一次删光。调用方 `signal.Service.Cleanup` 需要**多次调用**才能清完积压
  （每次最多 `retentionBatch` 行）。这是接口契约本来的意图，但值得知晓。
- **单点疏漏，非普遍问题**：`metricshistory` 的同类方法一直是对的。
  未做全仓同类扫描——若后续要查，搜索模式是
  `Delete(` 之后链式调用 `Limit(`/`Offset(`/`Order(`/`Select(`。
- **口径提醒**：覆盖率数字与本次修复无关（修复未改变覆盖率），
  仍以 `b159c31` 为覆盖率锚点。
- 该修复位于 `b159c31` **之后**，故锚点 `b159c31` 与 `baseline-coverage-t1-20260915`
  **不包含**本修复；引用覆盖率数字用旧锚点，引用 `signal` 清理逻辑用新锚点。
