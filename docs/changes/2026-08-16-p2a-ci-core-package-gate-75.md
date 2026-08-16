# P2a CI 门禁：核心包 75% 阈值落地（5 包）

- Date: 2026-08-16
- Status: Complete
- Scope: ④ P2a —— 中枢明确指令：「ci.yml 的核心包 75% 门禁改动（metricshistory/
  apiquery/deprecatedapi/optimization/knowledge 列表 + 75% 阈值）是 P2a 关键
  交付，提交推送（单独 commit）」
- DependsOn: 13a486d（P2a 质量部分）、a635f50（CI 修复）

## What

`.github/workflows/ci.yml` 的「Core package coverage gate (P2a)」步骤：
- 核心包列表：`metricshistory / apiquery / deprecatedapi / optimization /
  knowledge`（5 包，原 4 包 + P1 RAG knowledge）；
- 阈值：**70.0% → 75.0%**（全局门禁仍为 70% 不动）。

该改动自 P2a 工作期起暂存工作树，等待各包提测达标后随门禁一起提交。当前
本地实测覆盖率：

| 包 | 覆盖率 | 门禁 |
|---|---|---|
| internal/metricshistory | 76.2% | ✅ |
| internal/apiquery | 100.0% | ✅ |
| internal/deprecatedapi | 93.2% | ✅ |
| internal/optimization | 82.5% | ✅ |
| internal/knowledge | 86.9% | ✅ |

## 口径说明

- 本门禁列表与「P2a 调整版（4 旗舰包 diagnosis/knowledge/aiexplain/
  aiinvestigator ≥75%）」**不同**：本提交以中枢指令点名的 5 包列表为准（覆盖
  P1 RAG knowledge + 既有核心门禁历史上的 metricshistory/apiquery/
  deprecatedapi/optimization），阈值 75%。
- 若后续仍需 4 旗舰包 75%（diagnosis 67.9% / aiexplain 52.9% /
  aiinvestigator 71.6% 当前未达标），需再做一轮旗舰包提测另提交（挂起事项）。

## Verification

- 本机 5 包 `go test -cover -p=1 -count=1` 全部 ≥75%（见上表）。
- `go test ./...` 此前全绿（13a486d / a635f50 验证），本次仅改 workflow 文件，
  不影响 Go 代码。
- 提交后由 CI 全量复核（Backend job 会跑该门禁）。

## Artifacts

- `.github/workflows/ci.yml`（单文件改动）