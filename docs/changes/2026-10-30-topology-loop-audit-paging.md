# 拓扑周期采集、审计日志分页与契约空值清理

日期：2026-10-30
范围：`backend/cmd/server`、`backend/internal/{config,audit,capability,signal,httpserver}`、`frontend/src`（审计页）、`compose.yaml`

## 一、拓扑改为周期采集（补齐上一轮遗留）

上一轮的拓扑只有「启动预热一次 + 界面手动点『采集拓扑』」。缺口很明确：预热之后
集群里新增的 Deployment/Pod 不会进图，除非有人记得去点按钮。

现在改为：启动预热一次，之后按 `TOPOLOGY_COLLECT_INTERVAL`（默认 `5m`）周期刷新。

- 设为 `0` 时退化为「只预热」——采集要读 8 类资源 × 每个命名空间，对 api_server
  与大盘都有成本，必须允许关掉周期采集。
- 全程异步、失败只记日志、绝不阻塞启动或优雅退出；循环监听 `backgroundContext`。
- 日志带 `phase` 字段（`warm-up` / `periodic`），便于区分「启动补采」与「周期刷新」。

实测（临时设 30s）：预热 1 次 + 每 30s 一轮，两个集群各 119/36 条边，单轮约 0.5s，
`partial=false`。恢复默认 5m 后只见到 warm-up，符合预期。

## 二、审计日志：9074 条记录只能看最新 100 条

安全审计页顶部写着「操作记录 · 9074」，列表却固定 `limit=100` 且**没有任何翻页入口**。
超出部分只能靠 CSV 导出或加筛选条件才能看到——这是功能缺口，不是显示问题。

后端 `audit.Filter` 只有 `Limit`，仓储 SQL 只有 `LIMIT ?`，压根没有分页能力。

### 用游标而不是 offset

审计表是只追加的高频表：

- 翻页期间新审计记录会插到最前面，`OFFSET` 会让相邻两页出现**重复或遗漏**；
- `ORDER BY created_at DESC` 的深 `OFFSET` 每页都要扫过前面所有行。

所以按自增 id 做游标：`before_id` 取上一页最后一条的 id，SQL 加 `id < ?`，
响应新增 `next_before_id`（**只在取满一页时**给出，避免到底后还提示「还有更多」）。

`total` 仍是「符合筛选条件的总记录数」，不随游标变化——界面用它显示「已加载 N / 总数」。

### 前端

审计列表底部加控制条：「已加载 N / 总数」+「加载更多」/「已到底」。
`loadEntries` 重置游标，`loadMore` 追加并做一次 id 去重（游标本身是开区间不会重复，
去重是防止将来换实现时出现重复渲染）。

实测：初始 100 条 → 点一次变 200 条；API 层面第 1 页 `16421..16417`、
第 2 页 `16416..16412`，严格递减、无重叠。

## 三、契约空值清理（系统性排查）

此前修过 4 处「列表返回 `items: null` 导致前端整页白屏」。这类问题单测发现不了
（单测直接比对结构体，不经过 JSON 序列化），所以写了个扫描脚本：
遍历 OpenAPI 里全部 **201 条 GET 路径**，把路径参数替换成可用值实际调用，
递归检查响应中「键名像集合、值为 null」的字段。

命中 3 处，逐条判断：

| 接口 | 字段 | 判定 |
|---|---|---|
| `GET /incidents/metrics` | `mta_seconds` / `mttr_seconds` / `first_assigned_seconds` | **不改**：时长指标为 `null` 表示「无数据」，改成 0 会造成误导 |
| `GET /capability/providers` | `items[].dependencies` / `cluster_roles` | **修**：Registry 用 `append([]string(nil), …)` 复制，描述符未设置时仍是 nil slice |
| `GET /aiops/signals/catalog` | `items[].severity_policy.mappings` | **修**：多数描述符只设 `Fallback`，nil map 序列化成 null |

