# README 英文重写（Star 版首屏）— 落地

- Date: 2026-08-16
- Status: Complete
- Scope: P2b 按 docs/star-playbook.md 与 docs/aiops-readme-en-v1.md 骨架执行
- DependsOn: P1 RAG 知识库（a889b2a）

## What

用英文骨架完整替换 `README.md`（原 270 行中文 M115 版 → 英文 Star 版），
目标：仓库首屏 30 秒看懂，直接服务 Star 冲刺。

### 1. 首屏（第一屏）

- 一句话定位：**Kubernetes fault diagnosis with case memory**——确定性规则诊断
  + 自动蒸馏可检索案例库，同类故障秒回历史修法。
- `go install github.com/guiyi-labs/aiops-platform/cmd/aiops@latest` 一行体验
  （CLI 实现紧随其后，README 先行预留）。
- 定位标签行 + 徽章行（CI / Coverage ≥70% / Go 1.26 / Vue 3 / K8s 1.36 /
  Apache 2.0，全部为真实数值）。

### 2. 正文

- **What makes it different**：4 条差异化卖点（确定性诊断 / 案例记忆 RAG /
  可校验引用 AI 解释 / 受控运维）。
- **四象限能力表**：诊断 | 记忆 × 受控运维 | 平台，一屏内。
- **Quickstart**：Option A CLI（`aiops diagnose` / `aiops cases --query ...`，
  英文输出、`-o json`、exit code 0/1/2）；Option B docker compose。
- **Mermaid 架构图**（第二屏）：多集群 → 网关 → 确定性诊断 → 证据时间线 /
  案例库 / Operator，AI 解释带引用。
- **工程指标**：覆盖率全局 ≥70% / 核心包 ≥75%、lint 0 issues、gosec/trivy
  clean、race 强制、e2e 全绿。
- **Repository layout / Project boundaries / Documentation**：保留关键真实信息，
  完整功能清单不再挤首页（指向 docs/）。

### 3. 未纳入

- 终端演示 GIF（`docs/screenshots/terminal-demo.gif`）由指挥中枢负责录制，
  README 暂不引用未生成文件；现有截图 `01-dashboard.png` 保留。
- 完整里程碑/功能清单 → `docs/`，README 只保留骨架级摘要。

## Verification

- Markdown 结构检查：代码块配平、Mermaid 语法、链接路径均存在
  （docs/README.md、CHANGELOG.md、SECURITY.md、LICENSE、docs/screenshots/01-dashboard.png）。
- 纯文档改动，不触发 Go/前端门禁；后续 CLI commit 会带全量 `go test ./...`。

## Artifacts

- `README.md`（替换）
- `docs/aiops-readme-en-v1.md`（骨架来源，untracked 方案文档，不提交）
- `docs/star-playbook.md`（作战手册，untracked 方案文档，不提交）
