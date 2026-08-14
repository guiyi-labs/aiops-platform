# License：恢复 Apache-2.0 规范全文以修复 NOASSERTION 识别

- Date: 2026-08-14
- Status: Complete
- Scope: 仓库根 LICENSE 文件内容修复，使 GitHub 许可证识别恢复为 Apache-2.0

## Context

GitHub 将该仓库许可证识别为 `NOASSERTION`。经与 apache.org 规范文本全文 diff，发现仓库内 `LICENSE` 是被截断改写的版本：条款 4.4/4.6/4.7 被精简、APPENDIX 段整体缺失，licensee 无法将其匹配到规范 Apache License 2.0。来源：guiyi-labs 组织仓库优化清单第 2 项（开源许可证合规）。

## What Changed

### LICENSE
- `LICENSE`：替换为 Apache License 2.0 官方规范全文（202 行），并在文末保留 `Copyright 2025-2026 Guiyi Labs` 声明。

## Verification

- `diff` 对比：本地 LICENSE 与 `https://www.apache.org/licenses/LICENSE-2.0.txt` 规范文本，确认正文逐字一致（仅保留版权声明追加行）。
- `gh api repos/guiyi-labs/aiops-platform --jq .license.spdx_id`：推送后应返回 `Apache-2.0`（GitHub 重新识别可能有短暂延迟）。

## Risks / Notes

- 本次仅恢复规范文本，不改变 Apache-2.0 授权意图，无回退影响。