# M94 回放模式：只读 insight 链路回放（诊断详情）

- Date: 2026-08-12
- Status: Complete
- Scope: M94 剩余增量——按事件时间重放 M81 insight 链路的后端 API + 前端交互

## Context

M94 诊断叙事已分三步落地（根因卡 + 证据时间线、行动区、深链），三个既有
change-record 均声明"回放模式仍为后续增量"。本次完成该增量：在诊断详情中
按事件时间回放 M81 insight 链路（诊断创建 → 证据 → 状态与协作 → AI 引用解释
→ 受控动作），严格使用已存储产物，绝不重新生成或伪造历史 AI 结论。

## What Changed

### 后端（诊断领域 + HTTP 层）
- `backend/internal/diagnosis/replay.go`（新）：`ReplayStageID` 五阶段
  （diagnosis_created / evidence / activity / ai_explanation / remediation）、
  `ExplanationSnapshot`/`RemediationSnapshot`（由 HTTP 层填充，保持纯模块）、
  `ReplayStep`/`ReplayStage`/`ReplayView`（schema `aiops.diagnosis-replay/v1`）、
  `BuildReplay` 纯函数——只按存储产物构建、按事件时间稳定排序、可选服务为空
  则无步骤、不伪造；`diagnosis_created` 使用 `record.CreatedAt`（零值回退
  `ObservedAt`）。
- `backend/internal/diagnosis/replay_test.go`（新）：3 个纯函数测试全部 PASS。
- `backend/internal/httpserver/replay.go`（新）：`GET /diagnoses/:diagnosis_id/replay`
  handler + 快照转换（AI 解释经 `aiexplain.Service.List` 读 actor；remediation 经
  `remediation.Service.List`；任一失败返回 nil，不破坏回放）。
- `backend/internal/httpserver/replay_test.go`（新）：4 个 HTTP 层测试
  （存储链路渲染/非法 ID/未找到/可选服务降级）全部 PASS。
- `backend/internal/httpserver/diagnosis.go`：handler 结构体增加
  `explanations *aiexplain.Service`、`remediations *remediation.Service`。
- `backend/internal/httpserver/router.go`：注入服务并注册 replay 路由
  （AuditAction `diagnosis.replay.read`）。
- `docs/api/openapi.yaml`：新增 3 个 schema + replay path；
  `TestRegisteredRoutesMatchOpenAPI` 通过。
- `docs/security/permission-matrix.md`：重新生成（路由总数 279→280、已审计 157→158）。

### 前端
- `frontend/src/types/diagnosis.ts`：新增 Replay 类型。
- `frontend/src/api/diagnosis.ts`：新增 `getDiagnosisReplay(token, diagnosisID)`。
- `frontend/src/composables/useDiagnosisReplay.ts`（新）：播放/暂停/上一步/下一步/
  seek/按阶段筛选/reset 的状态机；定时器用全局 `setInterval`/`clearInterval`。
- `frontend/src/composables/useDiagnosisReplay.test.ts`（新）：4 个测试全部 PASS。
- `frontend/src/views/DiagnosesView.vue`：打开诊断详情时加载回放，新增 `replay-panel`
  区块（控制条 + 进度条 + 阶段 chips + 步骤卡片，置于证据时间线之前）。
- `frontend/src/styles/base.css`：追加 replay 样式（面板/控制/进度/阶段/步骤/错误态）。

## Verification

- `cd backend && go build ./... && go vet ./... && go test ./...`：全部通过
  （含 `TestRegisteredRoutesMatchOpenAPI`、权限矩阵一致性测试）。
- `cd frontend && pnpm typecheck && pnpm build && pnpm test -- --run && pnpm lint`：
  全部通过（141 tests / 26 files）。
- `scripts/scan-sensitive-fields.sh`：clean。
- 证据位置：无新 .artifacts；回放数据来自既有诊断记录，未新增存储。

## Risks / Notes

- 现有本地镜像（`k8s-aiops-backend/frontend:latest`）不含 replay 代码；如需端到端
  覆盖 replay 路由，须按既有约束离线重建镜像（宿主机 `GOOS=linux GOARCH=arm64`
  预编译后端 + 前端 build 打包）后再跑 demo/offline drills，可作为后续增强。
- 可选服务（AI 解释 / remediation）不可用或失败时，回放自动降级为纯存储步骤，
  前端展示错误提示而非空白，接口保持 200。
- 推送积压仍被 GitHub 凭据阻塞（本地领先 origin/main 18 个提交 + 多个 baseline tag），
  待用户提供凭据后统一 `git push origin main --tags`。
