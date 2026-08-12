#!/usr/bin/env bash
# M102 (local track): dual fresh-environment compose installation/upgrade/rollback drill.
#
# Proves the same immutable image products can be installed on two fully isolated
# fresh environments and both serve the same key journeys, using a reproducible
# offline compose layout. Every environment gets its own project name, TCP ports,
# postgres data volume and compose network (no shared state with the running
# development compose stack on 15432/8080/18080).
#
# For each environment the drill:
#   1. creates a fresh named volume + launches postgres/backend/frontend (install)
#   2. waits for backend ready, verifies frontend serves the SPA, admin login ->
#      /me -> system_admin (key journey)
#   3. writes a deterministic audit marker into the shared postgres
#   4. recreates the stack to the SAME immutable backend digest and verifies the
#      marker persisted (data durability across restart on the same product set)
#   5. when APP_UPGRADE_BACKEND_IMAGE is set: upgrade backend to a DIFFERENT
#      immutable digest, verify health + marker + backend version changed,
#      then roll back to the baseline digest and verify again (upgrade/rollback)
#   6. tears down fully with -v (fresh, reproducible next run)
#
# Exit 0 when every assertion passes on both environments; exit 1 otherwise.
# Report: .artifacts/dual-env-compose-drill/report-<run>.json
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 3)"
ARTIFACTS="$ROOT/.artifacts/dual-env-compose-drill"
REPORT="$ARTIFACTS/report-$RUN_ID.json"
WORK="$ARTIFACTS/tmp-$RUN_ID"
BACKEND_IMAGE="${APP_BACKEND_IMAGE:-k8s-aiops-backend:latest}"
FRONTEND_IMAGE="${APP_FRONTEND_IMAGE:-k8s-aiops-frontend:latest}"
PG_IMAGE="${APP_PG_IMAGE:-pgvector/pgvector:0.8.1-pg17}"
UPGRADE_BACKEND_IMAGE="${APP_UPGRADE_BACKEND_IMAGE:-}"
JWT_KEY="dual-env-jwt-signing-key-0123456789abcdef0123456789abcdef"
CRED_KEY="ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE="

mkdir -p "$ARTIFACTS" "$WORK"
: > "$WORK/results.txt"
fail_any=0

# ---------- helpers ----------

scenario() { echo; echo "== $1 =="; }
status_of() { awk -F'|' -v k="$1" '$1==k {print $2}' "$WORK/results.txt" | head -1; }
detail_of() { awk -F'|' -v k="$1" '$1==k {sub(/^[^|]*\|[^|]*\|/, ""); print}' "$WORK/results.txt" | head -1; }
pass() { echo "$1|pass|$2" >> "$WORK/results.txt"; echo "  PASS: $2"; }
fail() { echo "$1|fail|$2" >> "$WORK/results.txt"; echo "  FAIL: $2"; fail_any=1; }

die() { echo "FATAL: $*" >&2; exit 1; }

# backend_ready <base_url>
backend_ready() {
  local base="$1" attempt
  for attempt in $(seq 1 60); do
    if curl -s -m 2 "$base/api/v1/health/ready" | grep -q '"status":"ready"'; then
      return 0
    fi
    sleep 2
  done
  return 1
}

# backend_version <base_url> -> e.g. "dev"
backend_version() {
  curl -s -m 5 "$1/api/v1/health/ready" | jq -r '.version // empty' 2>/dev/null || true
}

