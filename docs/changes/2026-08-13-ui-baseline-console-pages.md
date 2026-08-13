# UI 截图基线 · 控制台关键页面扩展（鉴权捕获）

- Date: 2026-08-13
- Status: Complete
- Scope: `scripts/capture-ui-baselines.mjs` 鉴权流程 + 4 个控制台页面基线

## Context

`docs/changes/2026-08-13-ui-baseline-screenshots.md` 落地了登录页像素基线机制；
本改动把机制扩展到控制台关键页面（Track A「关键页面截图基线」的 Dashboard /
Clusters / Workloads / Diagnoses 首批），需要先通过平台登录（admin + 默认口令）。

## What Changed

- `scripts/capture-ui-baselines.mjs`：
  - 新增 `login(client)`：幂等登录——导航 `/login` 后若路由守卫已重定向
    （`location.pathname !== '/login'`）则跳过表单；否则以原生 value setter +
    `input` 事件填充 `AIOPS_UI_USERNAME` / `AIOPS_UI_PASSWORD`（默认 admin/admin123）
    并 `requestSubmit`，等待登录成功。修复"已登录再访问 /login 被重定向导致超时"。
  - `views` 增加 4 个 `auth: true` 页面（readyText 均取自当前线上实测文案）：
    - Dashboard `/` → `集群态势`
    - Clusters `/clusters` → `多集群管理`
    - Workloads `/workloads` → `资源工作台`
    - Diagnoses `/diagnoses` → `故障分析`
- 基线产物：`docs/ui-baselines/images/{dashboard,clusters,workloads,diagnoses}-{desktop-1440x900,mobile-375x812}.png`
  + manifest 10 条（登录 2 + 控制台 4×2）。

## Verification

`node scripts/capture-ui-baselines.mjs --verify`（同环境重截图对比）：

| 视口/页面 | 结果 |
|---|---|
| login@1440x900 | PASS diff 0.000%（时钟区掩码） |
| login@375x812 | IDENTICAL |
| dashboard / clusters / workloads / diagnoses × desktop/mobile | 全部 IDENTICAL（sha256 一致） |

- 注：控制台页面当前为**无集群空态**（demo 后端无已启用集群、无诊断记录），
  服务端数据不动因此像素确定；若后端数据变化（接入集群/新增告警等），
  需按 README「已知边界」更新掩码或重建基线。
- 构建与门禁：仅改 `scripts/*.mjs` 与文档，行内 `node --check` 通过；
  未触碰 `frontend/` 源码与后端。

## Follow-up

- 其余视图（告警规则、事件中心、拓扑、优化中心、事故工作空间、审计日志等）按同
  模式追加 `views`；带真实数据页面的动态区（时间列等）需补 `masks`。
- 基线 CI 化：`--verify` 接入 pipeline（Linux 需替换 `sips` 为纯 Node PNG 解码）。
