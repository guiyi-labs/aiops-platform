# 归档手册（Archiving Guide）

- Status: Accepted
- Updated: 2026-08-09
- 范围：仓库根目录下所有产物的变更登记。
- 强制规范总入口：`AGENTS.md`（根目录）。

本仓库的"最高水准"建立在**可追溯的变更档案**之上：每一处改动都能回答
"改了什么、为什么改、如何验证、影响哪个契约"。以下定义了统一规则。

---

## 1. 归档的四个层级

| 层级 | 载体 | 时机 |
|---|---|---|
| 行为记录 | `docs/changes/YYYY-MM-DD-<slug>.md` | 每次代码/配置/文档产物改动 |
| 产品级变更 | `CHANGELOG.md`（Unreleased 区段） | 用户可见/交付影响的改动 |
| 决策记录 | `docs/adr/DDD-*.md` | 影响架构、契约、安全边界的决策 |
| 交付基线 | git tag `baseline-<slug>-YYYYMMDD` | 每个里程碑/阶段封口并推送 |

## 2. 文件命名与目录

- change-record：`docs/changes/2026-08-09-ci-dependabot-recovery.md`
  （日期 + 短横线 slug，小写英文 kebab-case）。
- 里程碑用 `mNN` 或 `wNN` 前缀（如 `w12-real-cluster-e2e`）。
- 运维/故障/恢复类用具体主题（如 `ci-dependabot-recovery`）。
- 所有记录集中 `docs/changes/`；`docs/README.md` 维护索引入口。

## 3. change-record 模板

`docs/changes/TEMPLATE.md` 提供了最小完整结构。标准字段：

```
# <Slug 标题>
- Date: YYYY-MM-DD
- Status: Draft | Complete | Superseded
- Scope: 一句话范围
## Context      — 背景、为什么
## What Changed — 具体改动（文件级）
## Verification— 如何验证（命令、结果、证据位置）
## Risks / Notes— 已知风险、回退方式、后续
```

## 4. 提交与标签流程

```bash
# 1) 门禁
cd backend && go test -p=1 -count=1 ./...
cd frontend && pnpm typecheck && pnpm test -- --run && pnpm build
# 2) 写 change-record + 更新 CHANGELOG
# 3) 一起提交
git add docs/changes/xxx.md CHANGELOG.md <code...>
git commit -m "feat(scope): 描述"
# 4) 阶段封口打 tag
git tag baseline-xxx-YYYYMMDD
git push origin main --tags
```

## 5. 归档完整性检查表（提交前/会话收尾）

- [ ] `git status --porcelain` 无游离改动。
- [ ] 每处改动有对应 change-record 且已 add。
- [ ] CHANGELOG 与改动对应。
- [ ] 证据（.artifacts、日志）已入库或有明确位置说明。
- [ ] 不影响既有契约时已确认；影响时同步 ADR。

## 6. 常见场景速记

- **依赖升级 / dependabot PR**：CI 绿、无破坏即可；失败时把"合并 main→触发重跑"记入 change-record。
- **故障恢复 / 工作树还原**：必须记录"改了什么被还原、为什么"。
- **纯文档**：仍写 change-record，条目可极简。

## 7. 例外与边界

- 自动生成文件（`pnpm typegen` 产物、`*.pb.go` 等）不手写 change-record，但标记来源。
- 临时调试文件不得进入 `docs/changes/`，就地清理。
- 归档优先于"快"：不做即视为未完成。