修法选择「在输出处统一归一」而不是「逐个补空值」：

- capability：7 处 `append([]string(nil), …)` 改为 `append([]string{}, …)`
- signal：给 `SeverityPolicy` 加 `MarshalJSON`，nil map 输出 `{}`

理由是新增 provider / 描述符时不会再犯同样的错——逐个补空值只能治当前这几条。

扫描脚本留在 `/tmp/scan_null_arrays.py`（一次性诊断工具，未入库）。

## 验证

| 项 | 结果 |
|---|---|
| 拓扑周期采集 | 30s 周期下每轮 119 + 36 条边，`partial=false`，单轮约 0.5s |
| 审计游标分页 | 两页 id 严格递减无重叠；`total` 稳定 |
| 审计前端 | 100 → 200 条，0 console 错误 |
| null 字段扫描 | 201 条 GET → 命中 3 → 修 2，剩 1 为语义正确的「无数据」 |
| `ui-gate` | 4/4 PASS |
| 全站巡检 | 36 路由 / 0 异常 |
| 单测与契约 | audit / capability / signal / httpserver 全绿 |

## 仍未做（有意的取舍）

- **移动端顶栏 112px**：移动端是「标题 + 操作区」两行布局，占 812px 高的 14%。
  压成一行需要把次要操作（主题/账号/退出）收进溢出菜单，属设计改动而非缺陷修复。
- **36×36 图标按钮**：WCAG 2.5.8（AA）要求 24×24，已满足；44×44 是 AAA 级建议。
  提高到 44 会让移动端顶栏更高，与上一条冲突，需要一起设计。
- **像素基线对数据驱动页面仍不可靠**：靠 per-view 阈值缓解，彻底解法是给捕获注入
  固定 fixture，见 `2026-10-30-ui-gate-green.md` 的说明。

## 四、诊断列表：`total` 是假的，且无法翻页

审计修完后顺手核对诊断列表，发现契约层面的问题：

```go
c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
```

- `total` 被赋成 **本页条数**，不是总数；
- `remaining` **恒为 0**。

后果：界面若显示「共 N 条」，N 其实是「这一页有几条」；任何基于 `remaining` 的分页
判断都会立刻认为「已经到底」。诊断又是在持续增长的（规则命中就新增），实测已经从
36 涨到 41，`limit=50` 迟早被撑满而静默截断。

### 改动

- 仓储抽出 `listConditions`，让「本页取数」与「总数统计」共用同一套 WHERE ——
  两处各写一份是它们悄悄分叉的常见原因。
- 新增 `Repository.Count`（`SELECT COUNT(*)`），`total` 与 `remaining` 由它得出。
- `ListFilter` 加 `BeforeID` 游标，与审计同一套语义。
- 响应新增 `next_before_id`；**只在取满一页时给出**。

### 一个必须写下来的坑：`remaining` 不能用来判断「还有下一页」

`remaining = total - len(items)`。游标分页下每页条数都差不多，最后一页的
`remaining` 仍然大于 0——如果前端拿它判断，会永远显示「加载更多」，点了返回空列表。
所以判断依据必须是 `next_before_id` 是否存在（即「本页是否取满」）。

实测：`limit=5` 时返回 `next_before_id=91`；`limit=100` 取回全部 41 条时该字段缺省。

### 前端

诊断列表底部加「已加载 N / 总数」+「加载更多」/「已到底」，与审计页一致。
实测显示「已加载 41 / 41 条 · 已到底」——41 是真实总数（此前会显示成 50）。

## 五、仍未做的事项（有意的取舍）

- **事故列表（`/incidents`）同样没有分页**，但当前只有 2 条，且其 API 形态与诊断不同
  （带 summary 聚合），不在本轮范围。
- **事件列表**明确标注「最多展示最近 100 条」：Kubernetes Event 只保留 1 小时，
  100 条足够，属有意的产品说明而非缺陷。
