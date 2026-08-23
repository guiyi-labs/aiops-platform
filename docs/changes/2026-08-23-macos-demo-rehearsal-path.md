# 演示链路 macOS 化：pwsh 原生路径 + 跨平台修复 + 压测探针

- Date: 2026-08-23
- Status: Complete
- Scope: 让答辩演示/排练全链路（verify-fast → compose 栈 → kind → demo-up）在 macOS(colima + PowerShell 7) 上原生可跑，不维护平行 bash 移植；新增无依赖压测探针产出论文用延迟曲线。

## Context

`docs/thesis/demo-environment.md` 的演示准备脚本全部是 PowerShell
（demo-up/down/e2e-kind/verify-fast），原答辩环境是 Windows 11。毕设需在
macOS（Apple Silicon, colima 容器运行时）上排练。两条路线对比后选择
「安装 pwsh 直接运行原版脚本」而非移植 bash——避免 ~860 行 E2E 链路的平行
实现漂移导致证据分叉。

## What Changed

### scripts/verify-fast.ps1

- `Resolve-Go` 之后 gofmt 解析不再硬编码 `gofmt.exe`：
  `Join-Path (Split-Path -Parent $go) ('gofmt' + [IO.Path]::GetExtension($go))`，
  Windows(.exe)/POSIX('') 通吃。
- `Enable-NodePath` 追加 PATH 使用 `[IO.Path]::PathSeparator`。

### scripts/load-probe.mjs（新增）

- 零依赖 Node 探针：登录取 token → 按 `--levels` 并发档位对指定只读端点
  计时（每 worker 顺序请求），预热 20 次后测量，输出 p50/p95/p99/max/rps/
  errors 与 `-json` 报告。
- 输出明确标注 "functional-level reference — not a production benchmark"，
  与实验摘要的诚实边界口径一致。

### 环境结论（写入论文演示文档的事实）

1. macOS 安装官方 PowerShell 7.4.6 tarball（brew 无 pwsh formula）；30 个
   `.ps1` 全部解析通过，运行时 Windows-ism 仅 verify-fast 两处（已修）。
2. colima 默认 2C/2G 会被 compose 构建压垮（docker daemon 失去响应、kind
   建群失败）；`colima start --cpu 6 --memory 12` 后稳定。
3. kind node 拉镜像失败三连因：colima 的 containerd 存储 `docker save`
   产物缺 blob（ctr import 报 content digest not found）；node 继承宿主
   创建时的代理 env（127.0.0.1:7897 代理未开机即拒连）。解法：给 node 的
   containerd 加 no-proxy drop-in 后走 `docker.m.daocloud.io` 镜像源拉取并
   retag 回原始 tag（nginx/busybox×2/nginx-unprivileged）。
4. demo-up 全链在 macOS 实跑成功：kind v1.36.1 集群注册 Ready、3+4 条诊断、
   ImagePullBackOff 确认、rollout restart 幂等执行，证据
   `.artifacts/demo/demo-ready-20260823-145952.json`。

## Verification

- `pwsh -NoProfile -File scripts/verify-fast.ps1`（干净树）：passed in 1.45s。
- `pwsh -NoProfile -File scripts/demo-up.ps1`：exit 0，summary status=demo-ready。
- `node scripts/load-probe.mjs --path /api/v1/clusters?limit=10 --path
  /api/v1/diagnoses?limit=10 --path /api/v1/incidents?limit=10 --levels
  1,4,16,64 --total 240 --json .artifacts/bench/load-report.json`：
  c=1 p95≈10ms → c=16 p95≈40ms → c=64 p95≈100ms（rps≈720–800），全程 err=0；
  报告已落盘 `.artifacts/bench/load-report.json`。

## Risks / Notes

- colima/kind 的镜像预载步骤目前是手工操作；若答辩前重建集群需按本文
  Context 第 3 条重复。后续可固化为 `scripts/kind-preload-images.sh`。
- load-probe 为功能级下限参考；论文表述不得写成生产基准。
