# alert-rows-err-check：三处 `rows.Next()` 循环漏检 `rows.Err()`，部分结果以 nil 错误返回

- Date: 2026-09-16
- Status: Complete
- Scope: 修复 `internal/alert` 的三处游标循环；新增 `gorm_repository_rows_test.go` 回归用例

## Context

承接同日两轮 GORM 缺陷排查（`7f2af70` 修 `signal.DeleteExpired` 的无界删除、
`f33742e` 做全仓同类扫描）。那两轮覆盖的是「GORM 链式调用子句静默失效」。
当时在变更记录里明确列了**三项未扫描的静默失效类型**：

1. `Updates(struct)` 忽略零值字段
2. `Save` 在主键为零时退化为 INSERT
3. `Rows()` 未 `Close()` 造成连接泄漏

本次把这三项补上，并在第 3 项旁边发现了一个**更近的同类问题**：`Rows()` 确实都 `Close()` 了，
但循环结束后**没有检查 `rows.Err()`**。

## 三项排查结果

| 项 | 结论 |
| --- | --- |
| `Updates(struct)` 忽略零值字段 | **0 命中**。全仓 37 处 `Updates` 全部传 `map[string]any` 字面量，或从**指针字段**构建的 map（`if patch.Enabled != nil { updates["enabled"] = *patch.Enabled }`）——正是规避零值陷阱的正确写法 |
| `Save` 主键为零时退化为 INSERT | **0 命中**。4 处 `Save` 调用点：3 处是"先查后存"（主键必非零），1 处 `appcatalog.CreateRepository` 是显式新建（故意不带 ID 走 INSERT）且处理了唯一约束冲突 |
| `Rows()` 未 `Close()` | **0 命中**。3 处 `Rows()` 都有 `defer rows.Close()` |
| `rows.Err()` 漏检 | **3 处命中**（见下） |

## 缺陷：`rows.Next()` 返回 false 有两种含义

`for rows.Next()` 退出循环时，`false` 可能是「读完了」，也可能是「中途出错」——
连接中断、`ctx` 取消、`scan` 失败。**只有 `rows.Err()` 能区分这两者。**

三处循环都以 `return rules/instances, nil` 结束，把后者当成前者：

| 位置 | 函数 | 影响 |
| --- | --- | --- |
| `internal/alert/repository.go:113` | `ListRules` | 列表接口以 200 返回**截断的列表**，调用方无法察觉 |
| `internal/alert/repository.go:278` | `ListInstances` | 同上 |
| `internal/alert/repository.go:335` | `ClaimDueRules` | 最严重：行已在上一句 `UPDATE ... RETURNING` 里被 claim 掉，静默返回部分结果会让未返回的那部分一直被 claim 到租约过期才被重新拾起——表现为**漏评一轮且无任何错误信号** |

### 为什么这类缺陷只能靠"注入失败"来锁

它不报错、不 panic、无日志。常规信号（编译、测试、lint）全部指向"正常"。
唯一可靠的验证方式是：**注入一个迭代中途的失败，断言它必须冒出来**。

## 修复

三处各加一段检查（共 +15 行，含注释）：

```go
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
```

行为变更：出错时不再返回部分结果，而是返回 `(nil, err)`。

## Verification

### 回归用例在修复前必然失败（关键验证）

新增 `gorm_repository_rows_test.go`（221 行 / 5 个测试函数）：3 个"必须报错"用例
+ 2 个阳性对照。`git stash` 回退修复后重跑：

```
--- FAIL: TestGormRepositoryListRulesSurfacesMidIterationError
    err = <nil>, want injected mid-iteration failure（部分结果不能以 nil 错误返回）
--- FAIL: TestGormRepositoryListInstancesSurfacesMidIterationError
    err = <nil>, want injected mid-iteration failure
--- FAIL: TestGormRepositoryClaimDueRulesSurfacesMidIterationError
    err = <nil>, want injected mid-iteration failure
```

修复后 5/5 通过。

**阳性对照不能省**：若只写"必须报错"的用例，fixture 本身扫不过去也会让用例通过（假绿）。
所以每处都配了一个"干净数据必须成功返回 1 条"的对照，并用 `errors.Is(err, injected)`
而非 `err != nil` 断言——确保报的是**注入的那个错**，不是扫描失败。

### 探针踩坑：`RowError` 的索引语义

最初用「1 行 + `RowError(1, err)`」注入，结果错误不触发。查 sqlmock 源码发现
`driver.Rows.Next` 只在**交付某一行时**才返回 `nextErr[pos-1]`，行用完后一律返回 `io.EOF`
而不再查 `nextErr`。所以必须给 **2 行** + `RowError(1, ...)`：
第 2 次读取交付第 1 行时返回错误，循环体正好执行一次——这才复现出"收了一行之后才失败"。

### 全量验证

- `gofmt -l .`：无输出
- `go vet ./internal/alert/`：通过
- `go build ./...`：通过
- `go test -cover -p=1 -count=1 ./...`：**0 失败**
- 全局语句覆盖率 **72.3% → 72.5%**
- `alert` 包覆盖率 **33.9% → 52.1%**（修复前基线为临时移开新用例后实测）

## Risks / Notes

- **这是行为变更**：出错时不再返回部分结果。对 `ListRules` / `ListInstances` 而言，
  调用方现在会看到错误而不是截断的列表——这是正确方向，但属于可观测行为变化。
- **`alert` 不在 T1 七包内**，属 T2 依赖闭包，故 T1 覆盖率不受影响；
  但 `T2` 八包行数由 11,679 变为 11,694，锚定表已相应追加锚点 5。
- **未覆盖**：`sql.Row`（单行）不需要 `Err()`，本仓 19 处 `.Row().Scan()` 均直接返回错误，无此问题。
