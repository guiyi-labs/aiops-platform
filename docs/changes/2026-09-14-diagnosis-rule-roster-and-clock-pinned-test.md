# 诊断规则发现契约补齐 12 条 + 修复 inspection 覆盖率用例的时钟依赖

- Date: 2026-09-14
- Status: Complete
- Scope: ① `diagnosis.RuleIDs()`（analyzer discovery contract）补齐漏报的第 12 条规则并加守护测试；
  ② 修复 `internal/inspection` 一个随墙钟时间自我过期的覆盖率用例。

## Context

### ① 发现契约少报一条规则

`backend/internal/diagnosis/model.go` 的 `RuleIDs()` 自称是 analyzer discovery contract，
但只返回 **11** 条：`metric_breach.go:10` 声明的 `RuleNodeSustainedMetricBreach`
（`node.metric_sustained_breach.v1`）因与其实现在同一文件而未被纳入列表。

而该规则是**真实接线并可被引擎发出**的：

- `EvaluateSustainedMetricBreach` 被 `internal/diagnosis/service.go` 与 `internal/alert/service.go` 调用；
- `internal/golden` 的 M82 analyzer-discovery 契约通过 `backend/cmd/server/golden_contracts.go`
  直接消费 `RuleIDs()`；
- `backend/cmd/aiopsbench` 的标注语料 `testdata/diagnosis-corpus.json` 覆盖 **12** 条规则，
  `.artifacts/bench-diagnosis.json` 的 `per_rule` 也是 **12** 条。

即：任何顺着该契约清点规则数的人会得到 11，与语料/报告/文档的 12 不符——契约与实现漂移。

### ② inspection 覆盖率用例随日期过期

`internal/inspection/service_test.go` 的 `TestServiceCoverage_AggregatesWindow` 把 fixture
时间固定为 `2026-08-14`，但 `Service.Coverage` 内部用 `s.now()`（默认 `time.Now`）计算
30 天窗口。固定时间点一旦离当前时间超过 30 天，全部 fixture 行就落到窗口之外，
断言 `TaskTotal == 3` 必然失败——该用例已在本机稳定复现失败（HEAD 上同样失败，
`git stash` 对照确认与本次其它改动无关）。

## What Changed

### `backend/internal/diagnosis/model.go`

- `RuleIDs()` 纳入 `RuleNodeSustainedMetricBreach`，返回 **12** 条（按 node 家族就近插入）。
- 补充注释：说明该列表是引擎可发出规则的权威名册，必须与全包规则常量保持同步，
  并点明 `RuleNodeSustainedMetricBreach` 声明在 `metric_breach.go` 而非上方常量块。

### `backend/internal/diagnosis/rule_test.go`

- 新增 `TestRuleIDsMatchCompiledRules`：与硬编码的 12 条完整名册逐条比对、并检查空值与重复。
  该名册是用户可见契约（analyzer discovery + golden replay），在测试里固化是有意为之——
  增删规则必须同时改这里，契约不能再静默漂移。
- 新增 `TestRuleIDsIncludesMetricBreachRule`：钉住本次回归。

### `backend/internal/inspection/service_test.go`

- 构造 `Service` 后注入固定时钟：`svc.now = func() time.Time { return now }`，
  使窗口以 fixture 时间点度量，而不是墙钟时间。生产行为不变（`Service.now` 默认仍是 `time.Now`）。

## Verification

- `cd backend && gofmt -l ./internal/diagnosis/ ./internal/inspection/`：无输出。
- `go build ./...`：通过。
- `go test ./internal/diagnosis/ ./cmd/server/ ./internal/golden/ -count=1`：**全绿**。
- `go test ./... -count=1`：**79 个包 `ok`、0 失败**（修复前 `internal/inspection` 单点失败）。
- `go test ./internal/inspection/ -run TestServiceCoverage_AggregatesWindow -count=1`：由红转绿。

## Risks / Notes

- 新增的 `RuleIDs()` 条目会进入 golden analyzer-discovery 契约快照与任何消费该契约的
  下游读取端；现有测试只断言非空，无硬编码 11 条的断言，故无破坏。
- 本次未改动任何规则**实现**与判定逻辑，只修正契约名册，诊断结果不变。
- 覆盖率口径提醒：本次未提升覆盖率数值，全局语句覆盖率仍为 70.0%（门禁下限）。
