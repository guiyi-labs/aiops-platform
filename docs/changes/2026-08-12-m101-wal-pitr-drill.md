# M101-M90 本地数据轨：WAL 归档 + PITR + 备库 + 故障注入演练（8 场景确定性验证）

- Date: 2026-08-12
- Status: Complete
- Scope: M101 数据韧性第一步（路线 M90/M101 数据轨）——在本地 ephemeral PostgreSQL 上落地 WAL 归档 + PITR + 流式备库 + 故障注入演练脚本，覆盖无损 PITR、时间点恢复、缺 WAL 快速失败、迁移前逻辑备份、硬崩溃重启、备库优雅停机/追赶/故障切换、网络分区、归档目标故障，并输出实测 RPO/RTO 报告。

## Context

`docs/next-long-term-plan.md` M101 数据轨要求“WAL 归档、PITR、恢复点验证、迁移前备份和多副本优雅停机；明确 RPO/RTO 目标并以真实演练测量；故障注入覆盖数据库重启、网络中断、磁盘压力和不完整恢复点”。生产级 RPO/RTO 声明需要组织批准的基础设施（M90 授权轨），本记录交付**本地可重复演练**：同样的 5 个场景在任何装有 Docker 的环境上可再次运行，观测值入报告，作为 M101 本地证据链的第一块。

## What Changed

### 演练脚本

- `scripts/wal-pitr-drill.sh`：新增完整演练（`set -euo pipefail`，失败即退出；报告写入 `.artifacts/wal-pitr-drill/report-<run>.json`）。5 个场景：
  1. **无损 PITR**：源库开启 WAL 归档（`archive_timeout=1s`），seed 100 行 → `pg_basebackup` → prefail 50 行 → 记录故障点 → 停机 → 从 base backup + 归档 WAL 恢复到归档末尾并 promote，断言 seed=100 / prefail=50 / total=150。
  2. **时间点恢复**：after-marker 25 行 → 记录 marker 时间 → late 40 行 → 停机 → `recovery_target_time = marker` 恢复，断言 late=0 / after=25 / total≈175。
  3. **缺 WAL 快速失败**：记录故障 LSN 后停机，删除除最新段外的全部归档段，以 `recovery_target_lsn` 恢复——链路断裂导致恢复无法到达目标，容器快速退出；日志必须含 `could not restore` / `recovery ended before` / `No such file` 之一才判通过（防误报）。
  4. **迁移前逻辑备份**：全新集群 + 平台 41 个迁移 → premig 100 行 → `pg_dump -Fc` → ALTER TABLE（模拟迁移）→ postmig 10 行 → 停机 → WAL 恢复到迁移后状态（110 行 + 新列）→ `pg_restore --clean` 回灌 dump，断言恢复回 100 行且新列消失（逻辑备份独立于 WAL 的防线）。
  5. **硬崩溃故障注入**：SIGKILL 源容器（无优雅停机）→ 重启 → 崩溃恢复后断言已提交行全部存活（110→130，crash 行 20/20）；`pg_switch_wal()` 后归档段数增加，证明归档链路恢复。
  6. **流式备库**：新主库开启流复制（`pg_basebackup` + `standby.signal` + `primary_conninfo`，drill 网络追加 `host replication … scram-sha-256` hba 规则）→ 主库写入 130 行备库追平 → 备库优雅停机（`docker stop`）期间主库继续写入 → 备库重启后追平 150 行 → `pg_promote()` 故障切换，备库成为可写主库并持有全部 155 行。
  7. **网络分区**：`docker network disconnect` 把备库从 drill 网络隔离，主库继续写入 20 行；断言备库保持只读快照（150 行不增长）；`docker network connect` 重连后追平 170 行。专为此引入每轮独立的 user-defined 网络（`aiops-wal-net-*`），所有 drill 容器挂载其上。
  8. **归档目标故障（磁盘压力模拟）**：新源库（独立命名卷）以 `archive_command=false` 启动（归档目标不可用）→ 写入 130 行断言主库不受影响、`pg_stat_archiver.failed_count>0`（观测 5）→ 同一数据卷重建容器并恢复正确 `archive_command` → 断言数据完好（130 行）、WAL 积压排空进归档（3 段）→ base backup（130 行）+ 归档 WAL 恢复新增 20 行 → 无损 PITR 150/150。

