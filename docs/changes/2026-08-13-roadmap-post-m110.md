# 后续开发路线规划：M110 收口 + M111–M115 执行序

- Date: 2026-08-13
- Status: Complete
- Scope: 新增 `docs/development-roadmap-post-m110.md` 作为 M110 之后的执行入口，取代
  `docs/development-roadmap-post-m106.md` 的 M107–M110 执行序，并登记 CHANGELOG 与
  文档指针更新。

## Context

M107–M109 已封口，M110（v0.3.0-rc.6）本地发布预检 15 项全过（`docs/m110-release-preflight.md`），
剩余动作是用户授权的发布动作。项目进入"功能闭环可用 → 可运营/可度量/可解释"阶段，
需要一份明确的后续路线，覆盖：M110 收口、并行前端轨收口、主线 M111–M115、M89/M90
授权轨（Deferred）。

## What Changed

- 新增 `docs/development-roadmap-post-m110.md`：完整执行路线
  - Track 0 M110 收口（用户决策项：推送本地提交、授权 push rc.6、发布后演练、封口）；
  - Track A 前端优化轨收口（CSS token 层收敛、关键页面截图基线、≤720px 响应式审计、
    交互统一，衔接契约与门禁）；
  - Track B M111 事故响应深化（runbook 关联、MTTA/MTTR KPI、事故模板与严重级矩阵、
    SLA 升级链、复盘导出）；
  - Track C M112 AI 协调查询与解释深化（会话式调查、AI 事故摘要、解释覆盖率大盘，
    严守引用纪律）；
  - Track D M113 优化中心闭环与巡检深化（finding→runbook 预览导航、巡检趋势/覆盖率
    度量；受控操作目录扩展为可选契约评审项）；
  - Track E M114 可观测性深化（SLO burn 扩展与告警降噪、指标历史下采样、事件流/日志
    探索增强）；
  - Track F M115 工程卓越冲刺（覆盖率 65%→70%、性能基准入 CI fail-closed、fuzz 扩展）；
  - Track G M89/M90 授权轨（Deferred，随时可启动，指向 `authorization-gate-prep.md`）；
  - 编排与门禁总表、非目标（沿用既有边界，不新增写路径）。
- `docs/development-roadmap-post-m106.md`：头部增加一行"已被 post-m110 取代（M111+）"
  指针。
- `CHANGELOG.md`（Unreleased）：登记本路线规划条目。

## Verification

- 纯文档改动（docs/ + CHANGELOG.md），不触发 `scripts/check-change-record.sh` 代码门禁；
  已按 AGENTS.md §1 登记 change-record 并更新 CHANGELOG。
- 文档内引用的既有入口（`long-term-roadmap.md`、`polish-plan.md`、
  `authorization-gate-prep.md`、`m110-release-preflight.md`、`m96-gate-b-baselines.md`、
  `known-limitations.md`）与仓库现状一致。

## Risks / Notes

- M110 发布动作（push `v0.3.0-rc.6` tag、触发远端 Release）为不可逆远端动作，必须由
  用户授权后执行；本路线只提供清单与顺序，不自动执行。
- M111–M115 的范围为建议序，实际优先级可随用户调整；受控操作目录扩展（M113 可选
  项）涉及 ADR 与契约变更，需单独评审。
- M89/M90 未授权前版本保持 RC，不宣称 GA。
