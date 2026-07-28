# M10 Kubernetes 事件中心与控制台壳层统一

- 日期：2026-07-26
- 里程碑：M10
- 状态：Accepted
- 范围：Kubernetes Event API、独立事件中心、通知导航语义、Dashboard 共享壳层、响应式验收

## 目标

补齐面向真实 Kubernetes `core/v1 Event` 的统一观察入口，并消除
Dashboard 自行维护侧栏、顶栏和账号操作所造成的布局分叉。原有诊断通知
Outbox 页面继续保留，但明确命名为“通知投递”，避免与 Kubernetes 事件混淆。
本阶段只扩展只读查询和 UI，不增加 Kubernetes 写入权限或新的处置动作。

## 实现

1. 新增独立 `/events` 页面：
   - 对所有已登录角色开放，主导航“事件中心”直接进入该页面；
   - 支持集群、Namespace、事件类型、资源类型和资源名称筛选；
   - 按最新观察时间倒序展示 Normal/Warning、Reason、涉及资源、消息、
     累计次数和最后出现时间；
   - 摘要展示当前范围、Warning、Normal 和涉及资源数量；
   - 详情抽屉展示首次/最后时间、次数、Action、Reporting Component 和
     Reporting Instance。
2. 扩展 Kubernetes Event 合同：
   - 后端保留 `action`、`eventTime`、`reportingComponent`、
     `reportingInstance`、`series.count` 和 `series.lastObservedTime`；
   - Event 查询支持将资源名称作为 Kubernetes field selector 传入；
   - 前端时间和次数优先使用现代 Event series 字段，同时兼容旧字段。
3. 统一导航语义：
   - `/events` 为 Kubernetes 事件中心；
   - `/notifications` 改名“通知投递”，继续只对系统管理员和安全审计员
     显示，权限边界不变。
4. Dashboard 改用共享 `ConsoleLayout`：
   - 删除重复的侧栏、顶栏、账号安全和退出实现；
   - `ConsoleLayout` 新增 `actions` 插槽，Dashboard 通过插槽保留刷新入口；
   - 侧栏导航、当前用户、修改密码和退出行为与其他控制台页面统一。

## 真实环境与 UI 验收

保留的 `demo-kind-20260726-170601` 集群通过最小只读 ServiceAccount 凭据
读取到 11 条 Event，其中 Warning 5 条、Normal 6 条，涉及 3 个资源。
`Warning + crash-loop` 组合筛选精确返回 1 条；BackOff 详情显示 Pod、
Namespace、首次/最后时间、累计次数、kubelet 和上报实例。凭据以加密形式
保存在平台中，验证使用的临时 kubeconfig 已删除。

浏览器验收结果：

| 页面与尺寸 | 结果 |
|---|---|
| 事件中心 1440x1000 | 导航高亮、表格截断、详情抽屉和无页面横向溢出通过 |
| 事件中心 390x844 | 页面 `clientWidth=375`、`scrollWidth=375`；966px 表格只在 277px 事件面板内滚动 |
| 事件详情 390x844 | 抽屉宽 375px，8 个详情字段按单列排列，页面无横向溢出 |
| Dashboard 1440x1000 | 共享侧栏宽 203px，刷新/修改密码/退出按钮不重叠 |
| Dashboard 390x844 | 页面 `clientWidth=375`、`scrollWidth=375`，三个顶栏按钮均为 36px 且不重叠 |

事件中心和 Dashboard 控制台日志均无 error。验收结束后已移除临时视口覆盖。

## 自动验证

- 前端 typecheck 通过；
- 前端 Vitest 8 个文件、28 个用例通过；
- 前端生产构建通过，事件中心保持独立懒加载包；
- 后端 `internal/kubernetes` 与 `internal/httpserver` Go 1.25 测试通过；
- 新增前后端事件 API 合同测试，覆盖资源名称筛选和现代 Event 字段；
- PostgreSQL、Backend、Frontend Compose 服务均已重建并达到 healthy。

完整 `scripts/verify.ps1` 于 2026-07-27 09:41:04 +08:00 通过，耗时
142.28 秒：Go 1.25 Docker 工具链全包 vet/test/build、前端 typecheck、
8 个 Vitest 文件/28 个用例、生产构建、Compose 三服务健康、Kustomize
16/5/7/3 与前后端运行态 HTTP 检查全部通过。证据为
`.artifacts/verification/verify-20260727-094104.json`。

## 边界

- Event 数据继续实时读取 Kubernetes API，不进行全量持久化。
- 本阶段没有新增 watch、任意 YAML、任意 Patch、Pod Exec 或 WebShell。
- 通知投递 Outbox 的数据库、Worker、重试和审计合同均未改变。
- 保留的答辩集群、三条诊断记录和一条处置记录不因本阶段清理。
- 初始 Git commit 仍需人工确认作者身份和提交范围。
