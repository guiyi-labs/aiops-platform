# aiops-platform Operator/CRD 增强 —— 实施方案与进度

- Date: 2026-08-15
- Status: ✅ 已完成并推送（4 + 1 个提交，HEAD `fa03612`）
- 基线: main @ `fa03612`（毕设清理版；M115 收口完成，门禁 70% 已上调）
- 关联: 指挥中枢战略任务「Operator/CRD 增强」（K8s 求职加分项 + client-go 深度证据）

---

## 1. 方案摘要

### 1.1 CRD 设计：`ControlledOperation`

- 选定 `ControlledOperation`（非 `DiagnosisRule`）：
  - 直接映射平台既有「受控操作目录」语义（dry-run + 确认 + 幂等 + 审计，见 `internal/remediation` / `internal/automation`）
  - 改动小、可验证、不触碰旗舰主链路；`DiagnosisRule` 会把现有规则执行强耦合进 K8s 事件循环，属于大手术
- group/version: `aiops.platform/v1`；namespaced；kind `ControlledOperation`
- Spec:
  - `action`：`deployment.rollout_restart` / `deployment.scale` / `cronjob.suspend`（枚举校验）
  - `targetKind` / `targetNamespace` / `targetName`
  - `dryRun`（默认 true）、`idempotencyKey`（可选，幂等去重）
- Status: `phase`（Pending → Reconciling → Succeeded | Failed）、`attempts`、`observedGeneration`、`lastMessage`

### 1.2 Controller 技术选型：纯 client-go informer + cached typed client

- **不用 controller-runtime**：仓库 `backend/go.mod` 已含 `client-go v0.36.3` / `apimachinery`，纯 client-go 零新增重依赖、可读、易单测
- 形态：`backend/cmd/controlled-operation-operator` 独立小命令（沿用 `backend/cmd/*` style），与主进程解耦
- reconcile 语义：informer watch `ControlledOperation` → 校验 action/字段 → 经既有 `kubernetes.Service`（dry-run 语义）执行 → 更新 status；finalizer 保证幂等清理

### 1.3 RBAC / 部署 / 文档

- RBAC：最小权限（对应 CRD 的 get/list/watch/update + patch）
- 部署：Kustomize（`deploy/kubernetes/`）+ Helm `crds/` 双体系沿用
- 文档：README 一节「为什么做 / 怎么运行 / 验证了什么」，突出 K8s 深度

### 1.4 测试策略

- 单元测试（主）：reconcile 逻辑用 `k8s.io/client-go/kubernetes/fake` + informer 测试
  - 覆盖：成功路径、unsupported action 拒绝、dry-run、幂等（同 idempotencyKey 不重放）、finalizer 清理、observedGeneration 更新
  - 基建先例：`internal/kubernetes/api_resources_test.go` 已用 `kubernetes/fake`
- kind 真实验证（可选执行）：kind/kubectl 已装但当前无集群；可临时 `kind create cluster` → 应用 CRD+controller+示例对象 → 观察 status → 删除
  - **实现真实验证才写「已验证」，否则写「单元测试（fake client）」**

---

## 2. 当前进度

| 阶段 | 状态 |
|---|---|
| 方案评估（依赖/架构/测试基建/kind 可用性） | ✅ 已完成，无阻塞 |
| CRD yaml + Go types + informer/typed client + fake 单测 | ✅ `25ebd03` |
| reconcile controller 逻辑 + 单测 | ✅ `018d67d` |
| deploy 集成（Kustomize + Helm crds/ + RBAC） | ✅ `df85e6b` |
| README 一节 + 示例 yaml + CHANGELOG + 归档 | ✅ `0d6823b` |
| gofmt 格式化 | ✅ `fa03612` |
| （可选）kind 真实验证记录 | 未执行（单元测试为当前证据边界，未写「已验证」） |

- 产出 commit：5 个（`25ebd03` → `018d67d` → `df85e6b` → `0d6823b` → `fa03612`），
  均 noreply 身份推送；远端 `fa03612`，工作树干净（本文件除外）。

## 3. 需要指挥中枢的下一步指示

- ✅ 已按方案执行完毕。下一步可选项：
  - `kind 真实验证`：本机 kind/kubectl 可用，如需真实集群证据可批准执行
    （临时 kind 集群 → 安装 → 观察 status → 清理），完成后补充验证记录；
  - `补充证据`：如需 controller 运行日志/截图等展示材料。

**硬性约束**（确认时默认携带）：noreply 提交；不破坏 M115 收口、不碰 `docs/thesis` 及 ignore 守卫；不写无法验证的「生产级 / 已验证」表述；完成推送后回报 commit SHA + 验证证据。