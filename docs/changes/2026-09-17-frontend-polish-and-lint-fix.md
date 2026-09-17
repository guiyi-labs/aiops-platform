# frontend-polish-and-lint-fix：固化前端打磨改动并修复 lint/构建配置

- Date: 2026-09-17
- Status: Complete
- Scope: 将此前未提交的 UI 打磨改动落盘，并修复一处 lint 问题、完善前端构建忽略规则。

## Context

此前会话对诊断详情视图与导航壳做了打磨（证据卡结构化、侧边栏标签），但改动只停留在工作区、未提交，导致 Docker 部署版（:18080）与 dev server（:5173）看到的界面不一致。同时全仓 golangci-lint 存在 1 个 staticcheck SA4000（mcpserver 测试自比），前端构建也缺少对 `.env` 的忽略。

## What Changed

### 前端打磨（用户可见）
- `frontend/src/views/DiagnosesView.vue`：诊断详情证据由原始 JSON 倾倒改为结构化标签渲染（`evidence-fields`）；标题标注可追溯条数。
- `frontend/src/components/ConsoleLayout.vue`：侧边栏标签由「本地开发环境」改为「Kubernetes AIOps 平台」。

### 构建与质量
- `compose.yaml`：为 backend/frontend 增加本地镜像标签（`k8s-aiops-backend:latest` / `k8s-aiops-frontend:latest`），兼容离线/受限网络下的 `docker compose up`（避免拉取 Docker Hub 超时）。
- `backend/internal/mcpserver/tools_test.go`：修正 `TestInputSchemaIsStableAcrossCalls` 的自比断言（SA4000），改为两次独立调用结果比对。
- `frontend/.dockerignore`：新增 `.env*` 排除，避免本地环境变量进入镜像。

## Verification

- `golangci-lint run ./internal/mcpserver/...`：0 issues（修复前为 1 个 SA4000）。
- `golangci-lint run ./...`：全仓 0 issues（此前唯一 issue 已消除）。
- 前端 dev server（:5173）与 Docker 部署（:18080）经本次提交 + 镜像重建后将一致体现打磨。

## Risks / Notes

- compose 镜像标签为离线构建兼容措施；若环境可正常访问 Docker Hub，仍可删除标签走自动构建。
- 本次未改动任何诊断/关联/案例记忆核心逻辑，不影响三大核心模块的测试与覆盖率。
