# P2：Release Notes（v0.3.0-rc.6）与 Helm Chart 元数据修正

- Date: 2026-08-15
- Status: Complete
- Scope: 新增发布说明文档；修正 Helm Chart 元数据（home URL / maintainer）；走分支+PR+自我 review 流程

## Context

Obsidian「GitHub 项目展示优化与后续路线」P2 清单：
- 为成熟仓库维护版本 Release Notes、兼容范围和升级/回滚说明（本仓库保持 RC 口径，不宣称 GA）。
- 为代表性功能使用分支和 PR，保留至少一轮自我 Review 记录（本改动即代表样例）。
- 统一仓库 README/元数据链接一致性（Chart.yaml home URL 指向错误仓库 `aiops/aiops-platform`）。

## What Changed

### 文档
- `docs/releases/RELEASE-NOTES.md`：新增 v0.3.0-rc.6 发布说明（定位 RC 边界、能力摘要、
  兼容范围矩阵、从 rc.5 升级步骤、回滚说明、已知限制），关联 Issue #16/#17/#18。

### 配置
- `deploy/helm/aiops-platform/Chart.yaml`：`home` 从 `https://github.com/aiops/aiops-platform`
  修正为 `https://github.com/guiyi-labs/aiops-platform`；`maintainers.name` 从 `aiops-team`
  修正为 `Guiyi Labs`。版本号保持 0.1.0（release.yml 在打包时以 `VERSION` 覆盖，避免无谓变更）。

### 归档
- 本 change record；`CHANGELOG.md` Unreleased 增加条目。

## Verification

- `helm lint --strict deploy/helm/aiops-platform` 通过（本地 helm 可用时）；CI release.yml
  `helm package --version ${VERSION#v}` 覆盖版本号，不受 Chart.yaml version 影响。
- 流程验证：本改动经 `codex/p2-release-notes` 分支 → PR → 自我 Review 评论 → squash merge，
  代表 P2「分支 + PR + 自我 review」落地样例。

## Risks / Notes

- Release Notes 为 RC 口径；正式 GA tag 仍待 M89/M90 授权验收后再发布。
- 若后续 Chart 版本需与 RC 资产对齐，可单独提交并以 CI 打包验证。
