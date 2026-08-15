# 公开 README 基线与项目入口说明

- Date: 2026-08-14
- Status: Complete
- Scope: 对齐旗舰仓库 README 的公开项目状态、能力摘要和交付边界。

## Context

GitHub 主页已将 `aiops-platform` 作为基础设施方向的旗舰项目，但仓库 README 首屏仍停留在
M93 叙述，并把历史维护信息放在当前基线之前。访客需要先确认项目现在做到什么程度、有哪些可验证
能力，以及哪些生产条件尚未具备。

## What Changed

### README

- `README.md`：将首屏基线更新为 M112（2026-08-14），增加多集群运维、可观测诊断、事故响应、
  AI 辅助、受控运维和工程交付六个方向的能力摘要。
- `README.md`：修正 CI 覆盖率门禁的过时表述（60% → 65%），并链接当前执行路线、变更记录和
  文档演示材料。
- `README.md`：将 RC 边界、生产 OIDC/MFA、HA/PITR 和发布授权限制放到清晰的项目边界提示中，
  将历史安全脱敏说明下移到 Repository Notes。
- `README.md`：把 M1-M32 内容标记为早期里程碑索引，避免与当前 M112 状态混淆。

## Verification

- `git diff --check`：通过。
- README 旧基线关键词检查：未发现 M1-M93、2026-08-08、60% 和旧执行计划入口残留。
- 未运行代码测试：本次仅修改公开文档，不改变代码、配置、接口或运行时行为。

## Risks / Notes

- README 的能力摘要依赖现有 `CHANGELOG.md` 和 `docs/changes/` 证据；后续里程碑完成时应同步更新
  “当前基线”段，避免再次产生状态滞后。
- GitHub 个人简介的更新需要账户具备 `user` scope，本次令牌未授权，因此未在此仓库内代替完成该项。
