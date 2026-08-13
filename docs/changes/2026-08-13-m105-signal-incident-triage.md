# M105：信号归一化摄取接线 + 信号 → 事故工作空间联动（Signal-to-Incident Triage）

- Date: 2026-08-13
- Status: Complete
- Scope: 接通 M39 信号层缺失的诊断摄取路径（`DiagnosisNormalizer` 编译未接线），并新增第 5 个事故来源 `signal`：把归一化信号实例一键提升为事故工作空间，补齐「诊断/告警/巡检/信号 → 事故 → 复盘」闭环与同源语义统一。

## Context

M103/M104 已把告警与巡检连入事故工作空间；事故来源现覆盖 diagnosis/finding/alert/inspection。巡检期间发现 M39 信号层存在真实接线缺口：`internal/signal/diagnosis_normalizer.go` 已实现「诊断记录 → IngestRequest」的纯函数映射，但服务端从未调用它——`signal_occurrences` 仅由 SLO burn sink 写入。后果是诊断产生的信号（`diag.*.v1`）不会进入信号总览/关联引擎，也无法作为事故来源。M105 补上该管线（后台 drain worker），并让信号实例成为第 5 类事故来源，满足产品目标「同一资源的 diagnosis、finding、inspection 和 SLO 信号使用一致的严重度、时间和来源语义」。

## What Changed

### Backend：诊断信号 drain worker（M39 接线）

- `backend/internal/signal/diagnosis_drain.go`（新）：`DiagnosisDrain` 后台 worker，`updated_at` 严格递增游标 + 每批 `PageSize`（默认 50）分页拉取 `diagnosis_records`，经 `DiagnosisNormalizer` 归一化后 `Ingest`（fingerprint upsert 幂等）。启动 watermark 取当前时间（不回放历史）；未映射规则跳过（不视为失败）；单条摄取失败记录日志、不中断整批、watermark 照常推进（该记录下次状态变化时重试）。
- `backend/internal/signal/repository.go` / `gorm_repository.go` / `service.go`：新增 `Get(ctx, id)`（仓库 + Service），导出 `ErrSignalNotFound` sentinel（替换原预留的未导出 `errSignalNotFound`）。
- `backend/internal/diagnosis/model.go` / `repository.go`：`ListFilter` 新增 `UpdatedAfter *time.Time` 游标过滤（`d.updated_at > ?`）。
- `backend/internal/config/config.go`：`SignalConfig` 新增 `DiagnosisIngestion`（env `SIGNAL_DIAGNOSIS_INGESTION`，默认 true）与 `DiagnosisDrainInterval`（env `SIGNAL_DIAGNOSIS_DRAIN_INTERVAL`，默认 5s，下限 1s）。
- `backend/cmd/server/main.go`：M39 信号/SLO 装配块移动到事故服务之前（解决 `signalService` 顺序依赖）；drain 作为第 5 个后台协程（受 `DiagnosisIngestion` 门控）。

### Backend：事故第 5 来源 `signal`

- `backend/internal/incident/model.go`：新增 `SourceTypeSignal = "signal"` 与 `SourceRefForSignal(occurrenceID)`（`signal:<id>` 稳定去重身份）。
- `backend/internal/incident/service.go`：`Create` 白名单接受 `signal` 来源。
- `backend/cmd/server/incident_resolver.go`：新增 `signalOccurrenceReader` 接口 + `signalServiceAdapter`（包装 `*signal.Service.Get`）；`resolveSignal`：前缀/ID 校验、`ErrSignalNotFound` → `ErrInvalidSource`、簇防泄漏（`ClusterID` 必须等于调用方）、严重级映射（signal critical/warning/info 1:1）、资源/观测时间富集；`NewIncidentResolver(records, alerts, inspections, signals)` 签名扩展。
- `backend/migrations/000044_incident_signal_source.up.sql`（+ down）：`incidents.source_type` CHECK 增加 `'signal'`。
- `docs/api/openapi.yaml`：`IncidentCreateRequest.source_type` enum 增加 `signal`，source_ref 注释补 `signal:<id>`。

