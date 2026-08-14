# Lifecycle Boundaries：明确三个基础设施项目的 Day 0/1/2 职责

- Date: 2026-08-14
- Status: Complete
- Scope: 对齐 `kubernetes-cluster-bootstrap`、`devops-automation` 和 `aiops-platform` 的公开项目边界。

## Context

三个项目都涉及 Linux、容器和 Kubernetes。若不明确生命周期入口，后续实现容易重复建设，访客也
难以判断哪个项目负责建集群、哪个项目负责主机运行期、哪个项目负责 Kubernetes 运行期。

## What Changed

### AIOps 项目边界

- `README.md`：新增 Day 0/1/2 边界表，明确本项目从已有 Kubernetes 集群开始。
- `README.md`：明确集群创建、操作系统初始化、containerd、kubeadm、CNI 和控制平面 HA 不属于 AIOps 范围。

### 架构决策

- `docs/adr/0088-lifecycle-boundaries.md`：记录三个项目的职责、衔接方式和不重复建设原则。

## Verification

- `git diff --check`：通过。
- README 边界检查：确认包含 `kubernetes-cluster-bootstrap`、`devops-automation`、Day 0/1/2 和
  `kubeadm init/join` 的明确声明。
- 本次仅修改文档，不改变代码、接口、数据库或部署行为；未运行代码测试。

## Risks / Notes

- Bootstrap 当前仍是教程与自动化方案基线，不能因边界声明而被理解为已经具备生产级 Ansible 实现。
- 后续新增功能必须先判断属于集群交付、Linux 主机运行期还是 Kubernetes 运行期，并在对应仓库落地。
