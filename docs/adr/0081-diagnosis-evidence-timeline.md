# ADR 0081 — 诊断证据时间线与根因卡语义（M94）

- Date: 2026-08-10
- Status: Accepted
- Milestone: M94（诊断叙事与证据时间线）
- Supersedes: none
- Related: ADR 0005（先确定性诊断再 AI）、ADR 0007（append-only 诊断工作流）、
  ADR 0057（确定性 RCA）、ADR 0079（M81 insight 闭环）

## Context

M18/M43 的确定性诊断已经把"规则 → 证据 → 建议"落到 `diagnosis.Record`：
证据是 `{type, source, content}` 的原始 JSONB 集合，前端详情页把它当作
`JSON.stringify(content)` 的原始卡片展示。M94 要把诊断详情升级为"10 秒看清根因"，
需要两个新的展示语义：

1. **根因卡**：主结论、影响面、严重度、首次观察、当前状态、置信来源，必须在首屏出现。
2. **证据时间线**：资源状态、Event、日志摘录、告警、变更与自动化结果按统一时间轴呈现，
   而不是无时序的字段堆叠。

已有的约束（ADR 0005/0007/0057）要求：诊断记录仍是人工状态/SLA/反馈的事实来源；
时间线是展示投影，不改变存储、不伪造历史、不引入写路径。证据缺失必须显式可见，
不能把缺失数据渲染成健康或真实。

## Decision

引入纯函数投影 `diagnosis.WithNarrative(record) → record + timeline + root_cause_card`，
在服务层对返回给调用方的 `Record` 追加两个只读字段：

### 1. 证据时间（occurred_at）

每个时间线条目从原始证据 `content` 按证据类型提取规范化时间：

| 证据类型 | 时间键 | 缺失时 |
|---|---|---|
| `event` | `last_timestamp`（回退 `first_timestamp`） | 记 `observed_at` |
| `node_condition` / `pod_condition` | `last_transition_time` | 记 `observed_at` |
| `container_termination` | `finished_at` | 记 `observed_at` |
| `metric_sustained_breach` / `metric_evaluation_summary` | `window_end`（回退 `observed_at`） | 记 `observed_at` |
| 其余状态快照（`deployment_status`、`container_state`、`pod_status`、`endpoints`、`service_spec`、`hpa_*`、`ingress_backend`、`persistent_volume_claim`） | 无 | 记 `observed_at` |

时间统一输出 RFC3339 字符串；无法解析的时间键视为缺失，使用 `observed_at` 并保留原始值。

### 2. 来源与不可变引用（source / ref）

- `source` 保留原始证据的来源字段（如 `node.status.conditions`、`event/<name>`），
  作为溯源路径。
- `ref` 为确定性不可变引用：`diagnosis:<id>:evidence:<index>`，其中 index 是
  `diagnosis_evidence` 表按 id 排序后的稳定序号。Event 类型沿用
  `event/<metadata.name>` 便于与集群 Event 原始对象对应。

### 3. 完整性（integrity）

`integrity` = SHA-256（证据 content 的规范化 JSON）。Go `encoding/json` 对
map 按键排序序列化，因此同一证据在不同读取路径得到相同指纹；内容被篡改时
指纹不一致，前端可提示证据完整性校验失败。

### 4. 缺失语义（missing / missing_reason）

- `node_condition` 中 `status == "Missing"`（如 ReadyConditionMissing）→
  `missing=true`、`missing_reason=ReadyConditionMissing`。
- 事件时间键解析失败 → `occurred_at` 回退 `observed_at`，不标 missing（时间不可用 ≠ 证据缺失）。
- 证据完全不存在时时间线为空数组，根因卡 `key_evidence_refs` 为空，前端显示显式空态。

### 5. 根因卡（root_cause_card）

| 字段 | 来源 |
|---|---|
| `conclusion` | `record.Summary` |
| `severity` | `record.Severity` |
| `status` | `record.Status` |
| `first_observed_at` | 时间线最早 `occurred_at`，回退 `record.ObservedAt` |
| `confidence` | 固定 `deterministic` |
| `confidence_source` | `record.RuleID` |
| `resource` | `record.Resource` |
| `key_evidence_refs` | 前 5 条非 missing 证据的 `ref` |

### 6. 分类与只读约束

- 时间线条目带 `category`：`resource_state` / `event` / `log` / `alert` /
  `change` / `automation`。当前证据类型映射为 `resource_state`、`event`、`alert`
  （指标持续越界归为 alert）；log/change/automation 类别为未来扩展预留，投影
  对未知证据类型默认 `resource_state` 且不报错（向后兼容）。
- 投影是纯函数：不写数据库、不触达集群、不改变审计，不需要新角色或权限。
- 序列化顺序：按 `occurred_at` 升序；时间相同按 evidence index 升序，保证
  同输入确定性输出（ADR 0057 的可重放原则）。

## Consequences

- 诊断详情首屏可获得"根因卡 + 时间线"叙事，无需展开抽屉；旧记录无需迁移，
  投影在读取时按证据动态生成。
- 缺失与不可靠证据显式可见；AI 解释仍走 ADR 0058 的引用校验，时间线不替代
  AI 引用完整性检查。
- 不新增任何写路径、Pod exec、WebShell 或绕过确认的操作；所有现有安全边界
  与审计保持不变。
- M95 的 `FindingDetail v2` 可复用同一时间/来源/引用/完整性/缺失语义，作为
  统一证据展示模型的公共基础。