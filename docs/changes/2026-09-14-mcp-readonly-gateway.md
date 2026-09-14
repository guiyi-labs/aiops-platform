# mcpserver：面向通用 AI Agent 的只读 MCP 网关

- Date: 2026-09-14
- Status: Complete
- Scope: 新增 `internal/mcpserver` 与 `cmd/aiops-mcp` —— 以 Model Context Protocol 把平台既有的只读诊断能力暴露给通用 AI Agent（Claude Desktop / Cursor 等）。服务端**零新依赖**（协议子集手写），暴露面为编译进二进制的封闭工具目录。

## Context

平台对内的约束已经完整：AI 调查员只能引用服务端授权的证据 ID，日志 / 事件 / 标签被显式声明
为不可信数据，模型只允许调用固定的只读工具（禁 kubectl / SQL / PromQL）。但**对外**没有等价物
—— 除 Web 控制台外，外部 Agent 无法复用这套诊断能力，于是只能各自直连集群，等于绕开全部约束。

本次补上对外的那一半：让外部 Agent 只能调用服务端**已发布**的能力，而不是直连集群。

两半回答同一个问题（怎么让模型帮忙又不让它乱走），给出同一种答案（把许可集合做小并写死），
只是方向相反：对内约束模型，对外约束 Agent。

## What Changed

### internal/mcpserver（新增）

- `protocol.go`：JSON-RPC 2.0 消息类型、协议常量、错误码。固定 revision `2025-06-18`，
  **不镜像调用方版本** —— 镜像一个自己没测过的版本，等于声称了未经检验的一致性。
- `tools.go`：工具目录 + `Arg` / `Tool` 模型 + `ValidateArguments` + `URLPath` + `inputSchema`。
  - `argumentValuePattern` 是严格白名单 `^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`。
    这是本包**承重的控制点**：路径占位符由参数值替换，所以 `/`、`?`、`#`、空白、`%`、前导点
    都必须被排除。白名单而非黑名单，是因为黑名单需要每次有人想到新的元字符就更新一次。
  - 未声明参数一律**报错**而非静默丢弃：探测未公开参数的调用者应该收到"被拒"，而不是一个
    看起来正常的响应。
- `server.go`：会话循环（换行分隔 JSON-RPC）、方法分发、错误分型。
  `Backend` 接口只有**一个 GET 方法** —— 只读性做成结构性的：没有可以写入的函数，
  将来任何工具想写入都必须改这个接口，而这个改动在 diff 里看得见。
  区别对待两类失败：后端不可达 → 工具结果内 `isError`（调用本身合法）；调用畸形 → JSON-RPC 错误。
- `httpbackend.go`：带 Bearer token 的 HTTP 客户端；拒绝非相对路径与协议相对 URL；
  响应体上限 4 MiB；非 2xx 映射为错误；空 token 不发送 Authorization 头。
- 工具目录（9 个，全部只读）：`list_diagnoses` / `get_diagnosis` / `get_diagnosis_evidence` /
  `list_incidents` / `get_incident` / `get_incident_evidence` / `search_knowledge` /
  `knowledge_stats` / `get_cluster`。

### cmd/aiops-mcp（新增）

- `main.go`：环境变量配置（`AIOPS_API_URL` / `AIOPS_API_TOKEN` / `AIOPS_API_TIMEOUT`），
  stdio 传输。**独立进程**：平台的 API 面、认证与审计完全不变，本进程只持一个 token
  调同一批 GET 端点。stdout 专供协议，日志一律走 stderr。

## Verification

- `go test ./internal/mcpserver/ ./cmd/aiops-mcp/ -cover`：ok，coverage **97.3%** / **83.3%**。
- 安全负向用例（全部通过，且断言"未触达 backend"）：
  - 路径穿越 / 注入 8 例：`../../api/v1/users`、`7/../../admin`、`7?admin=1`、`7#fragment`、
    `7 8`、`7%2e%2e`、`.hidden`、`a/b`。
  - 未声明参数（`verb=DELETE`）被拒。
  - 未知工具被拒。
  - 缺必填参数、非字符串参数被拒。
  - 非相对路径 / 协议相对 URL 被 HTTP 客户端拒绝。
- 协议正向用例：initialize 返回值与能力声明；tools/list 目录完整且 schema 拒绝额外属性；
  tools/call 只转发已声明参数并原样返回后端响应体；notification 静默；未知方法 MethodNotFound；
  畸形消息不终止会话。
- `go vet ./internal/mcpserver ./cmd/aiops-mcp`：无输出。
- `gofmt -l .`（backend 全仓）：无输出。

## Risks / Notes

- **协议子集**：只实现 `initialize` / `notifications/initialized` / `tools/list` / `tools/call`。
  resources、prompts、sampling、subscriptions 未实现并明确拒绝。这是**收窄而非缺口** ——
  Agent 无法向本服务索取已发布只读工具之外的东西，正是设计目标。
- **未做真实客户端联调**：单测覆盖协议层与边界，但尚未用 Claude Desktop / Cursor 实机接入验证。
  这是明确的待补步骤，不在本次范围内。
- **零新依赖是有意的**：协议手写，未引入 `modelcontextprotocol/go-sdk`。理由：避免新增依赖链
  （含 jsonschema 等间接依赖）与供应链面，并让"拒绝子集外方法"成为实现的一部分而非配置项。
  代价是需自行跟进协议演进，尤其 2026-07-28 的无状态模型（`server/discover` 取代 initialize 握手）。
- **授权仍由 token 决定**：本服务不自带权限模型，Agent 能读到什么 = token 持有者能读什么。
  需要"工具目录"与"token 范围"同时失效才会越权。
