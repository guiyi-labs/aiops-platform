# gorm-clause-silent-failure-scan：全仓扫描 GORM 子句静默失效，确认 `signal` 为单点疏漏

- Date: 2026-09-16
- Status: Complete
- Scope: 只做扫描与取证，不改动任何实现代码

## Context

前一提交（`7f2af70`）修掉了 `signal.DeleteExpired` 的一个缺陷：`batchSize` 参数被静默忽略，
清理退化为无界删除。该缺陷是在补测覆盖率时偶然发现的，属于「顺手捡到的」，不是系统性排查的结果。

因此存在一个必须回答的问题：**同类缺陷在仓库里还有多少处？**

这个问题不能靠"看一遍代码"回答。该缺陷的特征是**静默失效**——不报错、不 panic、测试全绿，
只是行为与契约不符。人工审阅对这类缺陷的召回率很低。所以本次做两件事：

1. 把「同类缺陷」的形式定义清楚，写成可执行的扫描器；
2. 扫描器必须先能复现已知的那一处，否则它的"0 命中"没有意义。

## 方法：为什么不用正则，改用 AST

最初的实现是正则：匹配 `.Delete(...)` 之后是否还跟着 `.Limit(`/`.Order(` 等。
扫描 342 个非测试文件报告 0 命中，但**用修复前的文件自测也报 0 命中**——扫描器本身是坏的。

根因：正则 `\.(\w+)\s*\(` 只捕获**紧跟在 `.` 之后**的方法名。而在

```go
result := r.db.WithContext(ctx).
    Where("expires_at IS NOT NULL AND expires_at <= ?", now).
    Delete(&signalRow{}).Limit(batchSize)
```

这条链里，`Delete` 前面是 `Where(...)` 的**右括号 `)` 加换行**，不是 `.`。
于是 `Delete` 从未被匹配到，整个缺陷被漏掉。跨行链式调用用正则天然不可靠。

改为用 Go 官方 `go/parser` 解析成 AST。链式调用在 AST 里是确定的嵌套结构：

```
CallExpr{Fun: SelectorExpr{X: <上一层 CallExpr>, Sel: 方法名}}
```

由外向内遍历即可还原**完整调用顺序**，与方法前面是 `.` 还是 `)` 无关。

扫描器：`/tmp/chainscan/main.go`（临时工具，未入库；已固化为可复用能力，见文末 Notes）。

## 三段扫描

| 段 | 检查内容 | 为什么这是缺陷 |
| --- | --- | --- |
| A | 终结方法（`Delete`/`Update`/`Find`/`Exec` 等）之后还挂子句方法 | 终结方法调用即执行 SQL，其后的子句不会进入已执行的语句 |
| B | 写库方法（`Delete`/`Update`/`Updates`）链上出现 `Limit`/`Offset`/`Order`（顺序无关） | Postgres 方言直接丢弃这些子句，**与链序无关**——所以"把子句前移"修不好 |
| C | 变量中转：`q := ...Limit(10)` 之后 `q.Delete()` | 段 A 的另一种写法，AST 里不在同一条链上 |

段 B 是本次新增的。它来自上一提交的教训：当时我曾判断"把 `Limit` 前移到 `Delete` 之前会语法报错"，
实测证明**既不报错也不生效**。既然顺序无关，那么只检查链序是不够的。

### 有效性自测（先证明扫描器能抓到已知缺陷）

| 输入 | 期望 | 实际 |
| --- | --- | --- |
| `b159c31` 版 `signal/gorm_repository.go`（含已知缺陷） | 命中 1 处 | 命中 1 处（第 204 行 `Delete().Limit()`） |
| `7f2af70` 版（已修） | 命中 0 处 | 命中 0 处 |
| 合成正样本 7 例（A/B/C 三段） | 全中 | 全中，零漏报 |
| 合成负样本 3 组（子句在终结方法之前、非 GORM 同名方法、终结方法是链尾） | 零误报 | 零误报 |

负样本里特别验证了 `r.Count(&n)` 与 `q.Find(&rows)` **不被**段 C 误报——
它们是终结方法但不写库，链上的 `Limit` 正常生效，不算缺陷。

## 取证：GORM 1.31.2 + Postgres 方言实测

段 B 的前提是"方言真的会丢弃"，这个前提必须实测而不是凭印象。
用 sqlmock 的 `QueryMatcherFunc` 捕获 GORM 真正发给驱动的语句（捕获即地面真值）：

