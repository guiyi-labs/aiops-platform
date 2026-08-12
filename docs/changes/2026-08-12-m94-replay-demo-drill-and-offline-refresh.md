# M94 回放纳入演示闭环与离线包刷新（v0.3.0-rc.5-replay）

- Date: 2026-08-12
- Status: Complete
- Scope: 把 M94 回放模式纳入可复现演示闭环，并用最新代码重建镜像、刷新离线安装包证据

## Context

M94 回放模式（`docs/changes/2026-08-12-m94-diagnosis-replay.md`）已归档，但现有本地镜像
（`k8s-aiops-backend:latest` / `k8s-aiops-frontend:latest`，对应 commit 2e8a3813/75b1db2d）
不包含回放代码，无法覆盖 `GET /diagnoses/:id/replay` 端到端验证。本回合打通两条线：
（1）重建含回放代码的双新镜像，并让 `scripts/demo-drill.sh` 闭环断言回放链路；
（2）用新镜像重新产出可复用离线安装包，回填「离线包包含最新功能」的安装证据。

## What Changed

### 镜像重建（宿主机交叉编译，离线可复现）
- 后端：`go build` 交叉编译 `GOOS=linux GOARCH=arm64`（`api`/`credential-reencrypt`/
  `audit-archive`/`identity-readiness`/`recovery-readiness` 五个二进制，
  Version=`v0.3.0-rc.5-replay`）→ 以 `alpine:3.22` 封包
  `k8s-aiops-backend:v0.3.0-rc.5-replay`（sha256:4b69f…）。
- 前端：复用本地 `k8s-aiops-frontend:latest` 的 nginx 运行时层，覆盖新 `dist`
  （含 replay-panel bundle）→ `k8s-aiops-frontend:v0.3.0-rc.5-replay`（sha256:205ce…）。
- 继续采用宿主机预编译 + `DOCKER_BUILDKIT=0` 单阶段封包，规避 colima 2GiB 内
  BuildKit 并行构建 OOM。

### 演示闭环：回放断言
- `scripts/demo-drill.sh`：
  - 受控动作前新增 `Replay (回放)` 场景：Node 诊断回放 schema=`aiops.diagnosis-replay/v1`、
    `diagnosis_id` 一致、步骤 ≥ 2、阶段含 `diagnosis_created`+`evidence`、时间升序稳定。
  - 受控动作后新增 `Replay after actions (回放)` 场景：Pod 诊断回放阶段含
    `activity`+`remediation`，类型含 `remediation_created`+`remediation_executed`，时间升序稳定。

## Verification

- `scripts/demo-drill.sh`（`APP_BACKEND_IMAGE`/`APP_FRONTEND_IMAGE` 指向 rc.5-replay）：
  17/17 PASS（原 15/15 + 回放 2 场景），报告
  `.artifacts/demo-drill/report-20260812-224548-acb7f5.json`（replay-before/replay-after 均 pass）。
- `scripts/offline-install-drill.sh`（`APP_VERSION=v0.3.0-rc.5-replay`，新双镜像）：
  10/10 PASS，报告 `.artifacts/offline-install-drill/report-20260812-224624-c93190.json`；
  产出可复用离线包
  `.artifacts/offline-install-drill/bundle/aiops-platform-offline-v0.3.0-rc.5-replay/`
  （`images/*.tar` + `OFFLINE-SHA256SUMS` 6 文件校验 OK + `pull_policy: never` 全新环境安装 +
  admin 登录 → system_admin + 数据持久化跨 recreate）。
- `scripts/scan-sensitive-fields.sh`：clean。
- 前端门禁（dist 构建前）已通过：`pnpm typecheck/build/test(141)/lint`。

## Risks / Notes

- 新镜像 tag 为 `v0.3.0-rc.5-replay`，是本地演练产物；`k8s-aiops-backend:latest` /
  `frontend:latest` 以及 `v0.3.0-rc.5`、`rc.4` 等既有 tag 保持不变。
- 回放属只读增量，不引入新命令、Pod exec 或写操作；离线包仍与既有 compose/manifest 契约一致。
- 推送积压仍被 GitHub 凭据阻塞（本地领先 origin/main 20 提交 + 12 个 baseline tag），
  待用户提供凭据后统一 `git push origin main --tags`。
