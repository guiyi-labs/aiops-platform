# diagnoses-view-ts-fix：修复 DiagnosesView 未使用声明导致的生产构建失败

- Date: 2026-09-17
- Status: Complete
- Scope: 移除 DiagnosesView 中前两轮打磨引入的未使用声明，使 `pnpm build`（vue-tsc 严格模式）通过。

## Context

前一轮 UI 打磨（证据卡结构化渲染）在 `DiagnosesView.vue` 中引入了一个未被调用的辅助函数 `evidenceHeadline`，并在两处 `v-for="(item, ei) in detail.evidence"` 中保留了未使用的索引 `ei`。Vite dev server（:5173，HMR）不做类型检查，因此本地预览正常；但 `pnpm build` 执行 `vue-tsc -b` 严格类型检查时报 TS6133（声明已定义但从未读取），导致 Docker 镜像构建在 `RUN pnpm build` 阶段失败。

## What Changed

### 前端（构建修复）
- `frontend/src/views/DiagnosesView.vue`：
  - 删除未使用的 `evidenceHeadline` 辅助函数（其唯一调用点并不存在）。
  - 将两处 `v-for="(item, ei) in detail.evidence"` 改为 `v-for="item in detail.evidence"`，移除未使用的索引 `ei`。

## Verification

- `docker compose build frontend`：镜像构建在 `RUN pnpm build` 阶段通过（修复前因 TS6133 退出码 2 失败）。
- 行为与界面无变化——仅移除死代码，证据卡结构化渲染、侧边栏标签等打磨效果保持不变。

## Risks / Notes

- 纯死代码移除，无逻辑或视觉差异。
- 本次仅修复前端构建，不影响后端诊断/关联/案例记忆核心逻辑与覆盖率。