| 写法 | 实际发出的语句 | 结论 |
| --- | --- | --- |
| `Limit(10).Delete()` | `DELETE FROM probe_rows WHERE a = $1` | Limit 丢弃 |
| `Delete().Limit(10)` | `DELETE FROM probe_rows WHERE a = $1` | Limit 丢弃 |
| `Order(id).Delete()` | `DELETE FROM probe_rows WHERE a = $1` | Order 丢弃 |
| `Delete().Order(id)` | `DELETE FROM probe_rows WHERE a = $1` | Order 丢弃 |
| `Offset(5).Delete()` | `DELETE FROM probe_rows WHERE a = $1` | Offset 丢弃 |
| `Limit(3).Update(a,1)` | `UPDATE probe_rows SET a=$1 WHERE a=$2` | Limit 丢弃 |
| `Update(a,1).Limit(3)` | `UPDATE probe_rows SET a=$1 WHERE a=$2` | Limit 丢弃 |
| `Limit(3).Updates(map)` | `UPDATE probe_rows SET a=$1 WHERE a=$2` | Limit 丢弃 |
| `Order(id).Updates(map)` | `UPDATE probe_rows SET a=$1 WHERE a=$2` | Order 丢弃 |
| `Limit(10).Find()`（对照） | `SELECT * FROM probe_rows WHERE a = $1 LIMIT $2` | **生效** |
| `Delete()` 无 `Where`（对照） | 未发出语句，`err=WHERE conditions required` | GORM 已有全局删除保护 |

三条结论：

1. **`DELETE`/`UPDATE` 上的 `Limit`/`Offset`/`Order` 一律被静默丢弃，与链序无关。**
   所以有界写操作只能用 CTE + 子查询表达，`metricshistory` 与修复后的 `signal` 就是这个形状。
2. **`SELECT` 上的 `Limit` 正常生效**（对照组），说明子句本身没问题，是写库语句的方言限制。
3. **无 `Where` 的 `Delete` 已被 GORM 拦住**（`ErrMissingWhereClause`），
   "全表误删"这一风险类不需要额外防护。

## 结果

扫描全仓：非测试 `.go` **342 个**、测试 `.go` **280 个**。

| 段 | 非测试 | 测试 |
| --- | --- | --- |
| A 链序违规 | 0 | 0 |
| B 方言丢弃 | 0 | 0 |
| C 变量中转 | 0 | 0 |

**结论：`signal.DeleteExpired` 是单点疏漏，不是系统性模式。**

旁证：同仓库的 `metricshistory.GormRepository.DeleteExpired` 从一开始就是正确的
（`WITH expired AS (SELECT id ... ORDER BY expires_at ASC, id ASC LIMIT ?) DELETE ...`）。
正确范式在项目里已存在，说明这是"复制时漏抄了约束"，而非团队不掌握该写法。

### 顺带补查：手写 SQL 的无界写操作

段 A/B/C 只覆盖 GORM 链式调用，不覆盖手写 SQL。额外排查了全部 9 处手写 `DELETE FROM`
与 15 处手写 `UPDATE ... SET`：

- 8 处 `DELETE FROM` 都有精确 `WHERE`（按 `id`、`user_id`、`incident_id` 等），或有界 CTE；
- `cmd/seed-knowledge` 的两处是 CLI 工具的显式重置命令（按 provenance 标记清理自己写入的行），按设计全量；
- 15 处 `UPDATE` 均有 `WHERE`。

**唯一需要判断的是 `aiexplain.GormRepository.Reserve`（repository.go:50）：**

```go
if err := tx.Exec(`DELETE FROM ai_usage_reservations WHERE expires_at <= NOW()`).Error; err != nil {
```

这里**故意不做批量上限**，判断依据有两条：

1. **没有契约承诺。** `aiexplain.Repository` 接口没有 `batchSize` 参数，不存在"被忽略的约束"。
   这与 `signal` 不同——后者的接口注释明文写了 `Bounded by an internal batch size`。
2. **删除的完整性是承重的。** 紧随其后的预算检查是
   `COALESCE((SELECT SUM(reserved_tokens) FROM ai_usage_reservations), 0)`——**没有 `expires_at` 过滤**。
   它依赖前面的删除已经把过期行清干净。若给这个删除加批量上限，过期行会残留并被计入 `reserved`，
   直接导致**误报预算超限**。所以加上限在这里是**正确性回退**，不是修复。

表上有 `ai_usage_reservations_expires_idx (expires_at)` 索引支撑，删除代价可控。
**判定：不是缺陷，不改。** 记录在此是为了说明"扫到了但有意放过"，避免日后被当成漏网之鱼。

## Verification

- `go build ./...`：通过
- `go vet ./internal/signal/`：通过
- `go test ./internal/signal/ -count=1`：通过
- 探针文件 `zz_probe_dialect_test.go` 用完即删，`git status --short` 为空，工作树干净
- 扫描器自测：已知缺陷必中（1/1）、合成正样本全中（7/7）、合成负样本零误报（0/3）

## Risks / Notes

- **扫描器的局限**：段 A/B/C 只覆盖 GORM 链式调用这一种形式。
  手写 SQL（`Exec`/`Raw`）里的无界写操作不在覆盖范围内，本次是人工排查的。
  其他未覆盖的静默失效类型：`Updates(struct)` 会忽略零值字段、`Save` 在主键为零值时退化为插入、
  `Rows()` 未 `Close()` 造成连接泄漏——本次均未扫描。
- **扫描器未入库**：作为临时工具放在 `/tmp`，仓库工作树保持干净。
  它已固化为可复用能力（见 skill `gorm-clause-silent-failure-scan`），需要时可重新生成。
  若要入库为常驻守卫测试，需要另行决策——新增包会改变课题范围文档中
  已冻结的包数与行数口径，不宜顺手改动。
- **本次零代码改动**，因此不产生新的版本锚点。
