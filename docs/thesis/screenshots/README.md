# Thesis Screenshots

本目录保存当前系统自行采集的答辩截图，不包含参考项目图片。

| 文件 | 页面 | 数据来源 |
|---|---|---|
| `01-dashboard.png` | Dashboard 总览、集群数和诊断统计 | kind 集群 demo-kind-cluster (v1.34.0) |
| `02-clusters.png` | Clusters 集群 Ready 状态与 Kubernetes 版本 | 真实 kind 集群 |
| `03-workloads.png` | Workloads 真实 Namespace/Pod/Service 视图 | prod/staging 命名空间 6 个 Deployment |
| `04-diagnoses.png` | Diagnoses 三类规则历史与处置状态 | 4 条真实诊断 (OOMKilled/CrashLoopBackOff/ReplicasUnavailable) |
| `capture-metadata.json` | 采集时间、视口、路由和源码修订状态 | — |

## 数据采集环境（2026-08-01 采集）

- **集群**：kind v0.30.0 单节点 (kindest/node:v1.34.0)，通过 `host.docker.internal` 从后端 Docker 容器接入
- **工作负载**：3 个命名空间 (prod/staging/monitoring)，6 个 Deployment (api-gateway 3副本、payment-service 2副本、user-center 2副本、order-processor 3副本、inventory-sync 1副本、notification-worker 2副本)
- **故障注入**：broken-service (CrashLoopBackOff, exit 1) 和 memory-hog (OOMKilled, 256M stress)
- **诊断规则**：4 条诊断记录 — 1 critical (pod.oom_killed.v1) + 1 high (pod.crash_loop_backoff.v1) + 2 high (deployment.replicas_unavailable.v1)

重新采集：

```powershell
# 1. 创建 kind 集群并部署 demo 工作负载
kind create cluster --name aiops-demo --config deploy/kind/dev-cluster.yaml --wait 120s
kubectl --context kind-aiops-demo apply -f deploy/kind/demo-workloads.yaml

# 2. 启动平台 (docker compose up)

# 3. 注册集群到平台 (需将 kubeconfig 中 127.0.0.1 替换为 host.docker.internal 并设置 insecure-skip-tls-verify)

# 4. 触发诊断
# POST /api/v1/clusters/{id}/diagnoses 对 broken-service 和 memory-hog

# 5. 截图
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\capture-thesis-screenshots.ps1
```

采集器使用系统已安装的 Microsoft Edge 或 Google Chrome 和标准 DevTools Protocol，不安装浏览器依赖。密码只进入临时进程环境和登录表单内存；浏览器 profile 位于已忽略的 `.artifacts`，采集结束后删除。
