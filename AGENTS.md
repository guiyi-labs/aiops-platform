# AGENTS.md — 归档与交付规范（强制）

本文件覆盖仓库根目录下所有文件。任何代码、文档、配置、测试或资产改动，
**都必须归档后方可视为完成**。未归档的修改视为未完成，禁止提交。

## 1. 铁律：所有改动必须归档

- 每一次 `apply_patch` / 代码编辑 / 搭建新功能，都必须落到一处
  `docs/changes/YYYY-MM-DD-<slug>.md`（change-record）中。
- 用户可见、影响交付的改动必须同步更新 `CHANGELOG.md`（Unreleased 区段）。
- 完成一个里程碑/阶段时，打版本基线 tag：`baseline-<slug>-YYYYMMDD` 并推送。
- 例外：纯 refactor / 拼写修正仍须登记，但可合并进同一份 change-record。

## 2. 推荐工作流（每次改动）

1. 编写/修改代码。
2. 跑对应门禁：`go test ./...`、`pnpm typecheck`、`pnpm test -- --run`、`pnpm build`。
3. 写 `docs/changes/*.md` change-record（结构见 `docs/ARCHIVING.md` §3 模板）。
4. 更新 `CHANGELOG.md`。
5. `git add` 全部相关文件后一次 `git commit`，message 用
   `feat(scope): 描述 (#适用PR可选)`。
6. 多条相关提交合并 push；阶段性打 tag 推送。

## 3. 归档完整性检查（提交前）

- [ ] `git status --porcelain` 中没有被遗漏、无 change-record 对应的改动文件。
- [ ] 新增/变更行为都在 `docs/changes/` 有记录，且链接可点击。
- [ ] `CHANGELOG.md` Unreleased 与本次改动对应。
- [ ] 涉及集群/发布行为时，证据（.artifacts、日志片段）要么入库要么在 change-record 注明位置。

## 4. 违反后果

- 未归档的改动不提交、并入回退，等同未交付。
- 每次会话结束前必须重新核对工作树：任何游离改动必须立即归档或还原。

## 5. 文档索引

- 归档手册（规则/模板/流程/示例/清单）：`docs/ARCHIVING.md`
- change-record 模板：`docs/changes/TEMPLATE.md`
- 变更历史索引：`CHANGELOG.md`
- 里程碑变更记录：`docs/changes/`