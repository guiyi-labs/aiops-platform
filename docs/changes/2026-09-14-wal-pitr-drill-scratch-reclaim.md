# WAL/PITR 演练临时工作目录压缩归档并回收本地磁盘

- Date: 2026-09-14
- Status: Complete
- Scope: `scripts/wal-pitr-drill.sh` 在 2026-08-12 七次演练中遗留的 7 个临时工作目录
  （`.artifacts/wal-pitr-tmp-*`，合计 1.14 GiB）压缩归档后回收；不改脚本、不改 2026-08-12 的原始记录。

## Context

`scripts/wal-pitr-drill.sh` 第 43 行以 `.artifacts/wal-pitr-tmp-$RUN_ID` 作为临时工作目录，
用于存放该次演练生成的 PostgreSQL base backup、WAL 归档目录与 `pre-migration.dump`。
脚本结尾输出报告后**不清理该目录**，因此每跑一次演练就永久留下一个
约 114–242 MiB 的工作目录。

2026-08-12 的 M101 演练共跑了 7 次（4 场景首版 → 8 场景两连跑），累计留下 7 个目录、
12,220 个文件、1,225,253,890 字节（1.14 GiB）。当日记录
`docs/changes/2026-08-12-m101-wal-pitr-drill.md` 第 46 行只说明这些目录
「保留 base backup/归档目录」，未给出保留期限或回收方式，因而长期堆积。

需要澄清的是：**该次演练的可引用证据并不在这些目录里**。每个场景的结论、行数、
RTO 等观测值都写在 `.artifacts/wal-pitr-drill/report-<run_id>.json`（共 7 份，合计 28 KiB），
与 7 个临时目录按 `run_id` 一一对应。临时目录中只包含 PostgreSQL 的物理备份与归档段。

## What Changed

### 压缩归档

- 7 个目录打包为 `.artifacts/wal-pitr-drill/wal-pitr-tmp-archive-20260812.tar.zst`：
  `tar -cf - | zstd -T0 -12`，压缩后 **39,054,550 字节（37.25 MiB）**，压缩比 **3.11%**。
- 压缩比极高是因为 7 次演练在同一镜像、同一 schema 上重复执行，WAL 段与 base backup
  跨次高度重复。这也说明「归档后再回收」是一项成本极低的安全网。

### 回收原始目录

- 7 个原始目录及归档校验时产生的临时解出副本已移入系统废纸篓（可恢复），
  未做不可逆删除。`.artifacts` 体积由 4.4 GiB 降至 2.2 GiB。
- 废纸篓中的副本在用户确认归档无误后清空，即可实际回收 1.14 GiB。

### 未改动

- `scripts/wal-pitr-drill.sh` 未改：修改脚本会改变既有演练的执行环境，
  与「不篡改历史证据链」的约定冲突。根因修复留作独立条目（见 Risks / Notes）。
- `docs/changes/2026-08-12-m101-wal-pitr-drill.md` 未改：归档纪律为「只追加、不改历史」，
  本记录即为其第 46 行所述「保留」状态的后续处置说明。
- 未在本记录范围内：`.artifacts/offline-install-drill`（1.6 GiB）与 `.artifacts/oidc-drill`（494 MiB）。

## Verification

归档前后做了逐文件内容比对，而非仅比对文件数：

| 检查项 | 结果 |
|---|---|
| 归档前基线 | 12,220 文件 / 1,225,253,890 字节 |
| `zstd -t` 压缩流完整性 | OK |
| 解出后文件清单 `diff` | LIST IDENTICAL |
| 解出后逐文件 SHA-256 `diff` | **HASH IDENTICAL**（12,220 条全等） |
| 归档后 `.artifacts` 体积 | 4.4 GiB → 2.2 GiB |

即归档包与原目录在文件集合与逐文件内容两个层面完全等价，回收不损失任何数据。

## Risks / Notes

- **根因未修**：`wal-pitr-drill.sh` 仍会在每次演练后留下 `wal-pitr-tmp-$RUN_ID` 目录。
  建议后续为脚本加收尾清理（`trap ... EXIT` 或改用 `mktemp -d`），或在报告 JSON 中记录
  「临时目录已回收」以闭环。本次刻意未与脚本修改合并，便于该改动单独评审。
- **保留策略缺口**：原记录仅写「保留」而未写保留多久。此类物理备份目录的建议策略是
  「归档为单一压缩包 + 保留报告 JSON」，压缩比通常在一个数量级以内。
- 被回收的对象是本地 gitignored 数据（`.artifacts/` 不随仓库分发），因此本记录不改变任何
  仓库内可构建产物；记录本身入库仅为归档留痕，使「第 46 行所述目录后来怎么了」有据可查。
