# UI 截图基线（UI Baselines）

登录页（及后续视图）的**确定性像素级截图基线**与验证机制，用于前端视觉回归
（Track A · 关键页面截图基线）。机制复用本仓库既有的 CDP 截图思路
（见 `scripts/capture-ui-baselines.mjs`），但以"可重复像素对比"为目标。

## 目录结构

- `images/`：基线 PNG，命名 `<view>-<viewport>.png`
- `manifest.json`：清单（schema、commit、viewport、sha256、动态区掩码、diff 阈值）

## 如何使用

前置：登录页前端服务运行在 `http://127.0.0.1:18080`（或 `AIOPS_UI_BASE` 覆盖），
本机有 Google Chrome / Microsoft Edge。

```bash
# 重新生成基线（提交入库前使用）
node scripts/capture-ui-baselines.mjs

# 验证当前线上产物与基线一致（差异 ≤ 阈值则 PASS，退出码 0）
node scripts/capture-ui-baselines.mjs --verify
```

可选环境变量：

- `AIOPS_UI_BASE`：服务地址，默认 `http://127.0.0.1:18080`
- `AIOPS_UI_BASELINES_DIR`：基线目录，默认 `<repo>/docs/ui-baselines`
- `AIOPS_UI_DIFF_THRESHOLD`：允许的差异像素比例，默认 `0.002`（0.2%）
- `AIOPS_BROWSER_PATH`：浏览器可执行文件路径

## 确定性保证（可重复对比的前提）

1. **动效复位**：捕获时以 CDP `Emulation.setEmulatedMedia` 仿真
   `prefers-reduced-motion: reduce`，页面全部 CSS 动画与粒子画布循环复位。
2. **确定性随机**：注入固定种子的 `Math.random` 覆盖，使
   `ParticleNetwork` 等使用随机初始位置的画布每次渲染一致。
3. **动态区掩码**：真实本地时钟等不可固定文本，用 DOM 矩形记录到
   `manifest.json` 的 `masks`，对比时跳过该区域（登录页掩码为
   `.login-signal:nth-child(3) b` 的本地时间）。

## 已收录视图

精简版本由 `scripts/console-ui-views.mjs` 统一维护当前路由清单：
系统概览、资源、诊断记录、关联分析、拓扑、历史案例，以及集群/模型/用户/审计/账号设置和登录页。
每页包含桌面（1440×900）与移动（375×812）两种视口，共 24 张截图。

主菜单共五项，二级标签不作为独立业务模块计数。
截图使用真实本地 API 数据；诊断记录、审计、登录会话及资源状态变化会影响像素比较。
截图生成成功不代表未来任意时刻的真实数据都能通过像素一致性检查。

新增或合并界面时，应同步更新共享清单并重新生成基线。
精简前截图可从 `baseline-pre-slim-20261118` 标签恢复。
