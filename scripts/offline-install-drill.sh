#!/usr/bin/env bash
# M102 (local track): offline install bundle drill.
#
# Proves the full air-gapped distribution chain for the platform: assemble an
# offline bundle (images + compose manifest + config + docs + SHA256SUMS,
# mirroring the M97 release offline asset layout), verify integrity after
# "transfer", load images into a fresh environment, and install from the
# bundle with `pull_policy: never` so a missing local image fails loudly —
# i.e. the install provably needs no network.
#
# The published bundle is also a reusable offline install kit at
# .artifacts/offline-install-drill/bundle/aiops-platform-offline-<version>/.
# The fresh environment is fully isolated (own project name, ports
# 22432/22080/22081, volume and network) and torn down with -v at the end.
# Report: .artifacts/offline-install-drill/report-<run>.json
# Exit 0 when every assertion passes; exit 1 otherwise.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 3)"
ARTIFACTS="$ROOT/.artifacts/offline-install-drill"
REPORT="$ARTIFACTS/report-$RUN_ID.json"
WORK="$ARTIFACTS/tmp-$RUN_ID"
BACKEND_IMAGE="${APP_BACKEND_IMAGE:-k8s-aiops-backend:latest}"
FRONTEND_IMAGE="${APP_FRONTEND_IMAGE:-k8s-aiops-frontend:latest}"
PG_IMAGE="${APP_PG_IMAGE:-pgvector/pgvector:0.8.1-pg17}"
VERSION="${APP_VERSION:-v0.3.0-rc.4-local}"
PROJECT="aiops-offline"
PG_PORT=22432
BE_PORT=22080
FE_PORT=22081
BASE="http://127.0.0.1:$BE_PORT"
JWT_KEY="offline-drill-jwt-signing-key-0123456789abcdef0123456789abcdef"
CRED_KEY="ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE="

mkdir -p "$ARTIFACTS" "$WORK"
: > "$WORK/results.txt"
fail_any=0

scenario() { echo; echo "== $1 =="; }
pass() { echo "$1|pass|$2" >> "$WORK/results.txt"; echo "  PASS: $2"; }
fail() { echo "$1|fail|$2" >> "$WORK/results.txt"; echo "  FAIL: $2"; fail_any=1; }
die() { echo "FATAL: $*" >&2; exit 1; }

backend_ready() {
  local attempt
  for attempt in $(seq 1 60); do
    if curl -s -m 2 "$BASE/api/v1/health/ready" | grep -q '"status":"ready"'; then
      return 0
    fi
    sleep 2
  done
  return 1
}

BUNDLE="$WORK/aiops-platform-offline-$VERSION"
BUNDLE_STABLE="$ARTIFACTS/bundle/aiops-platform-offline-$VERSION"
COMPOSE="$BUNDLE/deploy/compose.offline.yaml"
mkdir -p "$BUNDLE"/{images,deploy,config,docs}

# ---------- 1. bundle assemble ----------

scenario "Assemble offline bundle (aiops-platform-offline-$VERSION)"
BE_DIGEST_BEFORE="$(docker image inspect "$BACKEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
FE_DIGEST_BEFORE="$(docker image inspect "$FRONTEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
PG_DIGEST_BEFORE="$(docker image inspect "$PG_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
if [[ -z "$BE_DIGEST_BEFORE" || -z "$FE_DIGEST_BEFORE" || -z "$PG_DIGEST_BEFORE" ]]; then
  fail bundle-assemble "missing local image (backend=${BE_DIGEST_BEFORE:-none} frontend=${FE_DIGEST_BEFORE:-none} pg=${PG_DIGEST_BEFORE:-none})"
else
  if docker save "$BACKEND_IMAGE" -o "$BUNDLE/images/k8s-aiops-backend.tar" >/dev/null 2>&1 \
    && docker save "$FRONTEND_IMAGE" -o "$BUNDLE/images/k8s-aiops-frontend.tar" >/dev/null 2>&1 \
    && docker save "$PG_IMAGE" -o "$BUNDLE/images/pgvector-pg17.tar" >/dev/null 2>&1; then
    pass bundle-assemble "images saved (backend=${BE_DIGEST_BEFORE:0:12} frontend=${FE_DIGEST_BEFORE:0:12} pg=${PG_DIGEST_BEFORE:0:12})"
  else
    fail bundle-assemble "docker save failed"
  fi
fi

cat > "$COMPOSE" <<YAML
name: $PROJECT
services:
  postgres:
    image: $PG_IMAGE
    pull_policy: never
    environment:
      POSTGRES_DB: aiops
      POSTGRES_USER: aiops
      POSTGRES_PASSWORD: change_me
    ports:
      - "$PG_PORT:5432"
    volumes:
      - "$PROJECT-pgdata:/var/lib/postgresql/data"
      - "$ROOT/backend/migrations/000001_init_schema.up.sql:/docker-entrypoint-initdb.d/000001_init_schema.sql:ro"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U aiops -d aiops"]
      interval: 5s
      timeout: 3s
      retries: 10
    restart: unless-stopped
  backend:
    image: $BACKEND_IMAGE
    pull_policy: never
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
      - "$BE_PORT:8080"
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
    pull_policy: never
    ports:
      - "$FE_PORT:8080"
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
  $PROJECT-pgdata:
