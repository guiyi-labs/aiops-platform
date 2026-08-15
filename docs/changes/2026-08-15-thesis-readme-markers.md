# 修复：thesis 截图 README 恢复交付资产门禁英文标记

- Date: 2026-08-15
- Status: Complete
- Scope: 修复 `TestDeliveryAssetsCoverVerificationAndThesisMaterials` 回归。

## Context

M115 答辩截图基线刷新（`a4d13ef`）把 `docs/thesis/screenshots/README.md`
表格中的英文标记 Clusters/Diagnoses 替换为中文描述，导致 delivery-assets
门禁找不到必需标记（README 要求 Dashboard/Clusters/Diagnoses）。

## What Changed

`docs/thesis/screenshots/README.md` 表格单元格恢复英文标记：

- `02-clusters.png` → `Clusters 集群接入（…）`
- `04-diagnoses.png` → `Diagnoses 智能诊断（…）`

## Verification

- `go test -run TestDeliveryAssetsCoverVerificationAndThesisMaterials ./internal/deployment/`：PASS。
- `go test -p=1 ./...`：72 包全绿。

## Risks / Notes

- 无。其余表格行保持中文描述不变。
