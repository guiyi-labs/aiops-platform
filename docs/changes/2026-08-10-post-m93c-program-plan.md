# Post-M93-C：下一阶段项目推进长计划

- Date: 2026-08-10
- Status: Complete
- Scope: 将 M93-C 之后的执行路线扩展为 M93-B2–M102 的 12–16 周项目计划。

## Context

原 `docs/next-long-term-plan.md` 只覆盖 M93–M97，已明确产品、规模和发布方向，但缺少 M97 之后的
事故协作、信号关联、安全治理、生产韧性和 GA 准入，也没有把并行轨道、阶段门、风险、指标和前十个
工作日任务拆开。用户要求继续向下推进并形成可长期执行的项目计划。

## What Changed

### Execution Program

- 重写 `docs/next-long-term-plan.md`，规划 M93-B2–M102，估算周期为 12–16 个有效开发周。
- 保留 M93-B2、M94–M97 原主线，并新增 M98 事故工作空间、M99 信号关联与 SLO、
  M100 安全与租户治理、M101 生产身份/数据韧性集成、M102 GA 证据封口。
- 将 M89 OIDC/MFA 与 M90 WAL/PITR/HA 明确为组织授权轨；未完成时版本保持 RC，不降低 GA 门禁。

### Governance And Evidence

- 新增 Gate A–D，分别约束产品叙事、规模证据、Release Candidate 和 GA。
- 新增 12–16 周节奏、四条并行工作轨、项目指标、前十个工作日任务板和风险登记。
- 明确 M96 处理稳定嵌套路由壳层与 CSS 历史覆盖技术债，避免只以根底板掩盖布局重复挂载。
- 每个里程碑继续使用 ADR、测试、artifact、change-record、tag、远端 CI 和干净工作树作为 DoD。

### Documentation Alignment

- 更新 `docs/README.md`，将 `next-long-term-plan.md` 标为当前唯一执行入口。
- 更新 `docs/long-term-roadmap.md` 的建议顺序至 M102，并同步 `README.md`、`CHANGELOG.md` 与
  `docs/PROJECT_STATUS.md` 的计划入口和归档计数。

## Verification

- 文档链接、里程碑编号、M89/M90 外部依赖和 M93-B2 待办口径已交叉检查。
- `git diff --check`：通过。
- 本次为文档规划变更，不修改运行代码、API、数据库、部署清单或产品行为。

## Risks / Notes

- 工期按有效开发周估算，未包含组织审批、OIDC Provider、PITR/HA 基础设施或独立演练环境等待。
- 性能阈值必须由 M93-B2/M96 实测生成；本文只定义测量和门禁方法，不预写未经证实的绝对结果。
- M102 是准入门，不是日期承诺；Gate D 未通过时必须维持 RC。
