# 公众门面措辞中性化：清除公开仓库中的个人语境措辞

- Date: 2026-09-14
- Status: Complete
- Scope: 公开仓库文本面（CHANGELOG / docs / 本地 exclude 注释）中残留的个人语境措辞
  （与工程无关的个人身份与私人路线用词）全部中性化，保留技术语义不变；同时清掉指向已迁出目录的路径引用。
- Amended: 2026-09-14 二次修订。**本记录初版曾逐类枚举被移除的词，等于把要移除的词又写回了公开仓库**——
  即"清理动作本身成为泄漏源"。现改为不点名表述，并已由红线门禁（提交/推送前硬阻断）守护，
  见 [`2026-09-14-public-facade-redline-guard.md`](2026-09-14-public-facade-redline-guard.md)。

## Context

私有材料已于 2026-09-14 整体迁出至公开仓库之外的独立仓库（本仓库中相关目录退役）。
但此前一轮「门面中性化」只覆盖了 `docs/adr/` 一处，公开仓库的**历史变更记录与规划文档**中
仍有 7 个文件、17 处遗留措辞：这会让仓库的读者读到与工程无关的私人语境，
也与「公众门面保持一致」的既有约定不符。

同时发现 4 处对已迁出目录的路径引用，指向不存在的路径。

## What Changed

### 措辞中性化（8 个文件）

| 文件 | 处理 |
|---|---|
| `CHANGELOG.md` | 4 处：验收记录条目、演示剧本条目、已迁出目录的同等纪律表述、中性化条目自身 |
| `docs/changes/2026-08-15-adr0002-neutralize-wording.md` | 整体重写：不再复述被中性化的原始措辞，改以「私有语境措辞」指代 |
| `docs/changes/2026-08-23-aiopsbench-quality-benchmark.md` | 3 处：范围说明与「打磨目标」表述 |
| `docs/changes/2026-08-23-archive-stray-planning-docs.md` | 2 处：阶段命名 + 已迁出目录的路径引用 |
| `docs/changes/2026-08-23-macos-demo-rehearsal-path.md` | 6 处：演示/排练语境 + 实验用延迟曲线表述 + 2 处文档路径 |
| `docs/changes/2026-08-23-p2c-knowledge-readonly-seed.md` | 5 处：范围说明、演示语境、演示文档小节 + 3 处文档路径 |
| `docs/enhancement-operator-plan.md` | 2 处：基线版本描述 + 硬性约束中的路径引用 |
| `docs/enhancement-p2-flagship-roadmap.md` | 1 处：门禁清单中的路径引用 |

### 路径引用清理

- `.git/info/exclude` 注释：措辞中性化（该文件不推送，仅保持本地一致）。
- 上述 4 处已迁出路径引用改为语义等价的中性表述（如「本地私有材料」），不再指向不存在的目录。

## Verification

- 以**本地维护的词表**（不入库、不枚举）对全部已跟踪文件做扫描：公开仓库内 **0 命中**。
  词表与扫描器即红线门禁（`.git/info/redline-words.txt` + `.git/info/redline-guard.sh`）。
- 已迁出目录的路径引用：仅余历史记录中的必要说明，公开仓库内为 0。
- 纯文本改动，无代码/行为影响：`go build ./...` 与 `go test ./...` 结果不受影响
  （本次同批另有一处代码修改，见 [`2026-09-14-diagnosis-rule-roster-and-clock-pinned-test.md`](2026-09-14-diagnosis-rule-roster-and-clock-pinned-test.md)）。

## Risks / Notes

- 历史变更记录被改写措辞：**技术事实、结论、影响范围均未改动**，只替换与工程无关的私人语境词。
  如后续需要审计"当时到底写了什么"，以 git 历史（本提交之前的版本）为准。
- `.qoder/repowiki/` 与 `.claude/worktrees/` 下的副本未处理：前者是工具生成的本地缓存且已被忽略，
  后者是无用工作树残留，均不进公开仓库。
- **git 历史中仍有既往暴露**：清理只保证 HEAD 与后续提交干净；早期提交的内容与提交信息里仍留有
  私人语境词。历史重写（需 force push、会改变全部 SHA，并使既有版本锚点提交失效）**未执行**，
  取舍见红线记录。
