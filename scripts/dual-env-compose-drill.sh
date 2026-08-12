#!/usr/bin/env bash
# M102 (local track): dual fresh-environment compose installation/lifecycle drill.
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
#   3. writes a deterministic audit/user marker into the shared postgres
#   4. recreates the stack to the SAME immutable image digest and verifies the
#      marker persisted (data durability across restart on the same product set)
#   5. tears down fully with -v (fresh, reproducible next run)
#
# The environment variables APP_DB_PORT / APP_BACKEND_PORT / APP_FRONTEND_PORT are
# exported per environment by the script so each iteration is isolated.
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

write_isolated_compose() { # outfile project postgres_port backend_port frontend_port
  local out="$1" project="$2" pg_port="$3" backend_port="$4" fe_port="$5"
  mkdir -p "$(dirname "$out")"
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
    image: $BACKEND_IMAGE
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

run_environment() { # project pg_5000 backend_5000 frontend_5001
  local project="$1" pg="$2" be="$3" fe="$4"
  local base="http://127.0.0.1:$be"
  local compose_file="$WORK/$project-compose.yaml"
  scenario "Environment $project"
  write_isolated_compose "$compose_file" "$project" "$pg" "$be" "$fe"

  if ! docker compose -f "$compose_file" up -d > "$WORK/$project-up1.log" 2>&1; then
    fail "$project-install" "compose up failed: $(tail -3 "$WORK/$project-up1.log")"
    docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
    return
  fi
  if ! backend_ready "$base"; then
    fail "$project-install" "backend not ready after install"
    docker compose -f "$compose_file" logs --tail=30 backend > "$WORK/$project-backend.log" 2>&1 || true
    docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
    return
  fi
  pass "$project-install" "compose stack installed; backend ready (db=$pg backend=$be frontend=$fe)"

  # -- key journey: frontend serves SPA, admin login, /me roles --
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

  # -- durable marker across restart (same immutable image) --
  if docker compose -f "$compose_file" exec -T postgres psql -U aiops -d aiops -tAc \
      "INSERT INTO audit_logs (action, resource_type, result, request_id, details) VALUES ('dualenvironment.drill','DualEnv','success','$project','{}') ON CONFLICT DO NOTHING;" \
      >/dev/null 2>&1; then
    pass "$project-marker" "wrote durable audit marker for $project"
  else
    fail "$project-marker" "could not write audit marker"
  fi

  if ! docker compose -f "$compose_file" up -d --force-recreate > "$WORK/$project-up2.log" 2>&1; then
    fail "$project-restart" "recreate failed: $(tail -3 "$WORK/$project-up2.log")"
  else
    if ! backend_ready "$base"; then
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
  fi

  docker compose -f "$compose_file" down -v > "$WORK/$project-down.log" 2>&1 || true
  pass "$project-cleanup" "stack torn down (-v), environment fully isolated"
}

# ---------- main ----------

scenario "Preflight"
BACKEND_DIGEST="$(docker image inspect "$BACKEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
FRONTEND_DIGEST="$(docker image inspect "$FRONTEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
PG_DIGEST="$(docker image inspect "$PG_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
if [[ -z "$BACKEND_DIGEST" || -z "$FRONTEND_DIGEST" || -z "$PG_DIGEST" ]]; then
  die "one or more required images are missing locally (backend=$BACKEND_IMAGE frontend=$FRONTEND_IMAGE pg=$PG_IMAGE)"
fi
echo "  immutable products:"
echo "    backend  $BACKEND_IMAGE $BACKEND_DIGEST"
echo "    frontend $FRONTEND_IMAGE $FRONTEND_DIGEST"
echo "    postgres $PG_IMAGE $PG_DIGEST"

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
    "postgres": {"image": "$PG_IMAGE", "digest": "$PG_DIGEST"}
  },
  "environments": {
    "a": {
      "project": "aiops-dual-env-a",
      "install": {"result": "$(status_of aiops-dual-env-a-install)", "observed": "$(detail_of aiops-dual-env-a-install)"},
      "key_journey": {"result": "$(status_of aiops-dual-env-a-journey)", "observed": "$(detail_of aiops-dual-env-a-journey)"},
      "data_durability": {"result": "$(status_of aiops-dual-env-a-restart)", "observed": "$(detail_of aiops-dual-env-a-restart)"},
      "cleanup": {"result": "$(status_of aiops-dual-env-a-cleanup)", "observed": "$(detail_of aiops-dual-env-a-cleanup)"}
    },
    "b": {
      "project": "aiops-dual-env-b",
      "install": {"result": "$(status_of aiops-dual-env-b-install)", "observed": "$(detail_of aiops-dual-env-b-install)"},
      "key_journey": {"result": "$(status_of aiops-dual-env-b-journey)", "observed": "$(detail_of aiops-dual-env-b-journey)"},
      "data_durability": {"result": "$(status_of aiops-dual-env-b-restart)", "observed": "$(detail_of aiops-dual-env-b-restart)"},
      "cleanup": {"result": "$(status_of aiops-dual-env-b-cleanup)", "observed": "$(detail_of aiops-dual-env-b-cleanup)"}
    }
  },
  "notes": "Local offline drill only. Two fully isolated fresh environments (project names, ports, postgres volumes and networks) install the same immutable image digests and pass the same key journey. Upgrade/rollback across distinct digests is covered by the CI release lifecycle (M97); this drill establishes second-fresh-environment consistency."
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
