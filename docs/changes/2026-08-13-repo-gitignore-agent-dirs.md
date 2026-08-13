# Repo Housekeeping：gitignore agent 工作区目录

- Date: 2026-08-13
- Status: Complete
- Scope: 仓库根 `.gitignore`

## What Changed

- 新增忽略 `.qoder/`（外部 agent 的分析/检索工作区，317 个非交付文件）与
  `.workbuddy/`（agent 产物目录，含交接提示词草稿），避免污染本地基线。
- 登录页 13–15 轮视觉打磨的完整归档不依赖这两个目录：进度均在
  `docs/changes/2026-08-13-login-ambience-enhance.md`（1–13 轮）、
  `docs/changes/2026-08-13-login-mobile-flush.md`（14 轮）、
  `docs/changes/2026-08-13-login-short-height.md`（15 轮）与 `CHANGELOG.md`。

## Verification

- `git status --porcelain` 仅剩本 change-record / `.gitignore` 改动，无游离文件。
