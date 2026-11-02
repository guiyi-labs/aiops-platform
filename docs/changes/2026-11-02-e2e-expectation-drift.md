# e2e-expectation-drift：修掉两条与界面/主题脱节的 e2e 断言

- Date: 2026-11-02
- Status: Complete
- Scope: `frontend/e2e/diagnosis-timeline.spec.ts`、`frontend/e2e/smoke.spec.ts` 各一条断言

## Context

`main` 上一次 CI 成功是 2026-10-26（`144c569`）。此后 18 个提交只在本地累积、从未经过 CI。
补上契约修复（见 `2026-11-02-fix-openapi-duplicate-schemas.md`）后，`Browser regression`
第一次真正跑到，暴露 4 个失败（2 个用例 × Desktop/Mobile）：

| 用例 | 失败断言 | 真实原因 |
|---|---|---|
| `diagnosis-timeline.spec.ts:102` | 期待 `原始证据 JSON` | 界面已改为 `原始证据（N 条 · 可追溯）` |
| `smoke.spec.ts:214` | `whiteDetected` 恒为 true | 白屏阈值与浅色主题底色相同 |

两者都**不是被测功能坏了**，而是断言与产品演进脱节——属于"测试没跟上"，不是回归。

### 一、证据摘要文案漂移

`DiagnosesView.vue` 的 `<summary>` 现为 `原始证据（{{ detail.evidence.length }} 条 · 可追溯）`，
比旧的「原始证据 JSON」更明确（点出条数与可追溯性）。用例仍按旧文案断言，必然失败。

### 二、白屏判据与设计底色撞车

`smoke.spec.ts` 在路由切换过程中逐帧采样 `.route-viewport` 的 `backgroundColor`，
旧判据是"三个通道都 `>=248` 即判为白屏"。而 `.theme-light .route-viewport`
的底色是字面量 `#f8fafb`，即 **RGB(248, 250, 251)**——三个通道全部 `>=248`。

也就是说：自 2026-10-30 默认主题切到 `light` 起，只要页面正常渲染，这个用例就**恒为假阳性**，
与是否真的闪白无关。它此前"通过"，只是因为那时默认主题不是 light。

## What Changed

### `frontend/e2e/diagnosis-timeline.spec.ts`

- `toContainText('原始证据 JSON')` → 分两条断言 `toContainText('原始证据')` 与
  `toContainText('可追溯')`。
- 不写死条数：条数属于数据，而这个 `<summary>` 必须点明"原始证据 + 可追溯"才是要守的意图。

### `frontend/e2e/smoke.spec.ts`

- 白屏判据由"三通道 `>=248`"收紧为 **"三通道 `>=253`"**，只认接近纯白（`#ffffff`）的闪现。
- 注释写明来由：248/250/251 是设计底色，不是白屏；否则默认浅色主题下该用例永远失败。
- 方向说明：这是**收紧**而非放宽——旧判据会把正常底色判成故障，新判据只认真正的白闪。

## Verification

- 本地完整 e2e（`PLAYWRIGHT_BROWSERS_PATH` 指向本机缓存、`BASE_URL=127.0.0.1:5173`）：
  **80 passed / 0 failed**（修复前该套件在 CI 上 4 failed / 76 passed）。
- 单跑这两个 spec：42 passed。
- 说明：本机 Playwright 版本要求 chromium 1243，缓存里只有 1234，故用软链把 1243 指到 1234
  完成本地复跑；这是本机环境适配，未改动仓库配置。

## 备注

- 本次只动测试断言，不动产品代码与样式，用户可见行为零变化。
- 顺带确认：CI 自 10-26 起长期未验证本地累积的提交，是这批次失败的制度性原因。
  后续宜避免长区间"只提交不推送"。
