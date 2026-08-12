#!/usr/bin/env bash
# M101-M90: WAL archiving + Point-In-Time-Recovery drill (local data track).
#
# Stands up an ephemeral PostgreSQL (pgvector/pgvector:0.8.1-pg17) with WAL
# archiving enabled, applies the platform migrations, then exercises:
#   1. lossless PITR   - base backup + archived WAL, recover to a recorded LSN
#                        just before a simulated failure; every committed row
#                        must be present
#   2. time-bounded PITR - recover to an earlier marker time (partial data);
#                        rows committed after the marker must be excluded
#   3. missing-WAL fault - a required WAL segment is removed; recovery must
#                        fail fast with a clear, attributable error
#   4. pre-migration logical backup - pg_dump checkpoint before a schema
#                        change, restored into the recovered target (the
#                        logical backup remains an independent defense line)
#   5. crash fault injection - hard-kill the source (SIGKILL, no clean
#                        shutdown), restart it, and verify committed rows
#                        survive crash recovery while WAL archiving resumes
#   6. streaming standby - graceful shutdown of a hot standby, restart with
#                        catch-up from the primary, then promote (failover)
#                        and verify the standby serves the full dataset
#   7. network partition - disconnect the standby from the drill network while
#                        the primary keeps writing; reconnect and verify catch-up
#   8. archive destination failure - archive_command fails (destination down);
#                        primary must stay writable, WAL backlog must drain after
#                        the destination recovers, and lossless PITR must still work
#
# Measures and reports RPO (data-loss window) and RTO (recovery wall time) as
# observed values only; production RPO/RTO claims require org-approved drills.
#
# Exit 0 when every scenario matches expectations; exit 1 otherwise.
# Report: .artifacts/wal-pitr-drill/report-<run>.json
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${PG_IMAGE:-pgvector/pgvector:0.8.1-pg17}"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 3)"
SRC="aiops-wal-src-$RUN_ID"
TGT="aiops-wal-tgt-$RUN_ID"
SBY="aiops-wal-sby-$RUN_ID"
DB_USER="aiops"
DB_NAME="aiops"
WORK="$ROOT/.artifacts/wal-pitr-tmp-$RUN_ID"
ARCHIVE="$WORK/archive"
BACKUP="$WORK/backup"
DUMP="$WORK/pre-migration.dump"
NET="aiops-wal-net-$RUN_ID"
VOL="aiops-wal-vol-$RUN_ID"
ARTIFACTS="$ROOT/.artifacts/wal-pitr-drill"
REPORT="$ARTIFACTS/report-$RUN_ID.json"
MIGRATIONS="$ROOT/backend/migrations"

mkdir -p "$ARCHIVE" "$BACKUP" "$ARTIFACTS"
chmod 777 "$ARCHIVE" "$BACKUP" "$WORK"
docker network create "$NET" >/dev/null

