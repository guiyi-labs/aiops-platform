# Operator/CRD 增强（Commit 1）：ControlledOperation 类型 + CRD 定义 + typed client

- Date: 2026-08-15
- Status: Complete
- Scope: 战略任务「Operator/CRD 增强」第一个提交：CRD 类型层。

## Context

指挥中枢批复战略任务：为 aiops-platform 增加真实可运行的 K8s Controller
（Operator 模式），作为 K8s/云原生运维求职的深度证据与 client-go 实战。
方案选型：`ControlledOperation` CRD + 纯 client-go（informer + dynamic
typed wrapper），零新增重依赖（client-go v0.36.3 已在 go.mod）。

## What Changed

- `backend/internal/operator/types.go`：ControlledOperation/Spec/Status/List
  类型 + DeepCopy + runtime.Scheme 注册（aiops.platform/v1）；固定 action
  目录（deployment.rollout_restart / deployment.scale / cronjob.suspend）；
  阶段枚举（Pending/Reconciling/Succeeded/Failed）；dryRun 默认 true；
  idempotencyKey 幂等语义。
- `backend/internal/operator/client.go`：Client 接口（Get/UpdateStatus）+
  dynamic.Interface 适配实现 + Unstructured↔typed 转换 + AsGVR 目标映射。
- `deploy/kubernetes/crds/controlledoperations.aiops.platform.yaml`：
  CRD v1 定义（namespaced、status subresource、additionalPrinterColumns、
  enum 校验、dryRun 默认语义说明）。

## Verification

- `go test ./internal/operator/`：7 个用例全绿（deepcopy 深拷贝指针隔离、
  unstructured round-trip、scheme 注册、AsGVR、dryRun 默认、nil 守卫）。

## Risks / Notes

- 纯 client-go，不引入 controller-runtime；controller/reconcile 逻辑在
  Commit 2 落地。
