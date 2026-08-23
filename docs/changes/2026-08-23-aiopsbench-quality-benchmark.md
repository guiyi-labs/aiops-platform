# aiopsbench：离线质量基准工具（诊断 P/R/F1 + 案例检索 Hit@k）

- Date: 2026-08-23
- Status: Complete
- Scope: 新增 `backend/cmd/aiopsbench`（诊断标注语料回放 + 知识库检索质量度量），将 knowledge 包的 InMemoryRepository 从测试文件提升为正式实现。为毕设实验章节提供可复现、可 CI 守护的量化证据链。

## Context

毕设打磨目标 ①「实验量化补强」：此前的工程证据是步骤级 golden 契约
（pass/fail）与覆盖率，缺少论文实验章节需要的标准指标——分类的
Precision/Recall/F1 与检索的 Hit@k/MRR。同时 `internal/knowledge` 的
InMemoryRepository 只存在于 `_test.go`，离线评测无法复用。

## What Changed

### 新增 cmd/aiopsbench

- `backend/cmd/aiopsbench/main.go`：子命令分发（diagnosis / retrieval），
  两模式均为 hermetic（不触集群/数据库/AI Provider）。
- `backend/cmd/aiopsbench/diagnosis.go`：
  - 语料模型与 `evaluateTarget`：按 target_rule 调用生产同款导出规则函数
    （12 条规则全覆盖）；`evaluatePipeline` 复刻 evaluatePod 优先级链
    （oom → image_pull → crash_loop → pending）度量 top-1 选择。
  - 指标：逐规则 TP/FN/FP/TN → Precision/Recall/F1；micro/macro F1；
    标签一致率；pipeline top-1 准确率；`-json` 输出机器可读报告。
- `backend/cmd/aiopsbench/retrieval.go`：确定性合成案例库（12 规则 × N 条 ×
  4 资源类型轮转），ground truth = 该规则最旧的 Pod 类条目（对新近序索引
  最难命中），跨语料规模 {2,5,8,10,15,20,30} 度量 Hit@1/Hit@3/MRR。
- `backend/cmd/aiopsbench/testdata/diagnosis-corpus.json`：38 个标注场景
  （12 规则 × 正例变体 + 近邻负例），标签锚定已评审单测
  （rule_test.go / m18-fixtures.json / metric_breach_test.go）与规则规格，
  不以引擎输出反标。
- `backend/cmd/aiopsbench/main_test.go`：
  - `TestCorpusLabelsMatchEngine` —— 语料即 CI 契约：规则行为变化破坏任一
    标签即测试失败，强制显式评审或重新标注。
  - `TestRetrievalBenchDeterministic` / `TestRetrievalShortlistBoundary`
    —— hermetic 性与新近序 shortlist 容量边界守护。

### knowledge 包

- `internal/knowledge/memory.go`（新增）：InMemoryRepository 从
  `memory_test.go` 提升为正式实现，供单测 / aiopsbench / 未来 CLI 降级路径
  复用；`ListByFilter` 补齐与 Gorm 生产版一致的 `noted_at DESC` 排序语义
  （原测试桩无排序，属保真度偏差）。
- `internal/knowledge/memory_test.go`（删除）：内容并入 memory.go。

## Verification

- `go vet ./cmd/aiopsbench && go test -count=1 ./cmd/aiopsbench`：ok
  （38/38 标签契约 + 检索确定性 + 边界退化断言全绿）。
- `gofmt -l .`（backend 全仓）：无输出。
- `go test -count=1 ./internal/knowledge ./cmd/...`：全部 ok。
- 基准实测（`.artifacts/bench/`）：
  - diagnosis：micro F1=1.000 / macro F1=1.000 / pipeline top-1=13/13。
    确定性规则在规格一致语料上满分符合预期；该数字的价值在于此后任何
    规则改动都会被 38 场景契约守护。
  - retrieval：shortlist=10 下，Pod 类条目 ≤3/规则时 Hit@1=1.000；
    ≥4 后旧案例跌出 shortlist，Hit@3 在 15 条/规则时归零 —— 量化暴露了
    新近序结构化检索的容量边界，为 Phase 2（pgvector 语义召回 /
    LLM 重排）提供实证动机。

## Notes — P2a 覆盖率门禁关联

本提交引入的 `InMemoryRepository.ListByFilter` 已对齐生产语义
`noted_at DESC`（原 `memory_test.go` 桩无排序）。该变更被
`retriever_test.go` 的短名单边界用例与 `aiopsbench retrieval` 的
Hit@k 衰退曲线共同覆盖；不单独增 ADR，遵循既有 change-record 约定。
CI 门禁保持：全局 ≥ 70%、核心包 ≥ 75%（`.github/workflows/ci.yml`
P2a 小节已落地）。

## Risks / Notes

- 语料规模尚小（38 场景）：结论限定为"规格一致性回归"，不宣称真实集群
  精确率；扩展场景只需增补 JSON 并跑 `-json` 报告。
- InMemoryRepository 进入生产包但不含任何 I/O；如后续需要并发版本应另行
  实现，不在其上扩展。
