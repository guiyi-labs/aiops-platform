# archive-gate-change-record：为归档铁律添加最小机械门禁

- Date: 2026-08-13
- Status: Complete
- Scope: 新增 docs/changes/ change-record 机械门禁（CI job + 本地 pre-commit 钩子）

> better-harness finding `archive-rule-not-enforced` 的修复。

## Context

AGENTS.md §1 声明「所有改动必须归档，未归档的修改视为未完成，禁止提交」，
但 .git/hooks 仅有 sample、三个 CI workflow 均无 change-record 校验，
归档完全依赖人工 checklist。本次为该铁律补上最小机械强制点，
不改动 AGENTS.md 现有语义。

## What Changed

### 门禁脚本
- `scripts/check-change-record.sh`：新增。`--base <ref>`（CI）与 `--staged`
  （提交钩子）两种模式；改动包含非文档代码文件（docs/、README.md、
  CHANGELOG.md 以外，与 ci.yml changes job 的文档判定一致）时，要求同一改动
  存在 `docs/changes/YYYY-MM-DD-<slug>.md`；缺失时输出指向 AGENTS.md §1、
  TEMPLATE.md、docs/ARCHIVING.md 的可读错误并以非零码退出。

### CI
- `.github/workflows/ci.yml`：新增 `change-record` job（push/PR/dispatch 均运行，
  不依赖 runtime_required，base 解析与 changes job 同源）；`result` job 的
  needs 与必填结果集纳入该门禁，两个 runtime 分支下均要求 success。

### 本地提交钩子（可选）
- `scripts/git-hooks/pre-commit`：新增。`ln -s` 安装后在提交前以 `--staged`
  模式执行同一门禁；CI 侧 job 为最终兜底。

### 归档
- `docs/changes/2026-08-13-archive-gate-change-record.md`：本记录。
- `CHANGELOG.md`：Unreleased 新增对应条目。

## Verification

- 负例：暂存 `backend/gate-sample.go`（无 record）运行 `--staged`：exit=1，
  输出「拦截 —— 改动包含非文档代码文件，但同一改动中缺少 change-record」
  并列出文件与修复步骤（样例文件验证后已删除，未入库）。
- 正例：将本 change-record 与代码文件同批暂存后运行 `--staged`：exit=0，
  输出「检测到 change-record … 归档门禁通过」。
- 基线回归：`--base HEAD~1` 对既有已归档提交通过；文档-only 暂存通过。

## Risks / Notes

- 门禁只校验「存在一条命名合规的 change-record」，不校验内容与代码的对应
  质量，保持最小；历史 squash/多提交场景以 PR/push 的 diff 为界。
- 回退方式：删除 ci.yml 的 change-record job 与 result 引用即可恢复原状。
