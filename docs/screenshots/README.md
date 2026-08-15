# Platform Screenshots

本目录保存当前系统自行采集的平台功能截图，不包含参考项目图片。截图用于 README 展示与功能验证。
截图数据全部来自运行中的平台 API，不写库、不伪造页面。

| 文件 | 页面 | 展示内容 |
|---|---|---|
| `01-dashboard.png` | Dashboard 集群态势 | 多集群健康、待处置诊断、资源用量 |
| `02-clusters.png` | Clusters 集群接入 | demo-kind-capture Ready + v1.36.0 + Condition 状态 |
| `03-workloads.png` | 资源工作台 | 演示命名空间 Pod/Deployment 等只读资源 |
| `04-diagnoses.png` | Diagnoses 智能诊断 | 规则诊断：node.not_ready / pod.oom_killed / replicas_unavailable |
| `05-incidents.png` | 事故工作空间 | 由诊断提升的事故、SLA/MTTA/MTTR |
| `06-alerts.png` | 告警规则 | 规则/接收器/抑制路由配置 |
| `07-events.png` | 事件中心 | 按严重级/原因/资源折叠的趋势与证据深链 |
| `08-slo.png` | SLO 仪表盘 | Error Budget 消耗与 Burn Rate 趋势 |

## 数据采集基线（M115 · 2026-08-15）

- **采集时间**：2026-08-15（`capture-metadata.json` 记录 UTC 时间与视口 1440×1000）。
- **平台环境**：本地 k8s-aiops 开发栈（PostgreSQL 17/pgvector、Go 后端、Vue 3 前端），
  三服务 healthy，`AI_ENABLED=false`（确定性规则主链路，无 AI 依赖）。
- **集群数据源**：`demo-kind-capture` 集群对接仓库内置的 `demo-kube-mock`
  （`backend/cmd/demo-kube-mock`，确定性 fixture 服务器，HTTPS 自签证书 +
  `insecure-skip-tls-verify`）。集群对象为固定 fixture
  （`demo-node` NotReady、`demo/demo-pod` OOMKilled、`demo/demo-app` 副本不可用），
  但诊断、事故、处置等平台行为全部经由真实后端 API 产生。
- **演示数据**：3 条真实诊断（node.not_ready.v1 critical、pod.oom_killed.v1 critical、
  deployment.replicas_unavailable.v1 high），其中 pod.oom_killed 已流转为 confirmed，
  并已提升为一条 critical 事故（INC-000004）。

> 诚实边界：截图中的集群对象来自确定性 mock fixture，不是真实 kind 集群；
> 现场演示使用真实集群，截图用于静态展示。

## 重新采集

```bash
# 1. 平台栈运行中（docker compose up --build，前端 18080 / 后端 8080）
# 2. 启动内置 mock 集群并注册
# 3. 采集（macOS / Windows 均可用 Node + Chrome/Edge）
AIOPS_CAPTURE_PASSWORD='<password>' AIOPS_CAPTURE_USERNAME=admin \
AIOPS_CAPTURE_WEB_BASE=http://127.0.0.1:18080 \
AIOPS_BROWSER_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
node scripts/capture-ui-baselines.mjs --out docs/screenshots
```
