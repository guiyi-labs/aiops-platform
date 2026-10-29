# 模型接入实跑打通 + 拓扑采集补齐

日期：2026-10-29
范围：`internal/aiexplain`、`internal/aisettings`、`internal/topology`、`internal/httpserver`、`cmd/server`、控制台（模型接入页、AIOps 概览拓扑页）

## 背景

上一轮做完了「控制台可配置模型接入」（`/settings/model`）与 `ChatCompletionsProvider`，但验证只覆盖到本地 mock 端点。接入真实服务商（`https://modelaxis.ai/v1` + `deepseek-flash`）后立刻暴露出一串只在真实模型上才会出现的问题，另有一项遗留缺口（拓扑采集器）未补。

## 一、真实模型生成失败：输出被截断

现象：`POST /diagnoses/{id}/explanations` 返回 `502 AI_INVALID_OUTPUT`，message 只有一句笼统的
「failed citation validation」，无从判断该改哪里。

排查后根因是 **`max_output_tokens=1200` 不够**：调高到 4096 后同一诊断一次通过，实际消耗
**2494 output tokens**。一份合规输出（`summary` + `analysis` + 3~5 条 `recommended_actions`
+ 3~5 条 `citations`，中文）在推理型模型上稳定超过 1200。

### 改动

1. **默认值 1200 → 4096**（`config.AI_MAX_OUTPUT_TOKENS` 默认值、`aisettings` 兜底值、
   控制台表单初始值三处一致）。上限同时放到 32768，与管理端校验对齐。
2. **失败原因具体化**（`decodeStructuredOutput`）。原来四个分支共用一句文案，现在分别给出：
   - 空内容
   - JSON 不合法（附 120 字符响应摘要）
   - 必需字段为空（列出缺哪个：summary / analysis / citations）
   - 引用了不存在的证据（给出被引 ID 与**可用 ID 白名单**，按字典序稳定输出）
   - `priority` 非法（给出实际值与合法枚举）
3. **识别截断**：Chat Completions 的 `finish_reason=length`、Responses API 的
   `status=incomplete` + `incomplete_details.reason`。不先判这个，截断会伪装成
   「malformed JSON」——而真正的处置是调高上限，报错必须指向它。
