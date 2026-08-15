# ADR 0002 措辞中性化：毕设 → 项目

- Date: 2026-08-15
- Status: Complete
- Scope: 公众门面一致性收尾——ADR 0002 遗留「毕设」措辞中性化。

## Context

指挥中枢门面自查发现 `docs/adr/0002-modular-monolith-and-request-pipeline.md`
第 8 行仍含「毕设需要清晰的模块边界」措辞，不符合公开仓库中性化要求。

## What Changed

`docs/adr/0002-modular-monolith-and-request-pipeline.md` 第 8 行：

- 「毕设需要」→「项目需要」（仅此一处，保持技术语义不变）。

## Verification

- `grep -rn "毕设" docs/adr/`：无命中。
- 全库门面（SECURITY/CONTRIBUTING/dependabot/badges/README/CHANGELOG/docs）：
  毕设/答辩措辞为零（docs/thesis 已随 a83ff5a 移出公开仓库，不受影响）。

## Risks / Notes

- 纯文档措辞，无代码/行为影响。