### 测试

- `backend/internal/signal/diagnosis_drain_test.go`（新）：摄取映射规则并跳过未映射、watermark 严格推进不重复摄取、List/Ingest 失败容忍（不崩溃、不阻塞）。
- `backend/internal/signal/service_test.go` / `slo_burn_normalizer_test.go` / `internal/httpserver/signal_test.go`：mock repo 补 `Get`。
- `backend/internal/incident/service_test.go`：`TestCreateFromSignal`（富集 + 重复去重）。
- `backend/cmd/server/incident_resolver_test.go`：`TestResolveSignal`、`TestResolveSignalRejectsForeignCluster`、`TestResolveSignalInvalidOrMissing`。
- `backend/internal/incident/model_test.go`：`SourceRefForSignal`。

### Frontend

- `src/types/incident.ts`：`IncidentSourceType` 增加 `'signal'`。
- `src/views/AIOpsOverviewView.vue`：信号列表新增「创建事故工作区」按钮（`FilePlus2`，ops/system_admin 且非 resolved 信号可用），调用 `createIncident(source_type:'signal', source_ref:'signal:<id>')`，处理 `SOURCE_ALREADY_USED`，顶部业务提示（复用 `.ok-message` 成功样式）。
- `src/views/IncidentsView.vue`：创建表单来源类型增加「信号实例」，标题/严重级/摘要自动填充禁用，`sourceRefPlaceholder` 与详情友好来源标签同步。
- `src/api/openapi.d.ts`：`pnpm typegen` 重新生成，`source_type` union 含 `signal`。

### 演示演练（demo-drill）

- `scripts/demo-drill.sh`：后端 env 加 `SIGNAL_DIAGNOSIS_INGESTION=true` + `SIGNAL_DIAGNOSIS_DRAIN_INTERVAL=2s`（确定性）；新增第 12 节「Signal → incident」4 条断言：等待 `diag.node.not_ready.v1`（诊断恒定 critical → drain 归一化）→ 从信号提升事故 → 严重级从 occurrence 富集为 critical → 重复提升被 `SOURCE_ALREADY_USED` 拒绝。

## Verification

- `cd backend && go build ./...`：通过；`gofmt -l .` 干净。
- `cd backend && go vet ./...`：通过；`golangci-lint run ./internal/signal/... ./internal/diagnosis/... ./internal/incident/... ./internal/httpserver/... ./cmd/server/... ./internal/config/...`：0 issues。
- `cd backend && go test ./...`：67 个包全绿。
- `cd frontend && pnpm typegen && pnpm typecheck && pnpm lint && pnpm test`：typecheck/lint 干净，26 files / 141 tests 通过。
- `cd frontend && pnpm build`：成功。
- `./scripts/demo-drill.sh`：**32/32 PASS**（原 28/28 + 4 条 signal→incident），报告 `.artifacts/demo-drill/report-20260813-094229-ea2e13.json`（artifacts 已被 gitignore，本地复验用）。

## Risks / Notes

- drain 是后台 best-effort：摄取失败仅记录日志并推进 watermark，记录在下次状态变化（`updated_at` 更新）时重试；因此瞬时故障不阻塞后续记录。
- 启动 watermark 取进程启动时刻，历史诊断不会在升级后被回放；如需补历史可重启后依赖诊断状态变化逐条收敛。
- `SIGNAL_ENABLED` 默认 false 控制的是概览窗口等可选能力；`signal_occurrences` 写路径（SLO sink / drain）与信号路由一直存活，本次 drain 默认开启不影响既有行为契约。
- 信号严重级无 high：映射 critical→critical、warning→warning、info→info，与信号目录语义一致。
- 新增枚举/来源不影响既有 diagnosis/finding/alert/inspection 事故；迁移只放开 CHECK，不破坏存量数据。
- `k8s-aiops-backend:latest` 本地镜像已按 arm64 交叉编译重建以承载新代码（`.artifacts` 不入库）。