### 演练脚本正确性修复（开发中发现的原始问题）

- `restore_target` 等待循环结果判定反写（ready/exited 混淆），重写为 `outcome=exited|ready|timeout` 三态判定。
- 容器内 `-c` 脚本的 heredoc 终止符带缩进导致 dash 不终止 heredoc、吞掉后续命令、容器静默退出；终止符改为顶格，内部单引号按 `'\''` 转义。
- `rm -rf` 挂载点（`/var/lib/postgresql/data` 为匿名卷）报 Device busy；改为 `find -mindepth 1 -exec rm -rf -- {} +` 清理内容。
- `printf "…%f %p…"` 把 `%f/%p` 当格式符报错；改用 heredoc 写 `restore_command`。
- PG17 archive recovery 在 WAL 耗尽且无 recovery target 时会自建新 timeline 完成恢复（非失败），因此“缺 WAL”场景必须携带一个位于缺失链之后的 `recovery_target_lsn` 才能触发快速失败；无损场景改以“恢复至归档末尾 + 显式 promote”表述。
- `pg_stat_archiver` 在重启后计数重置，不能跨重启比较；归档恢复信号改用归档目录文件计数 + 强制 `pg_switch_wal()`。

## Verification

- `bash -n scripts/wal-pitr-drill.sh`：syntax OK。
- 连续两次完整运行均 `exit=0`，8 场景断言全部命中：
  - 场景 1：restored 150 rows（seed=100 prefail=50），RTO=2.66s / 2.59s。
  - 场景 2：restored 175 rows，late writes excluded（late=0），RTO=2.49s / 1.39s。
  - 场景 3：missing-WAL failure detected in 2.33s / 2.34s（日志含 `recovery ended before configured recovery target was reached`）。
  - 场景 4：WAL 恢复到迁移后状态 110 行 + `migrated_at` 列；logical restore 回 100 行、列消失（dump 316K）。
  - 场景 5：SIGKILL 崩溃恢复 110→130 行（crash 行 20/20 存活），RTO=1.21s / 1.22s；归档段 5→6 / 6→7。
  - 场景 6：备库追平 130 行；优雅停机重启后追平 150 行；`pg_promote()` 后备库可写且持有 155 行（两轮一致）。
  - 场景 7：隔离期间备库保持 150 行快照（不增长），重连后追平 170 行（两轮一致）。
  - 场景 8：归档故障期间主库 130 行可写、`failed_count=5`；恢复后积压排空 3 段、无损 PITR 150/150（两轮一致）。
- 证据（本地 `.artifacts/`，gitignored）：`report-20260812-192609-6421a0.json`（4 场景首版）、`report-20260812-193556-eeee46.json` / `report-20260812-193644-d837b4.json`（5 场景两连跑）、`report-20260812-195020-f170bf.json` / `report-20260812-195145-1c0e5b.json`（6 场景两连跑）、`report-20260812-201159-aae073.json` / `report-20260812-201347-f2ce8e.json`（8 场景两连跑），对应 `wal-pitr-tmp-*` 目录保留 base backup/归档目录。

## Risks / Notes

- **本地轨边界**：网络中断（场景 7）与归档目标不可用（场景 8）已在本地模拟验证；真实磁盘压力（ENOSPC on pg_wal/数据盘）、跨主机 HA 与多副本生产拓扑仍依赖组织级基础设施（M90 授权轨），保持 Deferred。
- 观测 RPO/RTO 仅为本地环境实测值（archive_timeout=1s 下 RPO≤2s、RTO≈1.2–2.7s），不构成生产声明；生产 RPO/RTO 目标由组织批准后的演练给出。
- 演练使用 `pgvector/pgvector:0.8.1-pg17` 与平台真实迁移（41 个 `.up.sql`），未做任何生产数据操作；所有容器以 `io.guiyi.aiops.purpose=wal-pitr-drill` 标记并在退出时清理。
- 本记录与 `scripts/wal-pitr-drill.sh` 属 M101 本地数据轨第一步；远端推送因网络 SSL 失败暂缓（本地 9 个 M98–M100 提交 + 本记录待推）。