4. **容忍 ```json 围栏**：`json_schema` 严格模式下本不该出现，但兼容端点常见，
   拆围栏的成本远低于让整次调用失败。
5. **提示词显式给白名单**：输入里新增 `citable_evidence_ids`，系统提示要求「逐字照抄、
   不得改写或自造 ID」。此前模型只能从 `evidence[].id` 自行推断，能力较弱的模型常编出
   `EV-1`、`evidence_1`。
6. **错误透出**：`ErrInvalidOutput` / `ErrProviderFailure` 的具体原因拼到稳定文案之后
   （`withCause`），前半句不变以免破坏按文案匹配的调用方。

### 前端

- `DiagnosesView` 的 `describeAIError` 按 `code` + 关键词给出下一步动作：截断 → 去调高上限；
  未知证据 → 展示校验详情；`AI_DISABLED` → 去开开关。此前一律覆盖成「AI 解释暂不可用，
  请检查 Provider 配置」，把唯一有用的信息扔掉了。
- 模型接入页在**配置齐全但开关关闭**时显示横幅 + 「立即启用」。这个状态最容易漏：保存成功、
  页面不报错，但 AI 全程不生效（本次就是在这个状态下接手的）。

## 二、配置校验

保存时新增两条，均为 400：

| 错误码 | 条件 | 为什么必须挡 |
|---|---|---|
| `BUDGET_BELOW_OUTPUT_LIMIT` | `max_output_tokens > daily_token_budget`（预算为 0 时不校验） | 每次调用都会因预留额度不足被拒，页面表现成「AI 一用就失败」，但原因在配置而非运行时 |
| `OUTPUT_LIMIT_TOO_LARGE` | `max_output_tokens > 32768` | 防误填 |

## 三、拓扑：装配采集器，并修掉一条从未成功过的写入路径

### 装配

`main.go` 此前是 `topology.NewService(nil, repo, nil)`——采集器为 nil，边表永远是空的，
图接口返回空图，界面上表现为「一张空白画布」，看起来像渲染 bug 而不是没有输入。

现在装配 `topology.NewCollector(kubernetesService, 100)` +
`topologyNamespaceLister`（把 `kubernetes.Service.Namespaces` 的分页列表适配成拓扑包需要的
`VisibleNamespaces`，避免 topology 依赖 apiquery 的分页细节）。

触发方式为显式调用 `POST /aiops/topology/collect`（`system_admin` / `operations_admin`，
写审计 `aiops.topology.collect`），而非后台定时器：操作者能看到「采到 119 条」，
而不是猜有没有定时任务在跑。

### 关键缺陷：`ON CONFLICT` 匹配不上部分唯一索引

首次采集读到 **119 条边，但一条都写不进去**，每个命名空间都报：

```
ERROR: there is no unique or exclusion constraint matching the ON CONFLICT specification (SQLSTATE 42P10)
```

根因：`uq_topology_edges_active` 是**部分唯一索引**（`... WHERE valid_to IS NULL`），
而 GORM 的 `clause.OnConflict` 只给了列名。Postgres 只有在 `ON CONFLICT` 带上同样的谓词时
才会选中该索引。`UpsertChangeEvent` 有同样问题（`uq_change_events_plan ... WHERE plan_id <> ''`）。

修法：补 `TargetWhere`（GORM v1.31.2 支持）：

```go
TargetWhere: clause.Where{Exprs: []clause.Expression{
    clause.Expr{SQL: "valid_to IS NULL"},
}},
```

修后同一次采集：**`edges_upserted: 119`，`partial: false`**，无错误。

这类 bug 单测发现不了——`internal/topology` 的测试用内存仓储，既不经过真实 SQL，也不知道
索引形状；而它只出现在「采集结果里的 errors 数组」中，接口整体仍是 200。

### 全局排查

其余 `OnConflict` 调用点逐个核对：`cluster_conditions`、`inspection_rule_overrides`、
`signals` 用的是普通唯一约束（可用）；`slo.InsertEvaluation`、correlation 与 alertroute 的
写入是 `DoNothing: true` 且未指定列——Postgres 的无目标 `DO NOTHING` 不受部分索引影响。

### 顺带修：拓扑完整性卡片读了三个不存在的字段

前端 `GraphCompleteness` 写的是 `coverage` / `missing_producers` / `stale_edges`，
后端实际返回 `state` / `truncated` / `remaining`。后果是「拓扑完整性」后面的徽章空白、
「陈旧边 条」里的数字永远为空。已按后端契约重写类型与模板：显示状态徽章 +
「N 个资源 · M 条关系」，截断时显示剩余条数；陈旧边数改在采集结果里给（`edges_closed`），
那才是它的准确位置。

> 注：`stale_edges` 这一概念后端并不存在，属于前端照想象补的字段。

## 验证

| 项 | 结果 |
|---|---|
| 真实模型生成（诊断 #32） | 201，引用 `E5,E2,E3,E4,E1`，1324/2585 tokens |
| 真实模型生成（诊断 #85） | 201，引用 `E1,E2,E2`（两条不同断言），1150/1545 tokens |
| 浏览器点击「生成解释」 | 25 秒内返回带引用的卡片，0 console 错误 |
| 截断诊断 | 改小上限后返回 `... output truncated at max_output_tokens=1200 (finish_reason=length); raise the limit in model settings` |
| 预算小于输出上限 | 400 `BUDGET_BELOW_OUTPUT_LIMIT` |
| 输出上限超 32768 | 400 `OUTPUT_LIMIT_TOO_LARGE` |
| 全集群采集 | `edges_seen=119, edges_upserted=119, partial=false` |
| 边表内容 | 119 行：RunsOn 39 / Owns 24 / Selects 21 / BackedBy 21 / ProtectedBy 14 |
| 图接口（default） | 22 节点 / 33 边 / `state=complete` |
| 浏览器点击「采集拓扑」 | 「采集完成：9 个命名空间，写入 119 条关系，关闭 0 条陈旧关系。」 |
| 全站巡检 | 36 路由 / 0 异常 |
| 单测 | topology / aiexplain / aisettings（新增 11 例）/ httpserver / automation / aiinvestigator / cmd-server 全绿；契约与权限矩阵测试通过 |

## 仍未解决

- **拓扑无后台采集循环**：需要手动点「采集拓扑」。要自动化需加 Lifecycle 定时器，
  并按集群/命名空间做增量；当前设计有意保留手动触发，便于演示时说明每一步。
- **AI 的语义正确性无法校验**：引用校验只能保证「引用的证据存在且可回溯」，
  不能保证「这条断言确实由该证据支撑」。#85 的第二条 `E2` 引用把「采集覆盖完整」这条
  证据用来支撑「规则评估状态 firing」的判断，ID 合法但归因可疑。这是词法校验的边界，
  对外描述时应把它当作已知局限写明，而不是宣称「引用全部正确」。
- **拓扑图布局在 20+ 节点时较密**：`k8s-node01/02` 附近节点标签有重叠，属于既有布局算法，
  本轮未改。
