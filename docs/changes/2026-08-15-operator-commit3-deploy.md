# Operator/CRD 增强（Commit 3）：cmd 入口 + deploy 集成

- Date: 2026-08-15
- Status: Complete
- Scope: 战略任务「Operator/CRD 增强」第三个提交：deploy 集成与 cmd 入口。

## What Changed

- `backend/cmd/controlled-operation-operator/main.go`：纯 client-go 启动入口。
  dynamic informer 监听 ControlledOperation CRD；add/update/delete → Enqueue；
  cache sync → worker loop（可配置 `--workers`，默认 1，`--resync` 默认 30m）；
  kubeconfig 支持 flag / 环境变量 / in-cluster 三级降级；优雅关闭（SIGINT/SIGTERM）。

- `deploy/kubernetes/operator.yaml`：Operator 全套部署清单
  - ServiceAccount（`controlled-operation-operator`，namespace `aiops-system`）
  - ClusterRole：CRD get/list/watch/update/patch + target Deployment get/patch +
    target CronJob get/patch（最小权限）
  - ClusterRoleBinding
  - Deployment（`--workers=1`，readOnlyRootFilesystem，drop ALL capabilities）

- `deploy/kubernetes/crds/controlledoperations.aiops.platform.yaml`：CRD v1 定义
  （status subresource、additionalPrinterColumns、enum 校验、dryRun 默认 true）。

- `deploy/kubernetes/kustomization.yaml`：新增 `crds/` 和 `operator.yaml`。

- `deploy/helm/aiops-platform/crds/controlledoperations.aiops.platform.yaml`：
  Helm chart 的 CRD（沿用 `crds/` 惯例，不模板化，升级时先于 resources）。

- `deploy/helm/aiops-platform/templates/operator.yaml`：Helm operator 模板
  （ServiceAccount + ClusterRole + ClusterRoleBinding + Deployment，
  使用 `aiops-platform.labels` / `aiops-platform.namespace` helper，
  `automountServiceAccountToken: true` 因 operator 需 API server 访问）。

- `deploy/helm/aiops-platform/values.yaml`：新增 `operator:` 段
  （image、workers、resources、securityContext、podSecurityContext）。

## Verification

- `go build ./cmd/controlled-operation-operator/` + `go vet`：通过。
- `go test ./internal/operator/`：15 用例全绿。
- deploy 清单用 Go sigs.k8s.io/yaml 逐文件解析（非模板）：所有 YAML 结构合法。
- helm template 语法由 CI `.tools-ci/helm lint --strict` 覆盖。

## Risks / Notes

- `automountServiceAccountToken: true`（operator SA）：operator 需 watch/patch CRD
  与目标资源，必须挂载 token，与 backend SA（`false`）语义不同，符合最小权限。
- 原因：operator 是独立控制面二进制，不是无头 sidecar。
- CRD Helm 用 `crds/` 惯例（Helm 自动在 release 创建时安装，升级时不删除），
  与平台现有 Kustomize 双路径一致。
