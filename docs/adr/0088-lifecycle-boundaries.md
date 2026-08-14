# ADR 0088 — Infrastructure 运维项目的 Day 0/1/2 生命周期边界

- Date: 2026-08-14
- Status: Accepted
- Related repositories: `kubernetes-cluster-bootstrap`, `devops-automation`, `aiops-platform`

## Context

三个基础设施项目都涉及 Linux、容器和 Kubernetes。如果没有明确的生命周期入口，集群创建、
主机运维和 Kubernetes 运行期诊断会逐渐重复，README、演示和后续实现也难以说明各自的价值。

## Decision

按运行生命周期划分职责：

1. `kubernetes-cluster-bootstrap` 负责 Day 0/Day 1 集群交付：节点预检、容器运行时、kubeadm、
   CNI、控制平面 HA、安装后验收和交接材料。
2. `devops-automation` 负责 Linux 主机运行期：SSH、systemd、进程、磁盘、批量任务、备份、
   Docker 主机和主机监控。Kubernetes 只作为它自身的可选部署环境。
3. `aiops-platform` 负责 Kubernetes Day 2 运行期：多集群、工作负载、指标/日志/事件、SLO、
   证据型诊断、事故响应和受控修复。

`aiops-platform` 接收已经创建且可访问的集群，不承担 `kubeadm init/join`、操作系统初始化、
`containerd` 安装、CNI 安装或控制平面 HA 交付。三者可以通过 kubeconfig、集群注册信息、验收
报告和脱敏演练证据衔接，但不得共享真实凭据或互相复制控制台能力。

## Consequences

- 每个项目都有独立的入口、用户问题和演示场景，减少重复建设。
- Bootstrap 可以先作为教程/方案基线，再逐步重建为真实 Ansible 交付；不会被 AIOps 的运行期功能掩盖。
- EasyOps 的 Kubernetes 清单只证明它自身可部署，不暗示它管理 Kubernetes 集群。
- 跨项目 README、Obsidian 路线和简历描述必须保持 Day 0/1/2 术语一致。
