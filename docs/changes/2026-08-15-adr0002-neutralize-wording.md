# ADR 0002 措辞中性化

- Date: 2026-08-15
- Status: Complete
- Scope: 公众门面一致性收尾——ADR 0002 遗留的私有交付语境措辞中性化。

## Context

门面自查发现 `docs/adr/0002-modular-monolith-and-request-pipeline.md`
第 8 行仍含一处指向个人交付语境的措辞（「X 需要清晰的模块边界」式表述），
不符合公开仓库的中性化要求。

## What Changed

`docs/adr/0002-modular-monolith-and-request-pipeline.md` 第 8 行：

- 私有交付语境措辞 →「项目需要」（仅此一处，保持技术语义不变）。

## Verification

- `grep -rn` 检索该措辞于 `docs/adr/`：无命中。
- 全库门面（SECURITY/CONTRIBUTING/dependabot/badges/README/CHANGELOG/docs）：
  个人交付语境措辞为零（本地私有材料已于早前移出公开仓库，不受影响）。

## Risks / Notes

- 纯文档措辞，无代码/行为影响。
