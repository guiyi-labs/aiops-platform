# Verification Environment

采集时间：2026-07-27，时区：Asia/Shanghai。

## 宿主机

| 项目 | 当前值 |
|---|---|
| 操作系统 | Microsoft Windows 11 Pro 64-bit，10.0.26200 |
| CPU | 12th Gen Intel Core i5-1240P，12 核 / 16 逻辑处理器 |
| 内存 | 15.7 GB |
| 项目目录 | `E:\k8s\毕设\aiops-platform` |
| Docker Desktop 数据目录 | `E:\DockerData\wsl\disk` |

## 工具链

| 组件 | 版本/说明 |
|---|---|
| Docker Engine | 29.6.1 |
| Docker Compose | 5.2.0 |
| Go | 项目声明 1.25；正式构建与门禁使用 `golang:1.25-alpine` |
| 本地备用 Go | 1.24.4，仅保留在 `.tools`，版本低于项目声明时不得用于正式门禁 |
| Node.js | v24.14.0 |
| pnpm | 宿主机 11.9.0；`package.json` 声明 11.7.0，前端镜像安装 11.7.0 |
| kubectl / Kustomize | v1.36.1 / v5.8.1 |
| kind | v0.30.0（项目本地工具） |

## 运行组件

| 组件 | 版本/地址 |
|---|---|
| Frontend | Nginx 容器，`http://localhost:18080` |
| Backend | Go API，`http://localhost:8080` |
| PostgreSQL | PostgreSQL 17.8 + pgvector 0.8.1，宿主端口 15432 |
| Real kind | context `kind-aiops-test`，Kubernetes v1.34.0 |
| Metrics Server | v0.8.0，仅安装在保留的 kind 验收环境 |
| 平台演示集群 | `demo-kind-20260727-165016`，cluster ID 39，短期凭据；M18 保留验收环境 |

Windows 当前保留 TCP 端口区间 `5139-5238`，因此前端宿主端口从常见的 5173 调整为 18080。容器内端口和 `/api/` 反向代理契约未变化。

## 资源控制

- Docker 可用内存有限，验收期间只保留 Compose 三服务和一个轻量 kind 集群，不额外部署 Prometheus 全家桶。
- Kubernetes 平台清单为容器设置 requests/limits、探针、非 root 安全上下文和 NetworkPolicy。
- kind 演示工作负载统一位于 `aiops-demo`，验证后可用 `-CleanupDemoResources` 清理。

## 可复现入口

```powershell
# 全量代码、构建、Compose、Kustomize 和 HTTP 健康门禁
.\scripts\verify.ps1

# 真实 kind 诊断与受控处置闭环
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
.\scripts\e2e-kind.ps1

# 安装可选 Metrics Server，执行完整真实链路并保留演示集群
.\scripts\e2e-metrics-kind.ps1 -KeepPlatformCluster

# 更新依赖许可证清单
.\scripts\generate-license-report.ps1
```

生成物只进入 `.artifacts/` 或 `docs/thesis/dependency-licenses.md`。kubeconfig、ServiceAccount token、管理员密码和 Cookie 不进入归档。
