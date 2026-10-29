# 模型接入可配置化：控制台设置页 + 双协议 Provider + 运行时热更新

- **日期**：2026-10-29
- **类型**：功能新增（配置面 / Provider / 前端设置页）
- **关联**：`docs/changes/2026-10-29-console-page-availability.md` 中「AI 只能改环境变量」的缺口

## 背景

此前的模型接入只能通过环境变量配置，且协议被**写死为 OpenAI Responses API**
（`POST {base_url}/responses`）。这两个限制叠加后产生一个实际后果：绝大多数
OpenAI 兼容端点（DeepSeek、通义、Kimi、vLLM、Ollama 等）只实现 `/chat/completions`，
在 `/responses` 上直接 404 —— 也就是说填了 key 也接不通，而界面上没有任何地方可改。

## 变更

### 1. 配置从「环境变量」升到「数据库 + 运行期」

新增单行表 `ai_model_settings`（迁移 `000052`）。分层职责：

| 层 | 作用 |
|---|---|
| `internal/config` | 启动初值。仅在表为空时播种一次，保证升级后行为不突变 |
| `internal/aisettings` | 运行期真值。读、校验、加密存储、变更回调 |
| `aiexplain.Service` | 消费方。`ApplyConfig` 整体替换运行状态 |

**为什么不在 `config` 里做**：环境变量是进程启动时读一次的，改完必须重启；
而「在界面上改完立即生效」是这一需求的本质要求。

### 2. 密钥安全

- 与集群 kubeconfig 复用同一套加密器（`cluster.NewEncryptor`，AES-256-GCM + 密钥版本轮换）
- 表内只存 `api_key_ciphertext` + `api_key_key_version` + `api_key_hint`（末四位）
- 读取接口**永不返回明文**，只回 `api_key_set` 与 `api_key_hint`
- 写入时空值表示「保持原密钥」，`clear_api_key: true` 表示显式清除；**开启状态下拒绝清除**
  （否则会留下一个必然失败的可运行配置）
- 写路径的日志只打印掩码（`aisettings.Mask`）

### 3. 双协议 Provider

| 协议 | 端点 | 适用 |
|---|---|---|
| `chat_completions`（新增） | `POST {base}/chat/completions`，`response_format.json_schema` + `strict` | 绝大多数兼容端点 |
| `responses`（原有） | `POST {base}/responses`，`text.format` + `strict` | OpenAI 官方 |

两者实现同一个 `Provider` 接口，**四重约束完全复用**：证据引用强制校验、JSON Schema
严格模式、日 token 预算与并发准入、失败回退确定性诊断。协议差异对上层不可见。

### 4. 连通性测试

`POST /api/v1/settings/ai/test` 用**请求体里的值**探测（未保存也可测，`api_key` 留空回落到已存密钥）：

1. 先打 `GET {base}/models`——零 token 成本，兼容端点普遍实现；
2. 若该端点 404（只做推理的网关常见），退回一次极小 completion（`max_tokens: 16`）。

失败时按状态码给出可操作提示：401/403 → 密钥或权限、404 → 路径或协议选错、429 → 限流额度。

### 5. 控制台页面

`/settings/model`（系统管理员专属）：运行状态 / 今日用量 / 密钥状态三张卡 +
协议选择、地址、模型、密钥（密码框，占位符显示已存末四位）、超时、输出上限、日预算、
并发上限 + 测试连接与保存。页内说明密钥存储方式与四重约束。

## 验证（本地 mock 端点，两种协议各跑一遍）

用 `python3 /tmp/mock_llm.py` 起一个 OpenAI 兼容模拟端点（同时实现 `/v1/models`、
`/v1/chat/completions`、`/v1/responses`，并按请求里出现的第一个 `E<n>` 返回合规引用），实测：

| 项 | 结果 |
|---|---|
| 保存配置（含密钥） | 200，返回 `api_key_hint=****1234`，响应中**不含明文** |
| 库内存储 | `api_key_ciphertext` 不含明文，长度 44 字节，版本 `v1` |
| 保存前测试连通（chat_completions） | 200，`method=models`，可见 1 个模型 |
| 真实生成解释（chat_completions） | **201**，`provider=chat-completions-compatible`，引用 `E1` 通过校验，token 记账 120/80 |
| 切到 responses 后生成 | **201**，`provider=responses-compatible`，同样通过校验（**未重启进程**） |
| 空 `api_key` 保存 | 保留原密钥（`api_key_set` 仍为 true，hint 不变） |
| 关闭状态下清除密钥 | 200，`api_key_set=false` |
| 开启状态下清除密钥 | **400 `MISSING_API_KEY`**（按设计拒绝） |
| 不可达主机测试 | **502**，detail 为「无法连接到 127.0.0.1:19999（DNS/网络/证书问题）」 |
| AI 关闭时生成解释 | **503 `AI_DISABLED`**，提示确定性诊断仍可用 |
| 审计日志 | `ai_settings.read` 4 / `test` 3 / `update` 6 |

## 契约与文档同步

- `docs/api/openapi.yaml`：新增 3 个端点 + `AIModelSettingsWrite` / `AIModelSettingsProbe` schema
  （`api_key` 标注 `writeOnly`）
- `docs/security/permission-matrix.md`：随 `TestPermissionMatrixMatchesCommittedDocument` 重生成
- `internal/httpserver/openapi_route_test.go` 与 `router_harness_test.go`：注入最小
  `AIModelSettings` 桩，使契约测试双向覆盖新增路由

## 一处顺带修的旧缺陷

`internal/httpserver/diagnosis_node_metrics_test.go` 仍在断言内存越界的严重级为 `"medium"` ——
与该值不在 `diagnosis_records_severity_check` 允许集合内的事实冲突（昨天已改实现、漏改此断言）。
本次改为 `"high"` 并追加一条与 CHECK 约束一致的合法性断言。

## 边界

- 单行表单组织部署，不支持多套配置档案（多档案会带来「哪份生效」的歧义）
- 报告与配置均为单实例语义，未做多副本一致性（当前部署单副本）
- 模型调用的成本统计以 Provider 返回的 usage 为准；上游不返回 usage 时按 0 计
