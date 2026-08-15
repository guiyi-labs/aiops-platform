# 变更记录：答辩演示脚本草稿（三条主链路）

- **日期**：2026-08-15
- **里程碑**：M115
- **类型**：文档新增

## 内容

新增 `docs/thesis/demo-scripts-outline.md`，包含三条答辩演示主链路的结构化操作草稿：

1. **故障发现 → 规则诊断 → 受控修复**
   覆盖 CrashLoopBackOff 场景的多集群资源工作台、规则诊断（含可选 AI 解释）、
   dry-run rollout restart、幂等重放验证和审计追溯。

2. **事故响应闭环**
   覆盖告警抑制 → 事故工作区（SLA / MTTA / MTTR）→ Runbook 关联 →
   升级通知 → 复盘 Markdown 导出的完整闭环。

3. **SLO 超燃预警 + 告警降噪**
   覆盖 SLO burn rate 总览、Error Budget 下钻、告警路由去重配置、
   事件驾驶舱多信号对齐、指标历史下采样归档和降噪效果验证。

每条链路包含场景标题、入口、5–8 个具体操作步骤、证据锚点表格和翻车预案。

## 关联文件

- `docs/thesis/demo-scripts-outline.md`（新增）
- `docs/thesis/defense-demo-script.md`（已有 10 分钟脚本，本文件为其补充草稿）
- `docs/thesis/demo-environment.md`（演示环境准备流程）
