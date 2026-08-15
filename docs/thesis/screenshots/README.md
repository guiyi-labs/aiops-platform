# Thesis Screenshots

本目录保存当前系统自行采集的答辩截图，不包含参考项目图片。截图由采集脚本
`scripts/capture-thesis-screenshots.mjs` 驱动真实浏览器（Edge/Chrome DevTools
Protocol）登录平台并按路由逐页截取，数据全部来自运行中的平台 API，不写库、不伪造页面。

| 文件 | 页面 | 对应演示链路 |
|---|---|---|
| `01-dashboard.png` | Dashboard 集群态势（多集群健康、待处置诊断、资源用量） | 链路一 |
| `02-clusters.png` | 集群接入（demo-kind-capture Ready + v1.36.0 + Condition 状态） | 链路一 |
| `03-workloads.png` | 资源工作台（demo 命名空间 Pod/Deployment 等真实只读资源） | 链路一 |
| `04-diagnoses.png` | 智能诊断（3 条规则诊断：node.not_ready / pod.oom_killed / replicas_unavailable） | 链路一 |
| `05-incidents.png` | 事故工作空间（由诊断提升的事故、SLA/MTTA/MTTR） | 链路二 |
| `06-alerts.png` | 告警规则（规则/接收器/抑制路由配置） | 链路二/三 |
| `07-events.png` | 事件中心（按严重级/原因/资源折叠的趋势与证据深链） | 链路三 |
| `08-slo.png` | SLO 仪表盘（Error Budget 消耗与 Burn Rate 趋势） | 链路三 |

## 数据采集基线（M115 · 2026-08-15）

- **采集时间**：2026-08-15（`capture-metadata.json` 记录 UTC 时间与视口 1440×1000）。
- **源码修订**：`e7daa6fb14f58f90c2860c7d06edb5af7279fb54`（M115 基线 `baseline-m115-20260815`）。
- **平台环境**：本地 k8s-aiops 开发栈（PostgreSQL 17/pgvector、Go 后端、Vue 3 前端），
  三服务 healthy，`AI_ENABLED=false`（确定性规则主链路，无 AI 依赖）。
- **集群数据源**：`demo-kind-capture` 集群对接仓库内置的 `demo-kube-mock`
  （`backend/cmd/demo-kube-mock`，与 M102 demo drill 同一确定性 fixture 服务器，
  HTTPS 自签证书 + `insecure-skip-tls-verify`）。集群对象为固定 fixture
  （`demo-node` NotReady、`demo/demo-pod` OOMKilled、`demo/demo-app` 副本不可用），
  但诊断、事故、处置等平台行为全部经由真实后端 API 产生。
- **演示数据**：3 条真实诊断（node.not_ready.v1 critical、pod.oom_killed.v1 critical、
  deployment.replicas_unavailable.v1 high），其中 pod.oom_killed 已流转为 confirmed，
  并已提升为一条 critical 事故（INC-000004）。
- **测试结果**：截图所依赖的诊断/事故接口为 M115 已归档测试覆盖（后端全绿，
  覆盖率门禁 ≥65%，核心包 ≥70%）；本次采集本身仅涉及只读页面渲染与登录。

> 诚实边界：截图中的集群对象来自确定性 mock fixture，不是真实 kind 集群；
> 与 `docs/thesis/defense-demo-script.md` 所述"真实 kind 集群"现场演示并不矛盾——
> 现场演示使用真实集群，截图用于静态存档与答辩展示，数据源在本文档明确标注。

## 重新采集

```bash
# 1. 平台栈运行中（docker compose up --build，前端 18080 / 后端 8080）
# 2. 启动内置 mock 集群并注册（可选：保留 .artifacts/demo/demo-ready-*.json 证据）
docker run -d --name demo-mock --network k8s-aiops_default k8s-aiops-demo-mock:latest -listen 0.0.0.0:8443
# 注册集群（POST /api/v1/clusters，name=demo-kind-*，kubeconfig server=https://demo-mock:8443 + insecure-skip-tls-verify）
# 触发诊断（POST /api/v1/clusters/{id}/diagnoses 对 demo-node / demo-pod / demo-app）

# 3. 采集（macOS / Windows 均可用 Node + Chrome/Edge）
AIOPS_CAPTURE_PASSWORD='<password>' AIOPS_CAPTURE_USERNAME=admin \
AIOPS_CAPTURE_WEB_BASE=http://127.0.0.1:18080 \
AIOPS_BROWSER_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
node scripts/capture-thesis-screenshots.mjs
```

采集器使用系统已安装的 Chrome/Edge 和标准 DevTools Protocol，不安装浏览器依赖。
密码只进入临时进程环境和登录表单内存；浏览器 profile 位于已忽略的 `.artifacts`，
采集结束后自动删除。
