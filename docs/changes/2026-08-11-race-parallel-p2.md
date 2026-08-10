# Race Detector Parallel Optimization

- Date: 2026-08-11
- Status: Complete
- Scope: 将 CI race detector gate 的 Go 测试并行度从 -p=1 提升到 -p=2，降低全量 CI 延迟。

## Context

CI 优化第一阶段（CHANGELOG.md 文档快速路径、合并 Test/Coverage、共享 Backend
镜像、Node 24 action 升级）完成后，全量 CI 延迟约 376 秒，其中 Backend race
job 串行执行（-p=1）耗时约 6m14s，是唯一剩余的显著墙钟瓶颈。

此前分析建议先做远端 A/B，确认 -p=2 不引起 OOM 或竞态漏检后再提交。

## What Changed

- `.github/workflows/ci.yml`：Race detector 步骤从 `go test -race -p=1`
  改为 `go test -race -p=2`，并在并发组中保留 `cancel-in-progress: true`。
- `backend/internal/deployment/ci_workflows_test.go`：工作流契约测试中对应
  标记同步更新为 `-p=2`。

## Verification

- A/B 实验 branch `ci/race-parallel-ab`（run
  `31413981487`）：race job 墙钟 4m6s（基线 -p=1 为 6m14s），节省约 2 分钟
  （~33%）；全部 Go package 均通过 `ok`，无 DATA RACE 检出，无 OOM 或内存相关
  错误。
- 合并回 main 后的完整 CI（run `31415944207`，重跑后通过）：全部 job 绿，
  Backend race 3m55s，与基线 -p=1（6m46s）相比节省约 2m51s（~42%）。
  注：首次执行时 Compose runtime job 因 Docker Hub 拉取 `nginx:1.27-alpine`/
  `node:22.13.1-alpine3.21` 偶发超时失败（`dial tcp ...:443: i/o timeout`），
  与本次改动无关，重跑同一 run 后通过。
- `node --test scripts/release-manifest.test.mjs`：6/6 通过。
- `go test -p=1 -count=1 ./internal/deployment/`（backend 模块）：通过。

## Risks / Notes

- Go 的 -race 在进程级别检测竞态，-p=2 下两个进程无共享可变状态，
  不影响检测质量。
- 2 vCPU / 7 GB ubuntu-24.04 runner 的内存经验证可承受两个 race-instrumented
  进程并行，单进程内测试包内存使用量在本项目中较为保守。
- 若未来加入内存密集型集成测试，可考虑降至 -p=2 与 -p=1 的混合策略
  （对重包串行、轻包并行），但当前不需要。
