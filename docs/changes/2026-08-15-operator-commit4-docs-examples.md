# Operator/CRD 增强（Commit 4）：README 一节 + 示例 + CHANGELOG

- Date: 2026-08-15
- Status: Complete
- Scope: 战略任务「Operator/CRD 增强」收尾提交：文档与示例。

## What Changed

- `README.md`：新增「ControlledOperation Operator（K8s Controller 深度实践）」
  一节 —— 为什么做 / 架构（纯 client-go、CRD、Reconciler 可单测、RBAC 最小
  权限）/ 运行与验证（kustomize/helm 双路径 + dry-run 示例）/ 验证了什么
  （15 个 fake client 单测 + 清单校验；kind 真实验证为可选后续，未做不写
  「已验证」）；Repository Layout 补充 operator 目录与 crds/、examples/。

- `deploy/kubernetes/examples/`：3 个 dry-run 示例
  - `deployment_rollout_restart.yaml`（dryRun: true + idempotencyKey）
  - `deployment_scale.yaml`（desiredReplicas: 2）
  - `cronjob_suspend.yaml`

- `CHANGELOG.md`：Unreleased 汇总四个 operator 提交。

## Verification

- `go test ./internal/operator/`：15 用例全绿；`go build` / `go vet` 通过。
- 部署清单结构校验通过。

## Risks / Notes

- README 明确写入「kind 真实验证为后续可选步骤，未做不写已验证」，
  保持「真实验证才写已验证」的口径一致。
