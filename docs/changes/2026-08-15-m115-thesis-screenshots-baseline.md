# M115 答辩截图基线：论文截图刷新至 M115 最终基线

- Date: 2026-08-15
- Status: Complete
- Scope: 论文/答辩截图重新采集（8 页）、采集脚本预期文案更新、截图与演示文档同步

## Context

Obsidian 知识库「GitHub 项目展示优化与后续路线」P0 清单要求：
将论文截图的采集日期、commit、环境和测试结果更新为最终答辩基线（M115）。
旧截图采集于 2026-07-26 / `uncommitted-baseline`，且仅 4 页，与 M115 UI
（Dashboard 改版为集群态势、新增事故/事件/SLO 等链路）不一致；采集脚本中的
预期文案也已过时（`累计规则命中` 已不存在于当前 UI）。

## What Changed

### 采集脚本
- `scripts/capture-thesis-screenshots.mjs`：更新 4 个现有页面预期文案（Dashboard
  `累计规则命中`→`集群态势`、Clusters `demo-kind-`→`集群接入`、Workloads
  `工作负载`→`资源工作台`、Diagnoses 保持 `智能诊断`），并新增 4 页
  （Incidents `事故工作空间`、Alerts `告警规则`、Events `事件中心`、SLO `SLO 仪表盘`），
  共 8 页覆盖三条答辩演示链路。

### 截图资产
- `docs/thesis/screenshots/01-dashboard.png` … `08-slo.png`：按 M115 基线重新采集
  （视口 1440×1000，登录真实平台后逐路由截图）。
- `docs/thesis/screenshots/capture-metadata.json`：更新为
  `captured_at=2026-08-15T07:16:26.328Z`、`source_revision=e7daa6fb14f58f90c2860c7d06edb5af7279fb54`
  （M115 基线 HEAD）、8 页路由清单。

### 文档
- `docs/thesis/screenshots/README.md`：重写为 M115 采集基线说明（数据源
  `demo-kube-mock` 确定性 fixture、平台真实 API、诊断/事故演示数据、重新采集步骤、
  诚实边界声明）。
- `docs/thesis/demo-environment.md`：更新时间戳至 M115，补充 macOS/Node 采集方式与
  截图基线说明。

### 本地证据（不入库，.artifacts 已忽略）
- `.artifacts/demo/demo-ready-20260815-thesis-baseline.json`：本次演示环境与截图
  采集证据（集群、诊断、事故、截图元数据）。

## Verification

- 采集脚本 8 页全部 `Captured` 成功，页面文案断言全部命中（登录 → 集群态势 →
  集群接入 → 资源工作台 → 智能诊断 → 事故工作空间 → 告警规则 → 事件中心 → SLO 仪表盘）。
- 平台真实数据验证：集群 `demo-kind-capture` status=ready、kubernetes_version=v1.36.0；
  3 条诊断（node.not_ready.v1 critical / pod.oom_killed.v1 critical[confirmed] /
  deployment.replicas_unavailable.v1 high）；事故 INC-000004 critical 已建立。
- 截图 PNG 尺寸/内容核验通过（8 个文件，139–209 KB）。

## Risks / Notes

- 截图集群数据源为仓库内置 `demo-kube-mock`（确定性 fixture，HTTPS 自签 +
  insecure-skip-tls-verify），与现场演示的真实 kind 集群不同；README 已明确标注，
  不把 fixture 描述为真实集群。
- 现场正式答辩截图若需真实集群版本，可在真实环境重跑 `demo-up.ps1` + 采集脚本。
