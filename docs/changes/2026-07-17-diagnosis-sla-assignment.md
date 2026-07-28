# 2026-07-17 Diagnosis SLA and Assignment

## Scope

- 新增 `000007_diagnosis_sla_assignment`，回填诊断 SLA，记录解决时间并创建追加式转派历史。
- 按 critical/high/warning/info 建立 1/4/24/72 小时固定 SLA，统一列表、详情和 Dashboard 的逾期口径。
- 诊断列表增加状态与逾期筛选，汇总增加逾期数量。
- 新增受控负责人目录和转派 API，只允许活跃系统/运维管理员，重复转派返回 409。
- 首次状态操作的自动认领与显式转派都追加负责人历史；转派事务保存操作者及前后负责人名称快照，并接入平台审计。
- 智能诊断页面增加 SLA 标签、截止时间、筛选、负责人表单和转派历史；Dashboard 增加逾期指标。

## Verification

- `go test ./...` 与服务端构建通过；新增 SLA 时限、负责人目录过滤和转派审计映射测试。
- `pnpm typecheck`、7 个 Vitest 文件共 14 项测试、`pnpm build` 通过。
- PostgreSQL 真实应用 `000007`，确认 `sla_due_at` 非空、`resolved_at` 和 `diagnosis_assignments` 存在。
- API 使用 3 条诊断验证：`overdue=true` 只返回 1 条；汇总 overdue=1；解决后写入 `resolved_at` 并停止计入逾期；重新打开沿用原截止时间。
- 可转派目录返回系统管理员和测试运维管理员；首次转派写入历史，重复转派返回 HTTP 409；审计同时记录 success 与 failure。
- 独立数据库联调确认首次状态流转自动认领时，同时产生 1 条处置活动和 1 条负责人历史。
- 浏览器验证逾期筛选、倒计时/逾期/已关闭标签、两名负责人选项、页面转派和两条交接历史；Dashboard 显示 1 条逾期诊断。
- 验证完成后删除测试集群、测试用户、诊断和审计数据，并停止临时前后端服务。

## Boundaries

- SLA 是固定平台策略，不自动跳过维护窗口，也不按租户、规则或工作日定制。
- 转派和状态操作不会执行 Kubernetes 修复，不会修改规则证据。
- 转派备注保存在诊断业务历史，但不会复制到平台审计详情。
- 当前不发送站内、邮件或即时通讯通知。

## Deferred

- 可配置 SLA 策略、暂停/豁免、升级路径和通知。
- 批量认领/转派/处置与值班计划。
- 人工反馈评估；AI Provider 与引用式解释已在后续变更 `2026-07-17-cited-ai-explanations.md` 完成。
