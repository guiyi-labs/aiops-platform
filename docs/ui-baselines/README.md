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

| view | route | readyText | 鉴权 |
|---|---|---|---|
| login | `/login` | 进入控制台 | 否（时钟区掩码） |
| dashboard | `/` | 集群态势 | 是 |
| clusters | `/clusters` | 多集群管理 | 是 |
| workloads | `/workloads` | 资源工作台 | 是 |
| diagnoses | `/diagnoses` | 故障分析 | 是 |

## 新增视图基线

在脚本 `views` 数组中追加 `{ name, path, readyText, auth?, masks? }` 条目即可；
`auth: true` 会先以 `AIOPS_UI_USERNAME`/`AIOPS_UI_PASSWORD`（默认 admin/admin123）
幂等登录（已认证则跳过）。`viewportSet` 默认 Desktop 1440×900 / Mobile 375×812 两档。

## 已知边界

- 服务端健康状态文本（`/api/v1/health/live`）被假定稳定；若后端返回变化，
  需要把对应 DOM 追加到 `masks`。
- 对比基于 `sips` 转 BMP 后的 RGB 像素；sips 为 macOS 工具，跨平台基线对比
  需换用等价 PNG 解码（保留接口即可）。