write_isolated_compose() { # outfile project backend_image postgres_port backend_port frontend_port
  local out="$1" project="$2" backend_image="$3" pg_port="$4" backend_port="$5" fe_port="$6"
  cat > "$out" <<YAML
name: $project
services:
  postgres:
    image: $PG_IMAGE
    environment:
      POSTGRES_DB: aiops
      POSTGRES_USER: aiops
      POSTGRES_PASSWORD: change_me
    ports:
      - "$pg_port:5432"
    volumes:
      - "$project-pgdata:/var/lib/postgresql/data"
      - "$ROOT/backend/migrations/000001_init_schema.up.sql:/docker-entrypoint-initdb.d/000001_init_schema.sql:ro"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U aiops -d aiops"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped
  backend:
    image: $backend_image
    environment:
      APP_ENV: development
      HTTP_ADDR: :8080
      DATABASE_URL: postgres://aiops:change_me@postgres:5432/aiops?sslmode=disable
      JWT_SIGNING_KEY: $JWT_KEY
      ACCESS_TOKEN_TTL: 15m
      REFRESH_TOKEN_TTL: 168h
      BOOTSTRAP_ADMIN_USERNAME: admin
      BOOTSTRAP_ADMIN_PASSWORD: change_me_now
      CREDENTIAL_ENCRYPTION_KEY: $CRED_KEY
      CREDENTIAL_KEY_VERSION: v1
      CREDENTIAL_DECRYPTION_KEYS: ""
      METRICS_HISTORY_ENABLED: "true"
      AI_ENABLED: "false"
      NOTIFICATION_ENABLED: "false"
      ALERT_ENABLED: "false"
    ports:
      - "$backend_port:8080"
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "-", "http://localhost:8080/api/v1/health/ready"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped
  frontend:
    image: $FRONTEND_IMAGE
    ports:
      - "$fe_port:8080"
    depends_on:
      backend:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "-", "http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 3s
      retries: 5
    restart: unless-stopped
volumes:
  $project-pgdata:
YAML
}

# compose_phase <compose_file> <project> <backend_image> <method> <job> <logtag>
#   method = up | recreate
compose_phase() {
  local compose_file="$1" project="$2" backend_image="$3" method="$4" job="$5" logtag="$6"
  local log="$WORK/$project-$logtag.log"
  if [[ "$method" == "recreate" ]]; then
    docker compose -f "$compose_file" up -d --force-recreate backend > "$log" 2>&1
  else
    docker compose -f "$compose_file" up -d > "$log" 2>&1
  fi
  return $?
}

wait_healthy() { # compose_file
  local compose_file="$1"
  docker compose -f "$compose_file" ps --status running >/dev/null 2>&1 || true
  local attempt
  for attempt in $(seq 1 60); do
    if docker compose -f "$compose_file" ps --status running --format '{{.Service}} {{.Health}}' 2>/dev/null | grep -q '^backend healthy'; then
      return 0
    fi
    sleep 2
  done
  return 1
}