YAML
cat > "$BUNDLE/config/env.example" <<'ENV'
# Offline install defaults (drill bundle). Replace every change_me value
# before production use.
APP_ENV=development
POSTGRES_DB=aiops
POSTGRES_USER=aiops
POSTGRES_PASSWORD=change_me
JWT_SIGNING_KEY=change_me_jwt_signing_key
BOOTSTRAP_ADMIN_USERNAME=admin
BOOTSTRAP_ADMIN_PASSWORD=change_me_now
CREDENTIAL_ENCRYPTION_KEY=change_me_32_byte_base64_key
CREDENTIAL_KEY_VERSION=v1
AI_ENABLED=false
NOTIFICATION_ENABLED=false
ENV
cp "$ROOT/docs/release-candidate-operations.md" "$BUNDLE/docs/release-candidate-operations.md"

if (cd "$BUNDLE" && find . -type f ! -name OFFLINE-SHA256SUMS -print0 | sort -z | xargs -0 shasum -a 256 > OFFLINE-SHA256SUMS) 2>"$WORK/sha.log"; then
  pass bundle-manifest "offline bundle assembled with OFFLINE-SHA256SUMS ($(wc -l < "$BUNDLE/OFFLINE-SHA256SUMS" | tr -d ' ') files)"
else
  fail bundle-manifest "SHA256SUMS generation failed: $(tail -3 "$WORK/sha.log")"
fi

# ---------- 2. integrity verification after transfer ----------

scenario "Verify bundle integrity (simulated air-gapped transfer)"
if (cd "$BUNDLE" && shasum -a 256 -c OFFLINE-SHA256SUMS) > "$WORK/verify.log" 2>&1; then
  pass sha256-verify "all $(grep -c ': OK' "$WORK/verify.log" || true) files OK"
else
  fail sha256-verify "$(tail -3 "$WORK/verify.log")"
fi

# ---------- 2b. publish reusable bundle ----------

scenario "Publish reusable offline install kit"
python3 - "$BUNDLE" "$BUNDLE_STABLE" <<'PYEOF'
import os, shutil, sys
src, dst = sys.argv[1], sys.argv[2]
tmp = dst + ".tmp"
if os.path.exists(tmp):
    shutil.rmtree(tmp)
shutil.copytree(src, tmp)
if os.path.exists(dst):
    shutil.rmtree(dst)
os.replace(tmp, dst)
print("published " + dst)
PYEOF
if [[ -f "$BUNDLE_STABLE/OFFLINE-SHA256SUMS" ]] && (cd "$BUNDLE_STABLE" && shasum -a 256 -c OFFLINE-SHA256SUMS >/dev/null 2>&1); then
  pass bundle-publish "reusable offline kit: $BUNDLE_STABLE"
else
  fail bundle-publish "bundle publish failed"
fi

# ---------- 3. load images from bundle ----------

