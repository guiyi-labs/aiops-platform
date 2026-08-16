# aiops CLI（diagnose + cases）— 落地

- Date: 2026-08-16
- Status: Complete
- Scope: 用户拍板 CLI 范围限定 —— 只做 diagnose + cases 两个子命令
- DependsOn: P1 RAG 知识库（a889b2a）；README 英文版（0fcaacd）

## What

新增 `backend/cmd/aiops/`（Go 1.26，零新增第三方依赖），一个「no server、
no DB」的体验入口：`go install .../cmd/aiops@latest` 一行即可跑。

### 1. `aiops diagnose` — 确定性规则诊断

- **复用** `internal/diagnosis` 的编译期纯规则函数
  （EvaluatePodOOMKilled → EvaluateImagePullBackOff → EvaluateCrashLoopBackOff
  → EvaluatePodPending，与 `diagnosis.Service.evaluatePod` 同序）和
  `internal/cluster.ParseKubeconfig`；`internal/knowledge` 提供 Entry/Filter 语义。
- **两种模式**：
  - 真实集群：`--kubeconfig`（默认 `~/.kube/config`）→ 直连 API Server
    （`/api/v1/namespaces/{ns}/pods` + `fieldSelector=involvedObject.uid` 拉
    events，Bearer token），扫描命名空间全部 Pod（`--pod` 可指定单个，
    `--namespace` 默认 default）。
  - demo 降级：默认 kubeconfig 缺失/不可解析时，**内置 5 个演示 fixture**
    （crashloop / image-pull-backoff / oom-killed / pending / healthy）跑同一
    规则链——网友零门槛体验；显式 `--kubeconfig` 仍报错（exit 2）。
- **英文输出**（`ruleEnglish` 映射，全球受众）+ `-o json` 机器可读。
- **exit code**：0 无告警 / 1 有告警 / 2 错误。

### 2. `aiops cases` — 历史案例查询

- **复用** `internal/knowledge.Entry` 模型 + Severity 语义 + Filter 概念。
- 无 `--server`（或 server 端点不可达/404）→ **降级内置规则目录**（6 条
  代表性案例，覆盖 crashloop/imagepull/oom/pending/no-endpoints/node-not-ready），
  `--query` 关键词匹配（rule_id/summary/root_causes/资源名）+ `--severity`
  最小级别过滤 + `--limit`。
- `--server <url>` 尝试 `GET /api/v1/knowledge/entries`；端点暂未随 v0.1.0
  平台面世，失败即降级并在 stderr 明示（契合「知识库故障不阻塞诊断」纪律）。
- **exit code**：0 有结果 / 1 无结果 / 2 错误。

### 3. 脚手架

- `main.go`：子命令路由 + `--version`（v0.1.0）+ usage；英文帮助。
- G107 已按 gosec #nosec 注明（用户指定的 --server URL 属预期请求）。

### 4. 测试（cmd/aiops 覆盖率 84.8%）

- demo 模式：4 个故障 fixture 全部命中 + healthy 不命中；排序（critical 先）；
  `-o json` 结构断言。
- 真实集群模式：httptest 假 API server 验证 listPods/podEvents/Bearer 路径、
  `--pod` 未找到报错、非 2xx 透传。
- cases：命中/无结果(exit 1)/非法 severity(exit 2)/severity 过滤/排序/server
  成功/server 降级/关键词匹配。
- 全量 `go test ./...` 75 包全绿；全局覆盖率 **70.2% ≥ 70%**；
  golangci-lint（含 gosec）**0 issues**。

## Verification

- `go test ./...`（backend，75 包）全绿。
- `go test -cover ./...` 全局 70.2%（cmd/aiops 计入后不低于门禁）。
- `golangci-lint run --config ../.golangci.yml ./cmd/aiops/...` 0 issues。
- 手动冒烟：`go run ./cmd/aiops diagnose`（demo 降级 + 4 findings，exit 1）、
  `aiops cases --query oom`（1 条，exit 0）、无匹配 exit 1。

## Artifacts

- `backend/cmd/aiops/{main,diagnose,cases,english,demo}.go` + 3 个测试文件。
- README 已预留 CLI 段落（0fcaacd）。