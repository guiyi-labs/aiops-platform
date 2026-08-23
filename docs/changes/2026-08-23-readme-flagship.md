# README 旗舰重写（P2b）

- Date: 2026-08-23
- Status: Complete
- Scope: 对应 `docs/enhancement-p2-flagship-roadmap.md` §3 P2b — README 达到「旗舰仓库」观感

## Context

仓库功能已完成 P1 RAG（`knowledge` 案例库 + `aiexplain` 引用校验 + `aiopsbench` 基准），但
README 仍停留在 M115 基线的纯文字版：架构碎片化（`flowchart LR` 只有 6 节点）、徽章行形容词化
（`coverage-≥70%` 未区分全局/核心门禁）、缺少可复现的演示入口与路线图索引，求职/Star 场景不具
“旗舰样例”说服力。

## What Changed

- `README.md` 重写为旗舰版（149 → ~160 行，结构化 12 节）：
  - 架构图细化：补齐「只读有界网关 → 确定性诊断 → 证据时间线 → 案例库(RAG) → 引用式 AI → 控制台/CLI」
    与「B-tree + LLM 重排」检索标注、Operator `dry-run → confirm → idempotent` 链路。
  - 徽章行精确化：`coverage-≥70% (global)/≥75% (core)`，与 CI 真实门禁一致。
  - 快速上手补齐 Option C（`pwsh -File scripts/demo-up.ps1` 端到端 kind 演示）。
  - 新增「演示脚本」矩阵（`demo-up/down`, `verify-fast`, `aiopsbench`, `load-probe`）与语料契约说明。
  - 新增「工程信号」章节（`71 internal/ 包 · 12 binaries` 等可量化事实）与「路线图」索引
    （P2 旗舰深化 + Post-M110 产品轨）。
- `CHANGELOG.md` 新增 Unreleased 小节 `Added - README 旗舰重写（P2b）`。

## Verification

- `pwsh -File scripts/verify-fast.ps1` 全绿（Backend + Frontend + Manifests）。
- README 中出现的脚本路径（`scripts/demo-up.ps1`, `backend/cmd/aiopsbench`, `scripts/load-probe.mjs`）均已
  在工作树可验证存在；覆盖率徽章数值与 `.github/workflows/ci.yml` 门禁一致
  （`grep coverage .github/workflows/ci.yml` → 70.0% global / 75.0% core）。

## Risks / Notes

- RAG 演示的具体 seed 脚本与只读端点（P2c）尚未落地，README 仅索引位置，未宣称已可用。
- Federation 聚合视图（P2d）仍为规划态，README 仅通过路线图链接指向规划文档。
