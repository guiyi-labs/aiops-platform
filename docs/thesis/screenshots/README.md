# Thesis Screenshots

本目录保存当前系统自行采集的答辩截图，不包含参考项目图片。

| 文件 | 页面 |
|---|---|
| `01-dashboard.png` | Dashboard 总览、集群数和诊断统计 |
| `02-clusters.png` | Clusters 集群 Ready 状态与 Kubernetes 版本 |
| `03-workloads.png` | Workloads 真实 Namespace/Pod/Service 视图 |
| `04-diagnoses.png` | Diagnoses 三类规则历史与处置状态 |
| `capture-metadata.json` | 采集时间、视口、路由和源码修订状态 |

重新采集：

```powershell
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\demo-up.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\capture-thesis-screenshots.ps1
```

采集器使用系统已安装的 Microsoft Edge 或 Google Chrome 和标准 DevTools Protocol，不安装浏览器依赖。密码只进入临时进程环境和登录表单内存；浏览器 profile 位于已忽略的 `.artifacts`，采集结束后删除。

## 重采集状态（M34C 验收）

当前截图采集于 2026-07-26，`source_revision` 仍为 `uncommitted-baseline`，仅覆盖
Dashboard/Clusters/Workloads/Diagnoses 四个 M26 前页面，未包含 M27-M34 的新增主表面
（Alerts、Workload Protection、Namespace Posture、Node Maintenance、Restore 等视图）。

M32.5 与 M34C 均要求按已审核 revision 重新采集并扩展覆盖范围，但本地 TRAE 沙箱
不允许浏览器访问 TokenBroker/SystemTemp 缓存目录，采集脚本无法在沙箱内运行。该项
与 `docs/changes/2026-07-30-m32-formal-closure.md` §criterion 5 一致标记为
`deferred`，重采集需要在沙箱外执行 `demo-up.ps1` + `capture-thesis-screenshots.ps1`
并更新本目录与 `capture-metadata.json` 的 `source_revision`。
