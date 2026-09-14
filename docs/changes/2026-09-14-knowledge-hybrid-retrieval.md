# knowledge：向量召回阶段与 RRF 混合检索（Phase 2 落地）

- Date: 2026-09-14
- Status: Complete
- Scope: `internal/knowledge` 补上 Phase 2 —— 新增离线确定性嵌入器、向量召回阶段与 RRF 融合，`cmd/aiopsbench` 新增 `retrieval-hybrid` 消融子命令。把此前只被注释标为"未来"的召回缺口做成已实现且可量化的能力。

## Context

`retriever.go` 自 Phase 1 起就带着一行注释：

> Phase 2 (future, pgvector) will add a semantic embedding stage.

`docs/changes/2026-08-23-aiopsbench-quality-benchmark.md` 的实测把这件事从"计划"变成了"证据"：
新近序结构化检索在 Pod 类条目 ≥4/规则后开始丢命中，15 条/规则时 Hit@3 归零 —— 候选池
完全由字段精确匹配决定，未入选的案例永远没有机会被排序救回。那份记录明确把 Phase 2
（语义召回 / LLM 重排）写成了动机。本次兑现其中的召回部分。

另一个直接动机来自规则本体的形态：本平台所有规则 id 都带版本后缀（`.v1`）。规则晋升到
`.v2` 时，旧 id 下沉淀的历史案例会**静默失联** —— 这不是假想场景，而是 id 命名方式内建的
风险。`retrieval-hybrid` 就是为量化它而写。

## What Changed

### internal/knowledge

- `embedding.go`（新增）：
  - `EmbeddingProvider` 窄接口（`Embed` / `Dimension` / `Name`），可替换为神经实现。
  - `LexicalEmbeddingProvider`：确定性、零依赖的哈希嵌入（signed hashing trick）。
    中文按 Han 连续段切 bigram（连续段长度为 1 时退化为单字），ASCII 按词切，
    子线性 TF 加权（`1+log(tf)`），L2 归一化。空文本得到零向量。
  - `CosineSimilarity`：按共享前缀计点积与范数（宽度不一致时不产生无几何意义的数）。
  - `Entry.EmbeddingText()`：把 rule id / 资源标识 / 摘要 / 根因拼成被索引的文本；
    刻意**不含** recommendations —— 那是处置措施，不是被匹配的症状。
- `retriever.go`：
  - `RetrieverConfig` 新增 `VectorEnabled` / `VectorTopK` / `VectorCandidateSize` /
    `RRFRankConstant`；`DefaultConfig()` 给出可用默认；`NewRetriever` 补齐零值回退。
  - `WithEmbeddingProvider`：链式可选挂载。不挂载时行为与 Phase 1 **完全一致**，
    现有调用方零改动、零破坏。
  - `Retrieve` 由两阶段扩为四阶段：结构化 B-tree → 向量召回 → RRF 融合 → 可选 LLM 精排。
    向量阶段与精排均 **fail-open**：嵌入失败或返回数量不符都只回退上一阶段结果，
    knowledge 故障依旧不阻断诊断链。
  - `fuseReciprocalRank`：k=60 的 RRF。选它是因为无需在"排名位次"与"余弦相似度"
    之间标定权重 —— 那两个量不可比，任何加权系数都会是拍脑袋的常数。
  - `vectorRecall`：以宽松过滤（仅严重度下限）取候选池后应用层打分。**不收窄
    rule_id / resource kind**，因为那个收窄正是要补的召回缺口。
- `model.go`：包注释更新为四阶段混合管线，并写明规模边界。
- `cmd/server/main.go`：挂载离线嵌入器；顺带修正一条过时注释
  （`re-rank is a Phase-2 API call` —— LLM 重排早已实现且默认开启）。

### cmd/aiopsbench

- `retrieval_hybrid.go`（新增）+ `main.go` 注册子命令：2×2 消融
  （aligned / renamed × structured / hybrid），3 档语料规模，hermetic。

## Verification

- `go test ./internal/knowledge/ -cover`：ok，coverage **92.4%**。
- `go vet ./internal/knowledge ./cmd/aiopsbench`：无输出。
- `gofmt -l .`（backend 全仓）：无输出。
- 消融实测 `go run ./cmd/aiopsbench retrieval-hybrid`（节选，aligned 各行在全部规模上均为 1.000）：

| per-rule | corpus | scenario | method | Hit@1 | Hit@3 | MRR |
|---|---|---|---|---|---|---|
| 2 | 24 | renamed | structured | **0.000** | **0.000** | **0.000** |
| 2 | 24 | renamed | hybrid | 1.000 | 1.000 | 1.000 |
| 5 | 60 | renamed | structured | 0.000 | 0.000 | 0.000 |
| 5 | 60 | renamed | hybrid | 1.000 | 1.000 | 1.000 |
| 10 | 120 | renamed | structured | 0.000 | 0.000 | 0.000 |
| 10 | 120 | renamed | hybrid | **0.917** | 1.000 | **0.958** |

结论：结构化的强项（字段可匹配时 Hit@1=1.000）未被牺牲；规则重命名后结构化彻底失效
（0.000），混合管线恢复到 0.917–1.000。规模越大差距越明显。

## Risks / Notes

- **规模边界（重要，不要略过）**：向量阶段在应用层对候选池打分，候选池由
  `VectorCandidateSize` 限定，而 `ListByFilter` 按新近序截断 —— 库容量超过池容量时，
  最旧的条目**仍不可达**。当前默认 200，实测语料 ≤120 条。超过该规模需要物化向量 +
  ANN 索引（pgvector 等），届时只换 `Repository` 实现，调用方不变。这是诚实的边界，
  不是已经解决的问题。
- **嵌入器是词法的，不是语义的**：`LexicalEmbeddingProvider` 靠 n-gram 重叠而非神经语义。
  选它的理由是**可复现** —— 单测与基准无需 API key、无需模型下载、跨机器结果一致。
  生产可换神经实现（接口已就位），但届时需要补一组对照实验。
- **未引入 pgvector**：本次刻意不新增数据库扩展与迁移，保持部署面不变。
- **`per-rule=10` 时 hybrid Hit@1=0.917 而非 1.000**：是真实存在的 1/12 失败
  （词汇碰撞），未做调参掩盖。