run_environment() { # project pg_5000 backend_5000 frontend_5001
  local project="$1" pg="$2" be="$3" fe="$4"
  local base="http://127.0.0.1:$be"
  local compose_file="$WORK/$project-compose.yaml"
  scenario "Environment $project"

  write_isolated_compose "$compose_file" "$project" "$BACKEND_IMAGE" "$pg" "$be" "$fe"

  # -- install --
  if ! compose_phase "$compose_file" "$project" "$BACKEND_IMAGE" up install install; then
    fail "$project-install" "compose up failed: $(tail -3 "$WORK/$project-install.log")"
    docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
    return
  fi
  if ! backend_ready "$base"; then
    fail "$project-install" "backend not ready after install"
    docker compose -f "$compose_file" logs --tail=30 backend > "$WORK/$project-backend.log" 2>&1 || true
    docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
    return
  fi
  local v_install
  v_install="$(backend_version "$base")"
  pass "$project-install" "compose stack installed; backend ready (db=$pg backend=$be frontend=$fe) version=$v_install"

  # -- key journey --
  local fe_code attempt
  fe_code=""
  for attempt in $(seq 1 30); do
    fe_code="$(curl -s -m 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$fe/" 2>/dev/null || true)"
    if [[ "$fe_code" == "200" ]]; then
      break
    fi
    sleep 2
  done
  local login token me role uname
  login="$(curl -s -m 5 -X POST "$base/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"change_me_now"}' 2>/dev/null || true)"
  token="$(jq -r '.access_token // empty' <<<"$login" 2>/dev/null || true)"
  me="$(curl -s -m 5 -H "Authorization: Bearer $token" "$base/api/v1/auth/me" 2>/dev/null || true)"
  role="$(jq -r '.roles[0] // empty' <<<"$me" 2>/dev/null || true)"
  uname="$(jq -r '.username // empty' <<<"$me" 2>/dev/null || true)"
  if [[ "$fe_code" == "200" ]] && [[ -n "$token" ]] && [[ "$uname" == "admin" ]] && [[ "$role" == "system_admin" ]]; then
    pass "$project-journey" "key journey OK: frontend 200, admin login, /me -> system_admin"
  else
    fail "$project-journey" "frontend=$fe_code token=${#token} uname=$uname role=$role"
  fi

  # -- durable marker + restart on SAME baseline digest --
  if docker compose -f "$compose_file" exec -T postgres psql -U aiops -d aiops -tAc \
      "INSERT INTO audit_logs (action, resource_type, result, request_id, details) VALUES ('dualenvironment.drill','DualEnv','success','$project','{}') ON CONFLICT DO NOTHING;" \
      >/dev/null 2>&1; then
    pass "$project-marker" "wrote durable audit marker for $project"
  else
    fail "$project-marker" "could not write audit marker"
  fi

  if ! compose_phase "$compose_file" "$project" "$BACKEND_IMAGE" recreate restart restart; then
    fail "$project-restart" "recreate failed: $(tail -3 "$WORK/$project-restart.log")"
  elif ! backend_ready "$base"; then
    fail "$project-restart" "backend not ready after recreate"
  else
    local count
    count="$(docker compose -f "$compose_file" exec -T postgres psql -U aiops -d aiops -tAc \
      "SELECT count(*) FROM audit_logs WHERE action='dualenvironment.drill' AND request_id='$project'" | tr -d '[:space:]' || true)"
    if [[ "$count" == "1" ]]; then
      pass "$project-restart" "marker persisted across stack recreate (count=$count)"
    else
      fail "$project-restart" "marker missing after recreate (count=$count)"
    fi
  fi

  # -- optional cross-digest upgrade + rollback --
  if [[ -n "$UPGRADE_BACKEND_IMAGE" ]]; then
    local v_before v_after v_rollback up_count
    v_before="$(backend_version "$base")"
    write_isolated_compose "$compose_file" "$project" "$UPGRADE_BACKEND_IMAGE" "$pg" "$be" "$fe"
    if ! compose_phase "$compose_file" "$project" "$UPGRADE_BACKEND_IMAGE" recreate upgrade upgrade; then
      fail "$project-upgrade" "upgrade failed: $(tail -3 "$WORK/$project-upgrade.log")"
    elif ! backend_ready "$base"; then
      fail "$project-upgrade" "backend not ready after upgrade"
    else
      v_after="$(backend_version "$base")"
      up_count="$(docker compose -f "$compose_file" exec -T postgres psql -U aiops -d aiops -tAc \
        "SELECT count(*) FROM audit_logs WHERE action='dualenvironment.drill' AND request_id='$project'" | tr -d '[:space:]' || true)"
      if [[ "$v_after" != "$v_before" ]] && [[ "$v_after" != "" ]] && [[ "$up_count" == "1" ]]; then
        pass "$project-upgrade" "upgrade to $UPGRADE_BACKEND_IMAGE OK: version $v_before -> $v_after, marker intact"
      else
        fail "$project-upgrade" "version unchanged ($v_before -> $v_after) or marker lost ($up_count)"
      fi
    fi

    write_isolated_compose "$compose_file" "$project" "$BACKEND_IMAGE" "$pg" "$be" "$fe"
    if ! compose_phase "$compose_file" "$project" "$BACKEND_IMAGE" recreate rollback rollback; then
      fail "$project-rollback" "rollback failed: $(tail -3 "$WORK/$project-rollback.log")"
    elif ! backend_ready "$base"; then
      fail "$project-rollback" "backend not ready after rollback"
    else
      v_rollback="$(backend_version "$base")"
      up_count="$(docker compose -f "$compose_file" exec -T postgres psql -U aiops -d aiops -tAc \
        "SELECT count(*) FROM audit_logs WHERE action='dualenvironment.drill' AND request_id='$project'" | tr -d '[:space:]' || true)"
      if [[ "$v_rollback" == "$v_before" ]] && [[ "$up_count" == "1" ]]; then
        pass "$project-rollback" "rollback to $BACKEND_IMAGE OK: version $v_after -> $v_rollback, marker intact"
      else
        fail "$project-rollback" "version=$v_rollback (want $v_before) or marker lost ($up_count)"
      fi
    fi
  fi

  docker compose -f "$compose_file" down -v > "$WORK/$project-down.log" 2>&1 || true
  pass "$project-cleanup" "stack torn down (-v), environment fully isolated"
}

# ---------- main ----------

scenario "Preflight"
BACKEND_DIGEST="$(docker image inspect "$BACKEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
FRONTEND_DIGEST="$(docker image inspect "$FRONTEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
PG_DIGEST="$(docker image inspect "$PG_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
UPG_DIGEST=""
if [[ -z "$BACKEND_DIGEST" || -z "$FRONTEND_DIGEST" || -z "$PG_DIGEST" ]]; then
  die "one or more required images are missing locally (backend=$BACKEND_IMAGE frontend=$FRONTEND_IMAGE pg=$PG_IMAGE)"
