# M102 测试矩阵（Test Matrix）

- Date: 2026-08-12
- Status: RC 基线证据汇编（M102 本地轨道）
- 适用范围：`v0.3.0-rc.4` / 本地 HEAD `ae65c90`（tag `baseline-m102-demo-drill-20260812`）
- 声明边界：以下矩阵汇总**当前可验证的证据**。真实组织 kind/Helm 生命周期、WAL/PITR、OIDC/MFA 验收（M89/M90 授权轨）未完成前，版本保持 RC，本矩阵不构成 GA 声明。

## 1. 分层概览

| 层 | 门禁/命令 | 规模 | 最近证据 |
|---|---|---|---|
| Go 单元/集成 | `go test ./...`（`backend/`，含 PostgreSQL、k8s fake client、HTTP 契约） | 74 包 / 205 个 `*_test.go` | 全通过（含 `cmd/demo-kube-mock`） |
| 覆盖率 | CI 全局 ≥ 60%，核心包 ≥ 70%（fail-closed） | 全局 60.03%（本地报告） | `docs/changes/2026-08-12-m100d-*` 复验 |
| 契约 | OpenAPI ↔ Gin 路由双向比对（`TestRegisteredRoutesMatchOpenAPI`）、`pnpm typegen` sync gate | 全量路由 | CI `ci.yml` |
| 前端单元 | `pnpm test`（vitest） | 25 个 `*.test.ts` | CI 全绿 |
| 类型/构建 | `pnpm typecheck`（vue-tsc）、`pnpm build`、bundle gate | 全量 | CI 全绿 |
| 浏览器 | Playwright Desktop Chrome + Mobile（Pixel 7）双视口 | 5 个 spec：smoke 13 + incidents 4 + diagnosis-timeline 4 + finding-evidence 3 + a11y 2 = 26 用例 | CI 全绿（42/42 双视口回归基线，M93-C） |
| 无障碍 | axe（`e2e/a11y.spec.ts`） | 双视口 0 critical/serious | CI 全绿 |
| 静态安全 | `scripts/scan-sensitive-fields.sh`、govulncheck、pnpm audit、license、镜像基础层、SBOM 差异 | 1188 个 tracked 文件 | clean（含 M102 demo 提交后） |
| 真实集群 | 可弃 kind e2e（Windows CI / 授权环境） | M21–M31、M46–M60、诊断、联邦、全局搜索、备份恢复 | CI `real-kind-e2e.yml` |
| 发布供应链 | `scripts/release-verify.ps1`、SHA256SUMS、cosign fail-closed、`release-manifest.mjs` | 20 资产 prerelease `v0.3.0-rc.4` | CI `release.yml` |
| 本地演练（离线） | 见下节 | 见下节 | `.artifacts/*/report-*.json` |

## 2. 本地确定性演练（M101/M102/M89 本地轨道）

| 演练 | 脚本 | 场景数 | 覆盖 | 报告 |
|---|---|---|---|---|
| WAL/PITR 本地数据轨 | `scripts/wal-pitr-drill.sh` | 8 | 无损 PITR、时间点恢复、缺 WAL 快速失败、迁移前逻辑备份、SIGKILL 崩溃注入、流式备库/故障切换、网络分区重连、归档目标故障排空 | `.artifacts/wal-pitr-drill/`（连续两轮一致） |
| 双全新环境安装 | `scripts/dual-env-compose-drill.sh` | 10（环境 a/b 各 5） | install、关键旅程、数据持久化、清理 | `report-20260812-213243-bf91f0.json` |
| 跨 digest 升级/回滚 | 同上（`APP_UPGRADE_BACKEND_IMAGE`） | 14 | 升级 version 变更 + 标记保持、回滚还原 | `report-20260812-214043-17a17c.json` |
| 第三环境逻辑备份/恢复 | 同上（`APP_BACKUP_RESTORE=1`） | 16 | `pg_dump` 备份 + 全新空库还原 + 标记/登录复验 | `report-20260812-215101-1fcddd.json` |
| 可复现闭环演示 | `scripts/demo-drill.sh` | 15 | 登录→态势→根因→证据→受控动作→验证→事故复盘（平台真实 API + 仓库内 mock k8s） | `report-20260812-221752-b358d9.json`、`report-20260812-221935-a2784a.json`（连续两轮 15/15） |
| 离线安装包全链路 | `scripts/offline-install-drill.sh` | 9 | bundle 组装（docker save + `pull_policy: never` manifest + SHA256SUMS）、完整性校验、docker load（digest 不变）、全新隔离环境安装、关键旅程、持久化、清理 | `report-20260812-222514-a0fbf8.json`、`report-20260812-222537-4e6c8d.json`（连续两轮 9/9） |
| OIDC 本地身份轨 | `scripts/oidc-login-drill.sh` | 14 | PKCE 登录、nonce/state/issuer/audience 校验、组角色映射、acr MFA、9 种失败注入 fail-closed、审计落库 | `.artifacts/oidc-drill/` |

## 3. 关键用户旅程覆盖（浏览器 + API 双通道）

| 旅程 | API 证据 | 浏览器证据 |
|---|---|---|
| 登录 → 会话 → `/me` | `scripts/demo-drill.sh` login / `dual-env-compose-drill.sh` key journey | `e2e/smoke.spec.ts` |
| 态势（fleet health + 节点） | `demo-drill.sh` situation | `e2e/smoke.spec.ts` |
| 根因（Node/Pod 诊断） | `demo-drill.sh` root-cause（`node.not_ready.v1`、`pod.oom_killed.v1`） | `e2e/diagnosis-timeline.spec.ts` |
| 证据时间线 | `demo-drill.sh` evidence（5 项 / `container_termination`） | `e2e/diagnosis-timeline.spec.ts`、`e2e/finding-evidence.spec.ts` |
| 受控动作（确认 → preview → execute） | `demo-drill.sh` action（`deployment.rollout_restart`，`succeeded`，mock 记录 PATCH） | —（动作 API 层验证） |
| 事故工作区（create→note→resolve→postmortem→export） | `demo-drill.sh` incident-journey | `e2e/incidents.spec.ts` |
| 双视口关键界面 | — | 42/42（M93-C 基线，Desktop/Mobile） |

## 4. 未覆盖/待组织环境项（不视为通过）

- 真实集群（kind/Helm）生命周期：本机无 kind/helm/kubectl，由 CI `real-kind-e2e.yml` 与组织环境覆盖。
- M89 真实 OIDC/MFA Provider 验收、M90 真实 WAL/PITR/HA 组织演练：**Deferred**（组织授权）。
- ENOSPC 真实磁盘压力故障注入：本地已列 Deferred（`wal-pitr-drill.sh` 边界）。
- 生产监控网络抓取验证：部署验证项，不在本地单测声称范围（`docs/testing/README.md`）。
