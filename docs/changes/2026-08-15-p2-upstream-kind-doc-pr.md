# P2：向 kind 提交离线镜像镜像源文档 PR

- Date: 2026-08-15
- Status: Complete
- Scope: 向相关开源项目 `kubernetes-sigs/kind` 提交一项小型、可复核的离线文档改进

## Context

Obsidian「GitHub 项目展示优化与后续路线」P2 清单要求向相关开源项目提交小型文档、
测试或修复 PR，形成真实协作证据。当前项目在受限网络环境中实际遇到 Docker Hub
不可达、但镜像代理可用的 kind 节点镜像获取场景；kind 现有离线指南只覆盖
`docker save`/`docker load`，没有说明镜像源代理与本地重新打标签的流程。

## What Changed

- 上游项目：`kubernetes-sigs/kind`
- PR：[#4234](https://github.com/kubernetes-sigs/kind/pull/4234)
- Fork 分支：`guiyi-labs:docs/working-offline-mirror-registry`
- PR 当前 head：`6edff9221726de2a89c10c7ad400cea196000a84`（首个文档提交
  `c250ccd11280fdea973a6200e1c388fc23d88eac`，后续提交将示例改为组织批准的通用镜像源）。
- 改动：在 `site/content/docs/user/working-offline.md` 增加 mirror registry 小节，
  说明从组织批准的镜像源拉取 `kindest/node`、重新标记为 `kindest/node:<tag>`，
  以及使用 digest 固定引用创建集群。
- DCO：两笔提交均包含 `Signed-off-by: Guiyi Labs <277616126+guiyi-labs@users.noreply.github.com>`。

## Verification

- PR 差异确认仅包含 `site/content/docs/user/working-offline.md` 一个文档文件。
- 上游 PR 当前为 `OPEN`；Header rules、Pages changed、Redirect rules 检查已完成，
  EasyCLA 与 Netlify 检查等待上游环境执行。
- 命令序列已在本地受限网络环境实际执行：从组织批准的镜像源拉取
  `kindest/node:v1.34.0` 并重新标记，digest 与预期一致；本机后续 kind kubeadm
  阶段因 2 CPU/1.9 GiB 资源限制失败，未将该失败误报为文档功能通过。

## Risks / Notes

- 文档示例使用 `registry.example.com` 占位符；用户应替换为所在组织批准的镜像源，
  不应把特定镜像源写成 kind 的默认依赖。
- PR 是否合并由上游维护者决定；本项目将“已提交 PR”作为 P2 协作证据，后续审查结果
  继续在 PR 页面跟踪。
