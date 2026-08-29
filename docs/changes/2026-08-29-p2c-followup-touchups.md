# P2c 后续小修：aiopsbench/seed 报告文件权限收紧至 0600 + 格式对齐

- Date: 2026-08-29
- Status: Complete
- Scope: aiopsbench 两模式报告文件权限 0644 → 0600；seed-knowledge 缩进对齐；router 与 router_harness_test 的 import 排序。

> 复制本文到 `docs/changes/YYYY-MM-DD-<slug>.md` 填写。规范见 `docs/ARCHIVING.md`。

## Context

P2c（只读端点 + 幂等 seed 脚本，HEAD=4afe578）合并后，对落盘产物做了一次收尾性卫生清理：

- `aiopsbench` 的 `diagnosis` / `retrieval` 两种回放模式会把评估报告以 JSON 写入本地文件，原先权限为 `0o644`（组内/其他可读），不符合敏感质量基准产物的权限纪律（参考此前 P2a 质量部分已将多处文件权限收紧至 `0o600`）。
- `seed-knowledge/main.go` 结构体字面量对齐存在轻微错位，统一为 gofmt 风格。
- `router.go` 与 `router_harness_test.go` 的 import 块顺序未完全遵循排序，gofmt 下会产生噪声 diff。

本次改动零语义变化，仅为权限收紧与格式对齐（符合 AGENTS.md 第 1 节"纯 refactor / 拼写修正仍须登记，但可合并进同一份 change-record"的例外条款）。

## What Changed

### backend/cmd/aiopsbench

- `backend/cmd/aiopsbench/diagnosis.go`：`runDiagnosis` 写入报告文件 `os.WriteFile(*jsonOut, ..., 0o644)` → `0o600`（仅报告文件权限收紧，路径/内容不变）。
- `backend/cmd/aiopsbench/retrieval.go`：`runRetrieval` 写入报告文件 `os.WriteFile(*jsonOut, ..., 0o644)` → `0o600`（同上）。

### backend/cmd/seed-knowledge

- `backend/cmd/seed-knowledge/main.go`：`seed` 中 `KnowledgeEntry` 结构体字面量的 `Summary` / `RootCauses` 字段缩进对齐（纯空白，零行为变化）。

### backend/internal/httpserver

- `backend/internal/httpserver/router.go`：import 块中 `k8sgateway "k8s-aiops.local/backend/internal/kubernetes"` 行按字母序从 `knowledge` 之前移至之后（纯 import 排序）。
- `backend/internal/httpserver/router_harness_test.go`：import 块同样调整 `k8sgateway` 行顺序（纯 import 排序）。

## Verification

- `cd backend && gofmt -l cmd/aiopsbench/diagnosis.go cmd/aiopsbench/retrieval.go cmd/seed-knowledge/main.go internal/httpserver/router.go internal/httpserver/router_harness_test.go`：输出为空，5 个文件均已格式化。
- `cd backend && go build ./...`：构建成功（BUILD_OK）。
- `git diff` 确认：仅上述权限数值、缩进空白、import 顺序三类差异，无逻辑改动。

## Risks / Notes

- 权限收紧至 `0o600` 后，若此前由其他用户/组运行的 CI 或容器需读取 `aiopsbench` 报告，需确保由同一 uid 读取；本地单人运行无影响。
- 全部为卫生性改动，可随时通过还原对应行回退；不影响既有契约、开放端点或运行时行为。
- 提交未包含 `backend/out`、`backend/Dockerfile.local` 等可能残留的本地产物（不入库、不删除正在运行容器的镜像）。
