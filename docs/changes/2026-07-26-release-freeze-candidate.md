# 变更归档：发布冻结候选

- 日期：2026-07-26
- 阶段：RC-Freeze（发布冻结候选）
- 结果：验证通过，等待人工确认 Git 作者身份和初始提交范围

## 阶段结论

M1 基础运行、M2 多集群资源读取、M3 确定性诊断、M4 AI/审计/通知/受控处置、M5 交付包装和 M6 答辩演示环境均已完成。本阶段将可运行基线整理为可复现的发布候选，并补齐交接所需的证据索引。

## 冻结动作

- 重新执行 `scripts/verify.ps1`，确认后端、前端、Compose、Kustomize 和 HTTP 健康检查仍然通过。
- 运行源码与文档敏感材料扫描，确认未发现私钥、长令牌、JWT、Bearer 凭据或明文密码。
- 保留当前答辩演示环境，未清理演示集群、诊断记录和受控处置记录。
- 归档机器验证证据到被忽略的 `.artifacts`，不将 kubeconfig、Token、Cookie、CA 数据写入仓库。

## 验证证据

最新质量门禁：`.artifacts/verification/verify-20260726-180631.json`

- Go 使用 `golang:1.25-alpine` 容器执行 `go vet`、全包测试和构建，全部通过。
- 前端 typecheck、8 个 Vitest 文件和 26 个测试、生产构建全部通过。
- Compose 的 PostgreSQL、backend、frontend 均为 `healthy`。
- Kustomize 渲染资源数量为 `16 / 5 / 7`，分别对应平台、受管集群 RBAC 和演示场景。
- 后端 readiness、前端页面和前端 API proxy 均返回预期结果。
- 敏感材料扫描结果：`no matches`。

## 当前演示状态

- Web UI：`http://localhost:18080`
- Backend：`http://localhost:8080`
- PostgreSQL：`localhost:15432`
- kind 集群：`demo-kind-20260726-170601`
- 数据状态：1 个 Ready 集群、3 条诊断记录、1 条成功且幂等重放过的 remediation 记录。

演示 ServiceAccount Token 仅保留约一小时。正式答辩前应重新执行 `scripts/demo-up.ps1`，答辩结束执行 `scripts/demo-down.ps1`。

## 交付文档

- 交接入口：`docs/development-handoff.md`
- 论文材料索引：`docs/thesis/README.md`
- 阶段变更索引：`docs/README.md`
- 总体开发计划：`docs/development-handoff.md`
- 答辩截图：`docs/thesis/screenshots/`

## 待人工确认事项

1. 配置 `git user.name` 和 `git user.email`。
2. 确认将当前完整未提交基线作为初始提交范围。
3. 创建 baseline commit 后重新执行截图采集，使 `capture-metadata.json` 记录真实 revision。
4. 根据答辩或部署需要创建 release tag。

在上述确认完成前，仓库保持无初始 Git commit 的状态，不影响本地运行、验证和论文材料使用。