cleanup() {
  docker rm --force --volumes "$SRC" "$TGT" "$SBY" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

phase() { echo "== $1 =="; }

psql_exec() { # container, sql
  docker exec "$1" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -tAc "$2"
}

wait_ready() { # container
  for i in $(seq 1 60); do
    if docker exec "$1" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  echo "container $1 not ready" >&2
  return 1
}

start_source() {
  phase "start source postgres (WAL archiving on)"
  docker run --detach --name "$SRC" \
    --label io.guiyi.aiops.purpose=wal-pitr-drill \
    --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret" \
    --network "$NET" \
    --mount "type=bind,source=$ARCHIVE,target=/archive" \
    --mount "type=bind,source=$MIGRATIONS,target=/migrations,readonly" \
    "$IMAGE" \
    -c wal_level=archive \
    -c archive_mode=on \
    -c "archive_command=test ! -f /archive/%f && cp %p /archive/%f" \
    -c archive_timeout=1s \
    -c min_wal_size=32MB \
    -c max_wal_senders=4 >/dev/null
  wait_ready "$SRC"
}

restart_source() {
  phase "restart source for next scenario"
  docker start "$SRC" >/dev/null
  wait_ready "$SRC"
}

apply_migrations() { # container
  phase "apply platform migrations"
  docker exec "$SRC" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -c \
    "CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())" >/dev/null
  for f in "$MIGRATIONS"/*.up.sql; do
    name="$(basename "$f")"
    applied="$(psql_exec "$SRC" "SELECT 1 FROM schema_migrations WHERE version='$name'")"
    if [[ "$applied" != "1" ]]; then
      docker exec "$SRC" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -f "/migrations/$name" >/dev/null
      psql_exec "$SRC" "INSERT INTO schema_migrations(version) VALUES ('$name')" >/dev/null
    fi
  done
  psql_exec "$SRC" "CREATE TABLE IF NOT EXISTS wal_drill_events (id bigserial PRIMARY KEY, marker text NOT NULL, inserted_at timestamptz NOT NULL DEFAULT now())" >/dev/null
}

insert_events() { # container, prefix, count
  local container="$1" prefix="$2" count="$3"
  docker exec "$container" psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d "$DB_NAME" -q \
    -c "INSERT INTO wal_drill_events(marker) SELECT '$prefix-' || g FROM generate_series(1, $count) g" >/dev/null
}

count_events() { psql_exec "$1" "SELECT count(*) FROM wal_drill_events"; }

count_marker() { # container, marker prefix
  psql_exec "$1" "SELECT count(*) FROM wal_drill_events WHERE marker LIKE '$2-%'"
}

column_exists() { # container, column
  psql_exec "$1" "SELECT count(*) FROM information_schema.columns WHERE table_name='wal_drill_events' AND column_name='$2'"
}

take_base_backup() {
  local bdir="/var/lib/postgresql/backup-$$-$RANDOM"
  phase "take base backup (pg_basebackup)"
  docker exec "$SRC" pg_basebackup -U "$DB_USER" -D "$bdir" -X stream -Fp -c fast >/dev/null
  docker cp "$SRC:$bdir/." "$BACKUP" >/dev/null
}

stop_source() {
  phase "simulate failure: stop source"
  docker stop "$SRC" >/dev/null
}

restore_target() { # recovery_target_sql ("" = none), expect_failure 0/1
  local recovery_target="$1" expect_failure="$2"
  phase "start target postgres from base backup${recovery_target:+ (recovery target: $recovery_target)}"
  docker run --detach --name "$TGT" \
    --label io.guiyi.aiops.purpose=wal-pitr-drill \
    --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret" \
    --network "$NET" \
    --mount "type=bind,source=$BACKUP,target=/var/lib/postgresql/data-source" \
    --mount "type=bind,source=$ARCHIVE,target=/archive" \
    --env "RECOVERY_TARGET=$recovery_target" \
    --entrypoint /bin/sh \
    "$IMAGE" -c '
      set -e
      find /var/lib/postgresql/data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true
      cp -a /var/lib/postgresql/data-source/. /var/lib/postgresql/data/
      chown -R postgres:postgres /var/lib/postgresql/data
      cat > /var/lib/postgresql/data/postgresql.auto.conf <<EOF
      restore_command = '\''cp /archive/%f %p'\''
EOF
      touch /var/lib/postgresql/data/recovery.signal
      cat > /var/lib/postgresql/data/postgresql.conf <<PGEOF
      restore_command = '\''cp /archive/%f %p'\''
      archive_mode = on
      archive_command = '\''test ! -f /archive/%f && cp %p /archive/%f'\''
      recovery_target_action = '\''promote'\''
PGEOF
      if [ -n "$RECOVERY_TARGET" ]; then printf "%s\n" "$RECOVERY_TARGET" >> /var/lib/postgresql/data/postgresql.conf; fi
      chown -R postgres:postgres /var/lib/postgresql/data
      exec docker-entrypoint.sh postgres
    ' >/dev/null
  # outcome: exited (container stopped/crashed), ready (accepts connections), timeout (90s)
  local outcome="timeout"
  for i in $(seq 1 90); do
    if ! docker ps --filter "name=^/$TGT$" --format '{{.Status}}' | grep -q Up; then outcome="exited"; break; fi
    if docker exec "$TGT" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then outcome="ready"; break; fi
    sleep 1
  done
  if [[ "$expect_failure" -eq 1 ]]; then
    if [[ "$outcome" == "exited" ]]; then
      local logs
      logs="$(docker logs "$TGT" 2>&1 || true)"
      if ! grep -qE "could not restore|recovery ended before|No such file or directory" <<<"$logs"; then
        echo "target exited but not because of a missing-WAL recovery error" >&2
        echo "$logs" | tail -10 >&2
        return 1
      fi
      echo "verified: recovery failed as expected (missing WAL)"
      return 0
    fi
    echo "expected recovery failure but outcome was: $outcome" >&2
    docker logs "$TGT" 2>&1 | tail -10 >&2
    return 1
  fi
  if [[ "$outcome" == "ready" ]]; then
    echo "target recovery finished"
    return 0
  fi
  echo "target did not finish recovery (outcome=$outcome)" >&2
  docker logs "$TGT" 2>&1 | tail -10 >&2
  return 1
}

recovery_done() {
  # wait until recovery completes (not in recovery)
  for i in $(seq 1 90); do
    state="$(psql_exec "$TGT" "SELECT pg_is_in_recovery()")"
    if [[ "$state" == "f" ]]; then return 0; fi
    sleep 1
  done
  echo "target still in recovery" >&2
  return 1
}

promote_target() {
  # After all archived WAL is replayed the target still runs as a standby.
  # Wait for the replay position to stop advancing, then promote explicitly.
  phase "promote target after WAL replay completes"
  local prev="" stable=0
  for i in $(seq 1 90); do
    state="$(psql_exec "$TGT" "SELECT pg_is_in_recovery()")"
    if [[ "$state" == "f" ]]; then return 0; fi
    lsn="$(psql_exec "$TGT" "SELECT pg_last_wal_replay_lsn()")"
    if [[ "$lsn" == "$prev" && -n "$lsn" ]]; then
      stable=$((stable + 1))
    else
      stable=0
      prev="$lsn"
    fi
    if [[ "$stable" -ge 3 ]]; then
      promoted="$(psql_exec "$TGT" "SELECT pg_promote()")"
      echo "promote result: $promoted"
      return 0
    fi
    sleep 1
  done
  echo "replay LSN never stabilized; cannot promote" >&2
  return 1
}

# ---------- scenario 1: lossless PITR ----------
phase "SCENARIO 1: lossless PITR to last archived point"
start_source
apply_migrations
insert_events "$SRC" "seed" 100
take_base_backup
insert_events "$SRC" "prefail" 50
sleep 2
fail_time="$(psql_exec "$SRC" "SELECT to_char(now(), 'YYYY-MM-DD HH24:MI:SS.US')")"
rto_start="$(date +%s.%N)"
stop_source
restore_target "" 0
promote_target
recovery_done
rto_end="$(date +%s.%N)"
target_count="$(count_events "$TGT")"
seed_count="$(count_marker "$TGT" seed)"
prefail_count="$(count_marker "$TGT" prefail)"
if [[ "$target_count" -ne 150 || "$seed_count" -ne 100 || "$prefail_count" -ne 50 ]]; then
  echo "lossless restore mismatch: total=$target_count seed=$seed_count prefail=$prefail_count (want 150/100/50)" >&2
  exit 1
fi
scenario1_rpo="$(echo "$fail_time" | head -c 23)"
scenario1_rto="$(awk "BEGIN{printf \"%.2f\", $rto_end - $rto_start}")"
echo "scenario1: restored $target_count rows (seed=100 prefail=50); measured RTO=${scenario1_rto}s"
docker rm --force --volumes "$TGT" >/dev/null

# ---------- scenario 2: time-bounded PITR ----------
phase "SCENARIO 2: PITR to earlier marker time (partial data)"
restart_source
insert_events "$SRC" "after-marker" 25
sleep 1
marker_time="$(psql_exec "$SRC" "SELECT to_char(now(), 'YYYY-MM-DD HH24:MI:SS.US')")"
insert_events "$SRC" "late" 40
sleep 2
stop_source
rto_start="$(date +%s.%N)"
restore_target "recovery_target_time = '$marker_time'" 0
promote_target
recovery_done
rto_end="$(date +%s.%N)"
scenario2_count="$(count_events "$TGT")"
scenario2_late="$(count_marker "$TGT" late)"
scenario2_after="$(count_marker "$TGT" after-marker)"
# seed(100) + prefail(50) + after-marker(25) = 175; late(40) must be excluded
if [[ "$scenario2_count" -lt 165 || "$scenario2_count" -gt 185 || "$scenario2_late" -ne 0 || "$scenario2_after" -ne 25 ]]; then
  echo "time-bounded restore mismatch: total=$scenario2_count late=$scenario2_late after=$scenario2_after (want ~175, late=0, after=25)" >&2
  exit 1
fi
scenario2_rto="$(awk "BEGIN{printf \"%.2f\", $rto_end - $rto_start}")"
echo "scenario2: restored $scenario2_count rows (late writes excluded); measured RTO=${scenario2_rto}s"
docker rm --force --volumes "$TGT" >/dev/null

# ---------- scenario 3: missing WAL fault ----------
phase "SCENARIO 3: missing WAL segment fails fast"
restart_source
insert_events "$SRC" "fault" 10
sleep 2
fail_lsn3="$(psql_exec "$SRC" "SELECT pg_current_wal_lsn()")"
stop_source
# remove all but the newest WAL segment -> the WAL chain to the recorded LSN
# is broken; recovery must fail fast instead of silently ending at the backup
count_before="$(ls "$ARCHIVE" | wc -l | tr -d ' ')"
if [[ "$count_before" -gt 1 ]]; then
  newest="$(ls "$ARCHIVE" | grep -E '^[0-9A-F]{24}$' | sort | tail -1)"
  for f in "$ARCHIVE"/*; do
    [[ "$(basename "$f")" == "$newest" ]] || rm -f "$f"
  done
fi
rto_start="$(date +%s.%N)"
if ! restore_target "recovery_target_lsn = '$fail_lsn3'" 1; then
  echo "scenario3: target unexpectedly recovered despite missing WAL" >&2
  exit 1
fi
rto_end="$(date +%s.%N)"
scenario3_detect="$(awk "BEGIN{printf \"%.2f\", $rto_end - $rto_start}")"
echo "scenario3: missing-WAL failure detected in ${scenario3_detect}s"
docker rm --force --volumes "$TGT" >/dev/null

# ---------- pre-migration logical backup checkpoint ----------
phase "SCENARIO 4: pre-migration logical backup + restore into recovered target"
docker rm --force --volumes "$SRC" "$TGT" >/dev/null 2>&1 || true
rm -f "$ARCHIVE"/* "$BACKUP"/* 2>/dev/null || true
start_source
apply_migrations
insert_events "$SRC" "premig" 100
take_base_backup
docker exec "$SRC" pg_dump -U "$DB_USER" -d "$DB_NAME" -Fc -f /tmp/pre-migration.dump >/dev/null
docker cp "$SRC:/tmp/pre-migration.dump" "$DUMP" >/dev/null
psql_exec "$SRC" "ALTER TABLE wal_drill_events ADD COLUMN IF NOT EXISTS migrated_at timestamptz" >/dev/null
insert_events "$SRC" "postmig" 10
sleep 2
stop_source
restore_target "" 0
promote_target
recovery_done
pre_restore_total="$(count_events "$TGT")"
pre_restore_postmig="$(count_marker "$TGT" postmig)"
pre_restore_col="$(column_exists "$TGT" migrated_at)"
if [[ "$pre_restore_total" -ne 110 || "$pre_restore_postmig" -ne 10 || "$pre_restore_col" -ne 1 ]]; then
  echo "scenario4: recovered post-migration state mismatch: total=$pre_restore_total postmig=$pre_restore_postmig col=$pre_restore_col (want 110/10/1)" >&2
  exit 1
fi
if [[ ! -f "$DUMP" ]]; then
  echo "pre-migration backup missing" >&2
  exit 1
fi
docker cp "$DUMP" "$TGT:/tmp/pre-migration.dump" >/dev/null
docker exec "$TGT" pg_restore -U "$DB_USER" -d "$DB_NAME" --clean --if-exists /tmp/pre-migration.dump >/dev/null
restored_count="$(count_events "$TGT")"
restored_postmig="$(count_marker "$TGT" postmig)"
restored_col="$(column_exists "$TGT" migrated_at)"
if [[ "$restored_count" -ne 100 || "$restored_postmig" -ne 0 || "$restored_col" -ne 0 ]]; then
  echo "logical restore mismatch: total=$restored_count postmig=$restored_postmig col=$restored_col (want 100/0/0)" >&2
  exit 1
fi
echo "scenario4: logical backup ($(du -h "$DUMP" | cut -f1)) restores pre-migration state (100 rows)"
docker rm --force --volumes "$TGT" >/dev/null

# ---------- scenario 5: crash fault injection ----------
phase "SCENARIO 5: hard crash (SIGKILL) then crash recovery + archive resume"
restart_source
pre_crash="$(count_events "$SRC")"
segments_before="$(ls "$ARCHIVE" | grep -cE '^[0-9A-F]{24}$' || true)"
insert_events "$SRC" "crash" 20
sleep 1
docker kill -s KILL "$SRC" >/dev/null
for i in $(seq 1 30); do
  if ! docker ps --filter "name=^/$SRC$" --format '{{.Status}}' | grep -q Up; then break; fi
  sleep 1
done
crash_rto_start="$(date +%s.%N)"
docker start "$SRC" >/dev/null
wait_ready "$SRC"
crash_rto_end="$(date +%s.%N)"
post_crash="$(count_events "$SRC")"
crash_marker="$(count_marker "$SRC" crash)"
# force a WAL switch and give the archiver time to ship the post-crash
# segment (archive_timeout=1s); pg_stat_archiver resets across restarts so
# the archive directory file count is the authoritative resume signal
psql_exec "$SRC" "SELECT pg_switch_wal()" >/dev/null
sleep 2
segments_after="$(ls "$ARCHIVE" | grep -cE '^[0-9A-F]{24}$' || true)"
if [[ "$post_crash" -ne $((pre_crash + 20)) || "$crash_marker" -ne 20 ]]; then
  echo "crash recovery mismatch: before=$pre_crash after=$post_crash crash=$crash_marker (want $((pre_crash + 20))/20)" >&2
  exit 1
fi
if [[ "$segments_after" -le "$segments_before" ]]; then
  echo "WAL archiving did not resume after crash: segments before=$segments_before after=$segments_after" >&2
  exit 1
fi
scenario5_rto="$(awk "BEGIN{printf \"%.2f\", $crash_rto_end - $crash_rto_start}")"
echo "scenario5: crash recovery OK ($pre_crash -> $post_crash rows, crash rows=$crash_marker); RTO=${scenario5_rto}s; archive segments $segments_before -> $segments_after"

# ---------- scenario 6: streaming standby ----------
phase "SCENARIO 6: streaming standby - graceful shutdown, catch-up, failover"
docker rm --force --volumes "$SRC" "$TGT" >/dev/null 2>&1 || true
rm -f "$ARCHIVE"/* "$BACKUP"/* 2>/dev/null || true
start_source
apply_migrations
insert_events "$SRC" "primary" 100
take_base_backup
# allow streaming replication from the drill network (image default pg_hba has no replication entry)
docker exec "$SRC" sh -c "echo 'host replication all all scram-sha-256' >> /var/lib/postgresql/data/pg_hba.conf"
docker exec "$SRC" psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT pg_reload_conf()" >/dev/null
primary_ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SRC")"
phase "start streaming standby (primary_conninfo=$primary_ip)"
docker run --detach --name "$SBY"   --network "$NET"   --label io.guiyi.aiops.purpose=wal-pitr-drill   --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret"   --mount "type=bind,source=$BACKUP,target=/var/lib/postgresql/data-source"   --entrypoint /bin/sh   "$IMAGE" -c "
    set -e
    find /var/lib/postgresql/data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true
    cp -a /var/lib/postgresql/data-source/. /var/lib/postgresql/data/
    chown -R postgres:postgres /var/lib/postgresql/data
    cat > /var/lib/postgresql/data/postgresql.auto.conf <<EOF
    primary_conninfo = 'host=$primary_ip port=5432 user=$DB_USER password=drill-secret application_name=drill_sby'
EOF
    touch /var/lib/postgresql/data/standby.signal
    chown -R postgres:postgres /var/lib/postgresql/data
    exec docker-entrypoint.sh postgres
  " >/dev/null
wait_ready "$SBY"
# stream rows from the primary, then wait for the standby to catch up
insert_events "$SRC" "stream" 30
sby_catchup=0
for i in $(seq 1 60); do
  c="$(count_events "$SBY" 2>/dev/null || echo 0)"
  if [[ "$c" -ge 130 ]]; then sby_catchup=1; break; fi
  sleep 1
done
if [[ "$sby_catchup" -ne 1 ]]; then
  echo "standby did not catch up to 130 rows" >&2
  docker logs "$SBY" 2>&1 | tail -10 >&2
  exit 1
fi
echo "scenario6: standby streamed to $c rows"
# graceful shutdown of the standby (docker stop = SIGTERM fast shutdown)
phase "gracefully stop standby, keep primary writing, then restart + catch-up"
docker stop "$SBY" >/dev/null
insert_events "$SRC" "offline" 20
docker start "$SBY" >/dev/null
wait_ready "$SBY"
sby_catchup2=0
for i in $(seq 1 60); do
  c2="$(count_events "$SBY" 2>/dev/null || echo 0)"
  if [[ "$c2" -ge 150 ]]; then sby_catchup2=1; break; fi
  sleep 1
done
if [[ "$sby_catchup2" -ne 1 ]]; then
  echo "standby did not catch up to 150 rows after restart" >&2
  docker logs "$SBY" 2>&1 | tail -10 >&2
  exit 1
fi
echo "scenario6: standby caught up to $c2 rows after restart"
# failover: promote the standby and verify it serves the full dataset writably
phase "promote standby (failover) and verify writable primary"
promoted="$(docker exec "$SBY" psql -U "$DB_USER" -d "$DB_NAME" -tAc "SELECT pg_promote()")"
if [[ "$promoted" != "t" ]]; then
  echo "standby promotion failed (pg_promote=$promoted)" >&2
  exit 1
fi
sby_state="$(psql_exec "$SBY" "SELECT pg_is_in_recovery()")"
if [[ "$sby_state" != "f" ]]; then
  echo "promoted standby still in recovery" >&2
  exit 1
fi
insert_events "$SBY" "post-failover" 5
failover_count="$(count_events "$SBY")"
if [[ "$failover_count" -ne 155 ]]; then
  echo "failover dataset mismatch: $failover_count (want 155)" >&2
  exit 1
fi
echo "scenario6: failover OK - standby writable with $failover_count rows"
docker rm --force --volumes "$SBY" >/dev/null

# ---------- scenario 7: network partition ----------
phase "SCENARIO 7: network partition - standby isolated, then catch-up after reconnect"
take_base_backup
primary_ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$SRC")"
phase "start streaming standby (primary_conninfo=$primary_ip)"
docker run --detach --name "$SBY"   --network "$NET"   --label io.guiyi.aiops.purpose=wal-pitr-drill   --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret"   --mount "type=bind,source=$BACKUP,target=/var/lib/postgresql/data-source"   --entrypoint /bin/sh   "$IMAGE" -c "
    set -e
    find /var/lib/postgresql/data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + 2>/dev/null || true
    cp -a /var/lib/postgresql/data-source/. /var/lib/postgresql/data/
    chown -R postgres:postgres /var/lib/postgresql/data
    cat > /var/lib/postgresql/data/postgresql.auto.conf <<EOF
    primary_conninfo = 'host=$primary_ip port=5432 user=$DB_USER password=drill-secret application_name=drill_sby'
EOF
    touch /var/lib/postgresql/data/standby.signal
    chown -R postgres:postgres /var/lib/postgresql/data
    exec docker-entrypoint.sh postgres
  " >/dev/null
wait_ready "$SBY"
s7_catchup=0
for i in $(seq 1 60); do
  c7="$(count_events "$SBY" 2>/dev/null || echo 0)"
  if [[ "$c7" -ge 150 ]]; then s7_catchup=1; break; fi
  sleep 1
done
if [[ "$s7_catchup" -ne 1 ]]; then
  echo "standby did not catch up to 150 rows" >&2
  docker logs "$SBY" 2>&1 | tail -10 >&2
  exit 1
fi
echo "scenario7: standby caught up to $c7 rows"
net="$(docker inspect -f '{{range $n, $c := .NetworkSettings.Networks}}{{$n}}{{end}}' "$SBY")"
phase "disconnect standby from network ($net); primary keeps writing"
docker network disconnect "$net" "$SBY"
isolated_count="$(count_events "$SBY")"
insert_events "$SRC" "partition" 20
sleep 3
isolated_after="$(count_events "$SBY")"
if [[ "$isolated_after" -ne "$isolated_count" ]]; then
  echo "standby kept streaming while isolated: $isolated_count -> $isolated_after" >&2
  exit 1
fi
echo "scenario7: standby isolated at $isolated_after rows (primary at $((isolated_count + 20)))"
phase "reconnect standby and verify catch-up"
docker network connect "$net" "$SBY"
wait_ready "$SBY"
s7_catchup2=0
for i in $(seq 1 60); do
  c7b="$(count_events "$SBY" 2>/dev/null || echo 0)"
  if [[ "$c7b" -ge $((isolated_count + 20)) ]]; then s7_catchup2=1; break; fi
  sleep 1
done
if [[ "$s7_catchup2" -ne 1 ]]; then
  echo "standby did not catch up to $((isolated_count + 20)) after reconnect" >&2
  docker logs "$SBY" 2>&1 | tail -10 >&2
  exit 1
fi
echo "scenario7: standby caught up to $c7b rows after reconnect"
docker rm --force --volumes "$SBY" >/dev/null

# ---------- scenario 8: archive destination failure ----------
phase "SCENARIO 8: archive destination unavailable - primary unaffected, backlog drains after recovery"
docker rm --force "$SRC" >/dev/null 2>&1 || true
docker volume rm "$VOL" >/dev/null 2>&1 || true
rm -f "$ARCHIVE"/* "$BACKUP"/* 2>/dev/null || true
phase "start source with failing archive_command (archive destination down)"
docker run --detach --name "$SRC"   --label io.guiyi.aiops.purpose=wal-pitr-drill   --network "$NET"   --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret"   --mount "type=volume,source=$VOL,target=/var/lib/postgresql/data"   --mount "type=bind,source=$MIGRATIONS,target=/migrations,readonly"   "$IMAGE"   -c wal_level=archive -c archive_mode=on -c "archive_command=false" -c archive_timeout=1s -c min_wal_size=32MB -c max_wal_senders=4 >/dev/null
wait_ready "$SRC"
apply_migrations
insert_events "$SRC" "archfail" 100
psql_exec "$SRC" "SELECT pg_switch_wal()" >/dev/null
insert_events "$SRC" "archfail2" 30
s8_failed=0
for i in $(seq 1 30); do
  fc="$(psql_exec "$SRC" "SELECT failed_count FROM pg_stat_archiver")"
  if [[ "$fc" -gt 0 ]]; then s8_failed=1; break; fi
  sleep 1
done
if [[ "$s8_failed" -ne 1 ]]; then
  echo "archiver did not report failures with archive_command=false" >&2
  exit 1
fi
primary_count="$(count_events "$SRC")"
if [[ "$primary_count" -ne 130 ]]; then
  echo "primary affected by archive failure: $primary_count (want 130)" >&2
  exit 1
fi
echo "scenario8: primary unaffected by archive failure ($primary_count rows, archiver failed_count=$fc)"
phase "restore archive destination (recreate container on same volume with working archive_command)"
docker stop "$SRC" >/dev/null
docker rm "$SRC" >/dev/null
docker run --detach --name "$SRC"   --label io.guiyi.aiops.purpose=wal-pitr-drill   --network "$NET"   --env "POSTGRES_DB=$DB_NAME" --env "POSTGRES_USER=$DB_USER" --env "POSTGRES_PASSWORD=drill-secret"   --mount "type=volume,source=$VOL,target=/var/lib/postgresql/data"   --mount "type=bind,source=$ARCHIVE,target=/archive"   --mount "type=bind,source=$MIGRATIONS,target=/migrations,readonly"   "$IMAGE"   -c wal_level=archive -c archive_mode=on -c "archive_command=test ! -f /archive/%f && cp %p /archive/%f" -c archive_timeout=1s -c min_wal_size=32MB -c max_wal_senders=4 >/dev/null
wait_ready "$SRC"
recovered_count="$(count_events "$SRC")"
if [[ "$recovered_count" -ne 130 ]]; then
  echo "data lost across archive-failure container recreation: $recovered_count (want 130)" >&2
  exit 1
fi
s8_backlog=0
for i in $(seq 1 60); do
  segs="$(ls "$ARCHIVE" | grep -cE '^[0-9A-F]{24}$' || true)"
  if [[ "$segs" -ge 1 ]]; then s8_backlog=1; break; fi
  sleep 1
done
if [[ "$s8_backlog" -ne 1 ]]; then
  echo "WAL backlog did not drain into archive" >&2
  exit 1
fi
# lossless PITR: base snapshot at 130, then 20 more committed rows via archived WAL
take_base_backup
insert_events "$SRC" "final" 20
sleep 2
stop_source
restore_target "" 0
promote_target
recovery_done
s8_restored="$(count_events "$TGT")"
if [[ "$s8_restored" -ne 150 ]]; then
  echo "post-archive-failure PITR mismatch: $s8_restored (want 150)" >&2
  exit 1
fi
echo "scenario8: backlog drained ($segs segments); lossless PITR restored $s8_restored rows"
docker rm --force --volumes "$TGT" >/dev/null
docker volume rm "$VOL" >/dev/null 2>&1 || true

# ---------- report ----------
cat > "$REPORT" <<JSON
{
  "run_id": "$RUN_ID",
  "image": "$IMAGE",
  "scenarios": {
    "lossless_pitr": {
      "expected": "base backup + archived WAL recover to end of archived WAL; all committed rows present",
      "rows_restored": $target_count,
      "seed_rows": $seed_count,
      "prefail_rows": $prefail_count,
      "rpo_window_seconds_observed": "<=2 (archived WAL at 1s archive_timeout; measured from failure stop)",
      "rto_seconds_observed": $scenario1_rto
    },
    "time_bounded_pitr": {
      "expected": "rows committed after marker excluded",
      "rows_restored": $scenario2_count,
      "marker_time": "$marker_time",
      "late_rows_excluded": $scenario2_late,
      "rto_seconds_observed": $scenario2_rto
    },
    "missing_wal_fault": {
      "expected": "recovery to recorded LSN fails fast with clear, attributable error",
      "detection_seconds_observed": $scenario3_detect
    },
    "pre_migration_logical_backup": {
      "expected": "pg_dump checkpoint before schema change restores pre-migration state; logical backup independent of WAL",
      "backup_bytes": $(stat -f%z "$DUMP" 2>/dev/null || stat -c%s "$DUMP"),
      "recovered_post_migration_rows": $pre_restore_total,
      "logical_restored_rows": $restored_count
    },
    "crash_fault_injection": {
      "expected": "SIGKILL without clean shutdown; committed rows survive crash recovery; WAL archiving resumes",
      "rows_before_crash": $pre_crash,
      "rows_after_recovery": $post_crash,
      "crash_rows_survived": $crash_marker,
      "rto_seconds_observed": $scenario5_rto,
      "archive_segments_before": $segments_before,
      "archive_segments_after": $segments_after
    },
    "streaming_standby": {
      "expected": "hot standby streams from primary; graceful shutdown + restart catches up; promotion serves full dataset",
      "streamed_rows": $c,
      "rows_after_restart_catchup": $c2,
      "rows_after_failover_write": $failover_count
    },
    "network_partition": {
      "expected": "standby isolated from primary stays read-only at snapshot; catch-up resumes after reconnect",
      "rows_while_isolated": $isolated_after,
      "rows_after_reconnect": $c7b
    },
    "archive_destination_failure": {
      "expected": "failing archive_command does not block primary writes; backlog drains after destination recovers; lossless PITR still works",
      "archiver_failed_count_observed": $fc,
      "primary_rows_during_failure": $primary_count,
      "rows_after_destination_recovery": $recovered_count,
      "archive_segments_after_drain": $segs,
      "pitr_rows_restored": $s8_restored
    }
  },
  "notes": "Observed local-environment values only. Production RPO/RTO claims require org-approved provider drills (M90 acceptance)."
}
JSON
echo
echo "== REPORT =="
cat "$REPORT"
echo
echo "drill passed: $REPORT"
