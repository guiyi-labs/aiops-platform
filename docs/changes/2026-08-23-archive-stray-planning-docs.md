# 归档遗留计划文档并修复 CHANGELOG 结构

- Date: 2026-08-23
- Status: Complete
- Scope: 将 6 个游离于版本控制之外的规划文档归档（5 入库 + 1 转本地私有），并把 CHANGELOG 中 13 个重复的 `[Unreleased]` 小节头合并为 1 个。

## Context

AGENTS.md 铁律要求所有改动归档后方可视为完成。`git status --porcelain` 长期显示
6 个 untracked 文档（2026-08-16 前后的 P1/P2 规划、前端分析、英文 README 草稿、
Star 冲刺手册），均无对应 change-record，违反归档完整性检查第 1 条。
本记录一次性收口该欠账，作为「毕设打磨」阶段的第一步（仓库卫生）。

## What Changed

### 入库文档（5 个，均为技术规划，无敏感信息）

- `docs/enhancement-frontend-analysis.md`：P2b 前端精修分析（已随 P2b 执行）
- `docs/enhancement-operator-plan.md`：Operator/CRD 规划（已随 v0.1.0 落地）
- `docs/enhancement-p1-rag-diagnosis-plan.md`：P1 RAG 知识库规划（已随 bee2ef1..a889b2a 落地）
- `docs/enhancement-p2-flagship-roadmap.md`：P2 旗舰四方向总方案（执行中）
- `docs/aiops-readme-en-v1.md`：英文 README v1 草稿（0fcaacd 已吸收定稿，留档）

### 转本地私有（1 个，不入库）

- `docs/star-playbook.md`：Star 冲刺作战手册含个人求职策略上下文，
  按 `docs/thesis/_LOCAL_ONLY_NOTICE.md` 同等纪律加入 `.git/info/exclude`
  本地排除清单，不进入公开仓库。

### CHANGELOG 结构修复

- `CHANGELOG.md`：删除 12 个散落的重复 `## [Unreleased]` 小节头
  （M102–M107 时代各里程碑曾各自追加小节头而非并入顶部区块），
  合并为唯一一个 `[Unreleased]` 区块并附来源说明；净变化 +3/−12 行，
  不改动任何条目内容。
- 有意保持 `[0.1.0]` 位于 `[Unreleased]` 之前：v0.3.0-rc.* 系列 tag 先于
  v0.1.0 stable 切出，版本时间线交错，机械换位会暗示错误的时序。

## Verification

- `grep -inE "password|secret|token|api[_-]?key|admin123"` 扫描 6 个文档：
  仅 aiops-readme-en-v1.md 命中架构图中的 `kubeconfig` 字样（Mermaid 节点文本，
  非凭据），无任何敏感值。
- `grep -c '^## \[Unreleased\]$' CHANGELOG.md`：13 → **1**；
  `git diff --stat`：`CHANGELOG.md | 15 +++---`（3 insertions, 12 deletions），
  与「删 12 个重复头 + 加 3 行说明」严格一致，条目内容零改动。
- `git status --porcelain`：归档后仅剩本 change-record 与 CHANGELOG 编辑，
  提交后工作树干净。

## Risks / Notes

- star-playbook.md 若日后决定公开，需先从 `.git/info/exclude` 移除并补独立
  change-record；在此之前任何 `git add -f` 都应被视作违规。