scenario "Load images from bundle (docker load)"
LOAD_OK=1
for tar in "$BUNDLE"/images/*.tar; do
  out="$(docker load -i "$tar" 2>&1 || true)"
  if ! grep -q "Loaded image" <<<"$out"; then
    LOAD_OK=0
    echo "  load failed: $tar -> $out"
  fi
done
if [[ "$LOAD_OK" == "1" ]]; then
  BE_DIGEST_AFTER="$(docker image inspect "$BACKEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
  FE_DIGEST_AFTER="$(docker image inspect "$FRONTEND_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
  PG_DIGEST_AFTER="$(docker image inspect "$PG_IMAGE" --format '{{.Id}}' 2>/dev/null || true)"
  if [[ "$BE_DIGEST_AFTER" == "$BE_DIGEST_BEFORE" && "$FE_DIGEST_AFTER" == "$FE_DIGEST_BEFORE" && "$PG_DIGEST_AFTER" == "$PG_DIGEST_BEFORE" ]]; then
    pass image-load "all images loaded; digests unchanged (backend=${BE_DIGEST_AFTER:0:12} ...)"
  else
    fail image-load "digest mismatch after load"
  fi
else
  fail image-load "one or more images failed to load"
fi

# ---------- 4. install from bundle (pull_policy: never) ----------

scenario "Install from bundle (no network required)"
if ! docker compose -f "$COMPOSE" up -d > "$WORK/up.log" 2>&1; then
  fail install "compose up failed: $(tail -3 "$WORK/up.log")"
else
  if backend_ready; then
    pass install "fresh offline install ready (pg=$PG_PORT backend=$BE_PORT frontend=$FE_PORT, pull_policy=never)"
  else
    docker compose -f "$COMPOSE" logs --tail=30 backend > "$WORK/backend.log" 2>&1 || true
    fail install "backend not ready after offline install"
  fi
fi

# ---------- 5. key journey ----------

scenario "Key journey on offline install"
FE_CODE=""
for attempt in $(seq 1 30); do
  FE_CODE="$(curl -s -m 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$FE_PORT/" 2>/dev/null || true)"
  [[ "$FE_CODE" == "200" ]] && break
  sleep 2
done
LOGIN="$(curl -s -m 5 -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"change_me_now"}' 2>/dev/null || true)"
TOKEN="$(jq -r '.access_token // empty' <<<"$LOGIN" 2>/dev/null || true)"
ME="$(curl -s -m 5 -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/auth/me" 2>/dev/null || true)"
UNAME="$(jq -r '.username // empty' <<<"$ME")"
ROLE="$(jq -r '.roles[0] // empty' <<<"$ME")"
if [[ "$FE_CODE" == "200" ]] && [[ -n "$TOKEN" ]] && [[ "$UNAME" == "admin" ]] && [[ "$ROLE" == "system_admin" ]]; then
  pass key-journey "frontend 200, admin login, /me -> system_admin"
else
  fail key-journey "frontend=$FE_CODE token=${#TOKEN} uname=$UNAME role=$ROLE"
fi

# ---------- 6. durability across restart ----------

scenario "Data durability across restart on offline install"
if docker compose -f "$COMPOSE" exec -T postgres psql -U aiops -d aiops -tAc \
    "INSERT INTO audit_logs (action, resource_type, result, request_id, details) VALUES ('offline.drill','Offline','success','$RUN_ID','{}') ON CONFLICT DO NOTHING;" \
    >/dev/null 2>&1; then
  pass durability-write "wrote durable marker"
else
  fail durability-write "could not write marker"
fi
if docker compose -f "$COMPOSE" up -d --force-recreate backend > "$WORK/recreate.log" 2>&1 && backend_ready; then
  COUNT="$(docker compose -f "$COMPOSE" exec -T postgres psql -U aiops -d aiops -tAc \
    "SELECT count(*) FROM audit_logs WHERE action='offline.drill' AND request_id='$RUN_ID'" | tr -d '[:space:]' || true)"
  if [[ "$COUNT" == "1" ]]; then
    pass durability-restart "marker persisted across backend recreate (count=$COUNT)"
  else
    fail durability-restart "marker missing after recreate (count=$COUNT)"
  fi
else
  fail durability-restart "backend recreate failed: $(tail -3 "$WORK/recreate.log")"
fi

# ---------- 7. cleanup ----------

scenario "Cleanup"
docker compose -f "$COMPOSE" down -v >/dev/null 2>&1 || true
REMAINING="$(docker compose -f "$COMPOSE" ps -a --format '{{.Name}}' 2>/dev/null || true)"
if [[ -z "$REMAINING" ]]; then
  pass cleanup "offline install stack fully removed (down -v)"
else
  fail cleanup "leftovers: $REMAINING"
fi

# ---------- report ----------

PASSED="$(grep -c '|pass|' "$WORK/results.txt" || true)"
FAILED="$(grep -c '|fail|' "$WORK/results.txt" || true)"
{
  echo "{"
  echo "  \"schema\": \"aiops.offline-install-drill/v1\","
  echo "  \"run_id\": \"$RUN_ID\","
  echo "  \"bundle\": \"aiops-platform-offline-$VERSION\","
  echo "  \"bundle_stable\": \"$BUNDLE_STABLE\","
  echo "  \"images\": {"
  echo "    \"backend\": \"$BACKEND_IMAGE\","
  echo "    \"frontend\": \"$FRONTEND_IMAGE\","
  echo "    \"postgres\": \"$PG_IMAGE\""
  echo "  },"
  echo "  \"digests\": {"
  echo "    \"backend\": \"$BE_DIGEST_BEFORE\","
  echo "    \"frontend\": \"$FE_DIGEST_BEFORE\","
  echo "    \"postgres\": \"$PG_DIGEST_BEFORE\""
  echo "  },"
  echo "  \"steps\": ["
  FIRST=1
  while IFS='|' read -r id status detail; do
    if [[ "$FIRST" == "1" ]]; then FIRST=0; else echo ","; fi
    echo -n "    {\"id\": \"$id\", \"status\": \"$status\", \"detail\": $(jq -Rs . <<<"$detail")}"
  done < "$WORK/results.txt"
  echo
  echo "  ],"
  echo "  \"summary\": { \"passed\": $PASSED, \"failed\": $FAILED }"
  echo "}"
} > "$REPORT"

echo
echo "RESULT offline-install-drill: PASS=$PASSED FAIL=$FAILED"
echo "Report: $REPORT"
echo "Offline kit: $BUNDLE_STABLE"
if [[ "$fail_any" == "1" || "$FAILED" != "0" ]]; then
  exit 1
fi
exit 0
