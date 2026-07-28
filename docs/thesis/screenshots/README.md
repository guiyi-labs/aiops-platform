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

当前仓库尚无初始 Git commit，因此元数据必须标记为 `uncommitted-baseline`。创建人工确认的初始提交后，应重新运行采集器，生成绑定提交号的最终截图。
