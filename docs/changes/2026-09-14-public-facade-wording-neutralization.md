# 公众门面措辞中性化收尾：清除遗留的个人交付语境措辞

- Date: 2026-09-14
- Status: Complete
- Scope: 公开仓库文本面（CHANGELOG / docs / 本地 exclude 注释）中残留的个人交付语境措辞
  （指向课程、学位、答辩、论文的表述）全部中性化，保留技术语义不变；同时清掉指向已迁出目录的路径引用。

## Context

论文材料已于 2026-09-14 整体迁出至独立私有仓库（`docs/thesis/` 等路径退役）。
但此前一轮「门面中性化」只覆盖了 `docs/adr/` 一处，公开仓库的**历史变更记录与规划文档**中
仍有 7 个文件、17 处遗留措辞：这会让仓库的读者（含潜在面试官）读到与工程无关的交付语境，
也与「公众门面保持一致」的既有约定不符。

同时发现 4 处对已迁出目录 `docs/thesis/` 的路径引用，指向不存在的路径。

## What Changed

### 措辞中性化（8 个文件）

| 文件 | 处理 |
|---|---|
| `CHANGELOG.md` | 4 处：验收记录条目、演示剧本条目、`docs/thesis/` 同等纪律表述、中性化条目自身 |
| `docs/changes/2026-08-15-adr0002-neutralize-wording.md` | 整体重写：不再复述被中性化的原始措辞，改以「私有交付语境措辞」指代 |
| `docs/changes/2026-08-23-aiopsbench-quality-benchmark.md` | 3 处：范围说明与「打磨目标」表述 |
| `docs/changes/2026-08-23-archive-stray-planning-docs.md` | 2 处：阶段命名 + `docs/thesis/` 路径引用 |
| `docs/changes/2026-08-23-macos-demo-rehearsal-path.md` | 6 处：演示/排练语境 + 实验用延迟曲线表述 + 2 处文档路径 |
| `docs/changes/2026-08-23-p2c-knowledge-readonly-seed.md` | 5 处：范围说明、演示语境、演示文档小节 + 3 处文档路径 |
| `docs/enhancement-operator-plan.md` | 2 处：基线版本描述 + 硬性约束中的路径引用 |
| `docs/enhancement-p2-flagship-roadmap.md` | 1 处：门禁清单中的路径引用 |

### 路径引用清理

- `.git/info/exclude` 注释：措辞中性化（该文件不推送，仅保持本地一致）。
- 上述 4 处 `docs/thesis/` 引用改为语义等价的中性表述（如「本地私有材料」），
  不再指向已不存在的目录。

## Verification

- `grep -rn "毕设\|毕业设计\|毕业论文\|答辩\|开题\|论文"`（排除 `.git` / `node_modules` /
  `.claude` / `.qoder`）：**0 命中**。
- `grep -rn "docs/thesis"`（已跟踪文件）：仅余 `materials/` 与历史记录中的必要说明，公开仓库内为 0。
- 纯文本改动，无代码/行为影响：`go build ./...` 与 `go test ./...` 结果不受影响
  （本次同批另有一处代码修改，见 `2026-09-14-diagnosis-rule-roster-and-clock-pinned-test.md`）。

## Risks / Notes

- 历史变更记录被改写措辞：**技术事实、结论、影响范围均未改动**，只替换与工程无关的交付语境词。
  如后续需要审计"当时到底写了什么"，以 git 历史（本提交之前的版本）为准。
- `.qoder/repowiki/` 与 `.claude/worktrees/` 下的副本未处理：前者是工具生成的本地缓存且已被忽略，
  后者是无用工作树残留，均不进公开仓库。
