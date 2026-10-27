# metric-breach-severity：修正内存类指标越界诊断的非法严重级

- Date: 2026-10-27
- Status: Complete
- Scope: `internal/diagnosis/metric_breach.go` 的严重级取值；同步修正单测断言

## Context

告警规则评估在内存类指标越界时反复失败，每 30–45 s 刷一条：

```
ERROR alert/scheduler.go:90 alert rule evaluation failed
  {"rule_id": 4, "error": "insert alert diagnosis: ERROR: new row for relation
   \"diagnosis_records\" violates check constraint \"diagnosis_records_severity_check\" (SQLSTATE 23514)"}
```

根因：`metricBreachSeverity()` 对 `memory` 返回 **`"medium"`**，而 `diagnosis_records` 的约束为
`severity IN ('info','warning','high','critical')`（`000004_diagnosis_records.up.sql`）。
`medium` 不是 diagnosis 领域的合法严重级，插入直接被拒。

两个放大因素：

1. **单测把错误值固定成了预期**：`metric_breach_test.go` 断言 `record.Severity == "medium"`，
   因此该缺陷在 CI 中永远是绿的——测试只校验结构体字段，不经过数据库约束。
2. CPU 分支返回 `high`（合法），所以**只有内存类指标**受影响，表现为"CPU 告警正常、内存告警空转"，
   极易被误判为"内存阈值配置不当"。

## What Changed

### diagnosis
- `internal/diagnosis/metric_breach.go`：`memory` 分支 `"medium"` → `"high"`；`default` 分支 `"medium"` → `"warning"`，
  保证返回值恒在合法枚举内，并加注释说明约束来源。
- `internal/diagnosis/metric_breach_test.go`：断言由 `"medium"` 改为 `"high"`，
  并在失败信息中保留实际值以便定位。

## Verification

- `go test ./internal/diagnosis/ -count=1`：`ok`（0.62 s）；
- 重建后端并重启后，规则 #4（node02 内存 ≥1.7 GiB）状态由空 → `firing`，
  产出诊断 **ID 85** `node.metric_sustained_breach.v1`（severity `high`，证据 2 条）；
- 重启后 120 s 内 `severity_check` 相关错误日志计数 **0**（此前每 30 s 一条）。

## Risks / Notes

- 内存越界由 `"medium"` 提升为 `"high"` 属**严重级上调**，会改变排序与 SLA 计算口径；
  若业务上认为内存越界应低于 CPU，可改为 `"warning"`（同样是合法值），不应恢复 `"medium"`。
- 更根本的改进方向：严重级应改为**枚举类型 + 单一来源常量**（如 `diagnosis.Severity` 与数据库 CHECK 从同一处生成），
  避免同类漂移再次发生；当前代码里 `severity` 仍是裸字符串。
- 附带发现：插入失败会消耗主键序列（本次空号 35–84），不影响数据正确性，但会让 ID 看起来"跳号"。