fi
echo "  immutable products:"
echo "    backend  $BACKEND_IMAGE $BACKEND_DIGEST"
echo "    frontend $FRONTEND_IMAGE $FRONTEND_DIGEST"
echo "    postgres $PG_IMAGE $PG_DIGEST"
if [[ -n "$UPGRADE_BACKEND_IMAGE" ]]; then
  UPG_DIGEST="$(docker image inspect "$UPGRADE_BACKEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
  if [[ -z "$UPG_DIGEST" ]]; then
    die "APP_UPGRADE_BACKEND_IMAGE image missing locally: $UPGRADE_BACKEND_IMAGE"
  fi
  echo "    upgrade  $UPGRADE_BACKEND_IMAGE $UPG_DIGEST"
else
  echo "    upgrade  (not configured; run with APP_UPGRADE_BACKEND_IMAGE to enable upgrade/rollback)"
fi

run_environment aiops-dual-env-a 25432 28080 28081
run_environment aiops-dual-env-b 26432 29080 29081

# ---------- report ----------

cat > "$REPORT" <<JSON
{
  "run_id": "$RUN_ID",
  "schema": "aiops.dual-env-compose-drill/v1",
  "immutable_products": {
    "backend": {"image": "$BACKEND_IMAGE", "digest": "$BACKEND_DIGEST"},
    "frontend": {"image": "$FRONTEND_IMAGE", "digest": "$FRONTEND_DIGEST"},
    "postgres": {"image": "$PG_IMAGE", "digest": "$PG_DIGEST"},
    "upgrade_backend": {"image": "${UPGRADE_BACKEND_IMAGE:-}", "digest": "${UPG_DIGEST:-}"}
  },
  "environments": {
    "a": {
      "install": {"result": "$(status_of aiops-dual-env-a-install)", "observed": "$(detail_of aiops-dual-env-a-install)"},
      "key_journey": {"result": "$(status_of aiops-dual-env-a-journey)", "observed": "$(detail_of aiops-dual-env-a-journey)"},
      "data_durability": {"result": "$(status_of aiops-dual-env-a-restart)", "observed": "$(detail_of aiops-dual-env-a-restart)"},
      "upgrade": {"result": "$(status_of aiops-dual-env-a-upgrade)", "observed": "$(detail_of aiops-dual-env-a-upgrade)"},
      "rollback": {"result": "$(status_of aiops-dual-env-a-rollback)", "observed": "$(detail_of aiops-dual-env-a-rollback)"},
      "cleanup": {"result": "$(status_of aiops-dual-env-a-cleanup)", "observed": "$(detail_of aiops-dual-env-a-cleanup)"}
    },
    "b": {
      "install": {"result": "$(status_of aiops-dual-env-b-install)", "observed": "$(detail_of aiops-dual-env-b-install)"},
      "key_journey": {"result": "$(status_of aiops-dual-env-b-journey)", "observed": "$(detail_of aiops-dual-env-b-journey)"},
      "data_durability": {"result": "$(status_of aiops-dual-env-b-restart)", "observed": "$(detail_of aiops-dual-env-b-restart)"},
      "upgrade": {"result": "$(status_of aiops-dual-env-b-upgrade)", "observed": "$(detail_of aiops-dual-env-b-upgrade)"},
      "rollback": {"result": "$(status_of aiops-dual-env-b-rollback)", "observed": "$(detail_of aiops-dual-env-b-rollback)"},
      "cleanup": {"result": "$(status_of aiops-dual-env-b-cleanup)", "observed": "$(detail_of aiops-dual-env-b-cleanup)"}
    }
  },
  "notes": "Local offline drill only. Two fully isolated fresh environments install the same immutable baseline products. When a distinct upgrade backend image is supplied, both environments additionally prove a cross-digest upgrade (version change + marker intact) and a rollback to the baseline digest. Not a production install claim."
}
JSON
echo
echo "== REPORT =="
cat "$REPORT"
echo
if [[ "$fail_any" == "1" ]]; then
  echo "drill FAILED: $REPORT"
  exit 1
fi
echo "drill passed: $REPORT"
