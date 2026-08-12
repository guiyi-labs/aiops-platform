#!/usr/bin/env bash
# M102 (local track): reproducible demo drill — 登录 → 态势 → 根因 → 证据 →
# 受控动作 → 验证 → 事故复盘, against a fully isolated offline compose
# environment with a bundled mock Kubernetes API server.
#
# The platform's real API drives the whole journey; the mock k8s API
# (backend/cmd/demo-kube-mock) supplies deterministic cluster objects
# (a Node stuck NotReady and a Pod terminated by OOMKilled) so every run
# reproduces the same evidence. Controlled actions land as real PATCH
# mutations on the mock and are verified through the platform API.
#
# Ports: postgres 21432 / backend 21080 / frontend 21081 — fully isolated from
# the development stack (15432/8080/18080) and the dual-env drill (25432+).
# Report: .artifacts/demo-drill/report-<run>.json
# Exit 0 when every assertion passes; exit 1 otherwise.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 3)"
ARTIFACTS="$ROOT/.artifacts/demo-drill"
REPORT="$ARTIFACTS/report-$RUN_ID.json"
WORK="$ARTIFACTS/tmp-$RUN_ID"
BACKEND_IMAGE="${APP_BACKEND_IMAGE:-k8s-aiops-backend:latest}"
FRONTEND_IMAGE="${APP_FRONTEND_IMAGE:-k8s-aiops-frontend:latest}"
PG_IMAGE="${APP_PG_IMAGE:-pgvector/pgvector:0.8.1-pg17}"
MOCK_IMAGE="${APP_MOCK_IMAGE:-k8s-aiops-demo-mock:latest}"
REBUILD_MOCK="${APP_REBUILD_MOCK:-}"
JWT_KEY="demo-drill-jwt-signing-key-0123456789abcdef0123456789abcdef"
CRED_KEY="ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE="
PROJECT="aiops-demo"
PG_PORT=21432
BE_PORT=21080
FE_PORT=21081
BASE="http://127.0.0.1:$BE_PORT"

mkdir -p "$ARTIFACTS" "$WORK"
: > "$WORK/results.txt"
fail_any=0

# ---------- helpers ----------

scenario() { echo; echo "== $1 =="; }
pass() { echo "$1|pass|$2" >> "$WORK/results.txt"; echo "  PASS: $2"; }
fail() { echo "$1|fail|$2" >> "$WORK/results.txt"; echo "  FAIL: $2"; fail_any=1; }
die() { echo "FATAL: $*" >&2; exit 1; }

api() { # api <method> <path> [json-body] [extra-curl-args...]
  local method="$1" path="$2" body="${3:-}" extra="${4:-}"
  if [[ -n "$body" ]]; then
    curl -s -m 15 -X "$method" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
      -d "$body" $extra "$BASE$path"
  else
    curl -s -m 15 -X "$method" -H "Authorization: Bearer $TOKEN" $extra "$BASE$path"
  fi
}

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

cluster_ready() { # cluster_id
  local id="$1" attempt status
  for attempt in $(seq 1 30); do
    status="$(api GET "/api/v1/clusters/$id" | jq -r '.status // empty' 2>/dev/null || true)"
    if [[ "$status" == "ready" ]]; then
      return 0
    fi
    sleep 2
  done
  return 1
}

compose_file="$WORK/$PROJECT-compose.yaml"

write_isolated_compose() {
  cat > "$compose_file" <<YAML
name: $PROJECT
services:
  postgres:
    image: $PG_IMAGE
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
  demo-mock:
    image: $MOCK_IMAGE
    command: ["-listen", "0.0.0.0:8443"]
    ports:
      - "21443:8443"
    healthcheck:
      test: ["CMD", "/demo-kube-mock", "-healthcheck"]
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
}

ensure_mock_image() {
  if [[ "$REBUILD_MOCK" == "1" ]] || ! docker image inspect "$MOCK_IMAGE" >/dev/null 2>&1; then
    scenario "Build demo mock image ($MOCK_IMAGE)"
    local arch goarch
    arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || echo arm64)"
    case "$arch" in
      x86_64|amd64) goarch="amd64" ;;
      *) goarch="arm64" ;;
    esac
    (cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -o "$WORK/mock/demo-kube-mock" ./cmd/demo-kube-mock) \
      || die "mock build failed"
    cat > "$WORK/mock/Dockerfile" <<'DOCKER'
FROM scratch
COPY demo-kube-mock /demo-kube-mock
ENTRYPOINT ["/demo-kube-mock"]
DOCKER
    docker build -q -t "$MOCK_IMAGE" "$WORK/mock" >/dev/null || die "mock image build failed"
  fi
}

cleanup_stack() {
  docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
}

trap cleanup_stack EXIT

# ---------- 1. install ----------

ensure_mock_image
write_isolated_compose

scenario "Install isolated demo environment"
if ! docker compose -f "$compose_file" up -d > "$WORK/up.log" 2>&1; then
  die "compose up failed: $(tail -3 "$WORK/up.log")"
fi
if ! backend_ready; then
  docker compose -f "$compose_file" logs --tail=30 backend > "$WORK/backend.log" 2>&1 || true
  die "backend not ready after install"
fi
pass install "isolated demo stack installed (pg=$PG_PORT backend=$BE_PORT frontend=$FE_PORT)"

# ---------- 2. login ----------

scenario "Login (管理员登录)"
FE_CODE=""
for attempt in $(seq 1 30); do
  FE_CODE="$(curl -s -m 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$FE_PORT/" 2>/dev/null || true)"
  [[ "$FE_CODE" == "200" ]] && break
  sleep 2
done
LOGIN="$(curl -s -m 5 -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change_me_now"}' 2>/dev/null || true)"
TOKEN="$(jq -r '.access_token // empty' <<<"$LOGIN" 2>/dev/null || true)"
ME="$(api GET /api/v1/auth/me)"
UNAME="$(jq -r '.username // empty' <<<"$ME")"
ROLE="$(jq -r '.roles[0] // empty' <<<"$ME")"
if [[ "$FE_CODE" == "200" ]] && [[ -n "$TOKEN" ]] && [[ "$UNAME" == "admin" ]] && [[ "$ROLE" == "system_admin" ]]; then
  pass login "frontend 200, admin login, /me -> system_admin"
else
  fail login "frontend=$FE_CODE token=${#TOKEN} uname=$UNAME role=$ROLE"
fi

# ---------- 3. cluster registration (态势基础) ----------

scenario "Register demo cluster against offline mock k8s API"
KC_DOC="$(cat <<'KC'
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://demo-mock:8443
    insecure-skip-tls-verify: true
  name: demo
contexts:
- context:
    cluster: demo
    user: demo-user
  name: demo
current-context: demo
users:
- name: demo-user
  user:
    token: demo-mock-token
KC
)"
CREATED="$(api POST /api/v1/clusters "{\"name\":\"demo\",\"kubeconfig\":$(jq -Rs . <<<"$KC_DOC")}")"
CLUSTER_ID="$(jq -r '.id // 0' <<<"$CREATED")"
if [[ -z "$CLUSTER_ID" || "$CLUSTER_ID" == "0" ]]; then
  fail cluster-register "cluster create failed: $CREATED"
else
  api PATCH "/api/v1/clusters/$CLUSTER_ID" '{"enabled":true}' >/dev/null || true
  api POST "/api/v1/clusters/$CLUSTER_ID/probe" >/dev/null || true
  if cluster_ready "$CLUSTER_ID"; then
    pass cluster-register "cluster demo registered id=$CLUSTER_ID, probe -> ready"
  else
    fail cluster-register "cluster probe not ready: $(api GET "/api/v1/clusters/$CLUSTER_ID")"
  fi
fi

# ---------- 4. situation (态势) ----------

scenario "Situation (态势): fleet health + nodes"
FLEET="$(api GET /api/v1/fleet/health)"
FLEET_IDS="$(jq -r '[.items[].cluster_id // .items[].id] | join(",")' <<<"$FLEET" 2>/dev/null || true)"
NODES="$(api GET "/api/v1/clusters/$CLUSTER_ID/nodes")"
NODE_NAMES="$(jq -r '[.items[].metadata.name] | join(",")' <<<"$NODES" 2>/dev/null || true)"
if [[ "$NODE_NAMES" == *"demo-node"* ]]; then
  pass situation "fleet + nodes view: demo-node present (nodes=$NODE_NAMES)"
else
  fail situation "demo-node missing (fleet_ids=$FLEET_IDS nodes=$NODE_NAMES)"
fi

# ---------- 5. root cause (根因) ----------

scenario "Root cause (根因): deterministic Node + Pod diagnosis"
NODE_DIAG="$(api POST "/api/v1/clusters/$CLUSTER_ID/diagnoses" '{"resource_kind":"Node","name":"demo-node"}')"
NODE_DIAG_ID="$(jq -r '.id // 0' <<<"$NODE_DIAG")"
NODE_DIAG_RULE="$(jq -r '.rule_id // empty' <<<"$NODE_DIAG")"
NODE_DIAG_SEV="$(jq -r '.severity // empty' <<<"$NODE_DIAG")"
if [[ -n "$NODE_DIAG_ID" && "$NODE_DIAG_ID" != "0" ]] && [[ "$NODE_DIAG_RULE" == *"not_ready"* ]] && [[ "$NODE_DIAG_SEV" == "critical" ]]; then
  pass root-cause-node "Node diagnosis id=$NODE_DIAG_ID rule=$NODE_DIAG_RULE severity=$NODE_DIAG_SEV"
else
  fail root-cause-node "unexpected: $NODE_DIAG"
fi
POD_DIAG="$(api POST "/api/v1/clusters/$CLUSTER_ID/diagnoses" '{"resource_kind":"Pod","namespace":"demo","name":"demo-pod"}')"
POD_DIAG_ID="$(jq -r '.id // 0' <<<"$POD_DIAG")"
POD_DIAG_RULE="$(jq -r '.rule_id // empty' <<<"$POD_DIAG")"
if [[ -n "$POD_DIAG_ID" && "$POD_DIAG_ID" != "0" ]] && [[ "$POD_DIAG_RULE" == *"oom"* ]]; then
  pass root-cause-pod "Pod diagnosis id=$POD_DIAG_ID rule=$POD_DIAG_RULE severity=$(jq -r '.severity' <<<"$POD_DIAG")"
else
  fail root-cause-pod "unexpected: $POD_DIAG"
fi

# ---------- 6. evidence (证据) ----------

scenario "Evidence (证据): traceable root cause, evidence and recommendations"
NODE_VIEW="$(api GET "/api/v1/diagnoses/$NODE_DIAG_ID")"
NODE_EV="$(jq '.evidence | length' <<<"$NODE_VIEW")"
NODE_RC="$(jq '.root_causes | length' <<<"$NODE_VIEW")"
NODE_REC="$(jq '.recommendations | length' <<<"$NODE_VIEW")"
POD_VIEW="$(api GET "/api/v1/diagnoses/$POD_DIAG_ID")"
POD_TERM="$(jq '[.evidence[] | select(.type == "container_termination")] | length' <<<"$POD_VIEW")"
if [[ "$NODE_EV" -ge 1 && "$NODE_RC" -ge 1 && "$NODE_REC" -ge 1 ]]; then
  pass evidence-node "node evidence=$NODE_EV root_causes=$NODE_RC recommendations=$NODE_REC"
else
  fail evidence-node "node evidence=$NODE_EV rc=$NODE_RC rec=$NODE_REC"
fi
if [[ "$POD_TERM" -ge 1 ]]; then
  pass evidence-pod "pod container_termination evidence count=$POD_TERM"
else
  fail evidence-pod "no container_termination evidence"
fi

# ---------- 7. controlled action (受控动作) ----------

scenario "Controlled action (受控动作): confirm -> preview -> execute rollout restart"
api PATCH "/api/v1/diagnoses/$POD_DIAG_ID" '{"status":"confirmed","comment":"confirmed during demo drill"}' >/dev/null
STATUS_AFTER="$(api GET "/api/v1/diagnoses/$POD_DIAG_ID" | jq -r '.status')"
if [[ "$STATUS_AFTER" != "confirmed" ]]; then
  fail action-confirm "diagnosis status=$STATUS_AFTER"
else
  pass action-confirm "diagnosis confirmed"
fi
PREVIEW="$(api POST "/api/v1/diagnoses/$POD_DIAG_ID/remediations/preview" '{"action":"deployment.rollout_restart","target_name":"demo-app"}')"
PLAN_ID="$(jq -r '.id // empty' <<<"$PREVIEW")"
CONF_TOKEN="$(jq -r '.confirmation_token // empty' <<<"$PREVIEW")"
if [[ -n "$PLAN_ID" && -n "$CONF_TOKEN" ]]; then
  pass action-preview "remediation plan $PLAN_ID created (confirmation required)"
else
  fail action-preview "preview failed: $PREVIEW"
fi
if [[ -n "$PLAN_ID" && -n "$CONF_TOKEN" ]]; then
  EXECUTED="$(curl -s -m 15 -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -H "Idempotency-Key: demo-drill-$RUN_ID" -d "{\"confirmation_token\":\"$CONF_TOKEN\"}" \
    "$BASE/api/v1/remediations/$PLAN_ID/execute")"
  PLAN_STATUS="$(jq -r '.status // empty' <<<"$EXECUTED")"
  if [[ "$PLAN_STATUS" == "succeeded" ]]; then
    pass action-execute "rollout restart executed (plan status=succeeded)"
  else
    fail action-execute "execute failed: $EXECUTED"
  fi
fi

# ---------- 8. verify (验证) ----------

scenario "Verify (验证): the controlled action landed on the cluster"
DEPLOY="$(api GET "/api/v1/clusters/$CLUSTER_ID/deployments/demo/demo-app")"
DEPLOY_CODE="$(jq -r '.metadata.name // empty' <<<"$DEPLOY")"
MUTATIONS="$(curl -sk -m 10 "https://127.0.0.1:21443/mock/mutations")"
MUT_COUNT="$(jq '.items | length' <<<"$MUTATIONS" 2>/dev/null || echo 0)"
RESTART_AT="$(jq -r '.items[] | select(.kind == "Deployment") | .patch.spec.template.metadata.annotations["k8s-aiops.local/restarted-at"] // empty' <<<"$MUTATIONS" 2>/dev/null | head -1 || true)"
REMED_ID="$(jq -r '.items[] | select(.kind == "Deployment") | .patch.spec.template.metadata.annotations["k8s-aiops.local/remediation-id"] // empty' <<<"$MUTATIONS" 2>/dev/null | head -1 || true)"
if [[ "$DEPLOY_CODE" == "demo-app" && -n "$RESTART_AT" && -n "$REMED_ID" && "$MUT_COUNT" -ge 1 ]]; then
  pass action-verify "mock recorded $(jq -r '.items | length' <<<"$MUTATIONS") mutation(s); restarted-at=$RESTART_AT remediation-id=$REMED_ID; platform deployment view OK"
else
  fail action-verify "mutations=$MUT_COUNT deploy=$DEPLOY_CODE restarted_at=${RESTART_AT:-empty} remediation_id=${REMED_ID:-empty}"
fi

# ---------- 9. incident workspace (事故复盘) ----------

scenario "Incident workspace (事故复盘): create -> note -> resolve -> postmortem -> export"
INCIDENT="$(api POST /api/v1/incidents "{\"source_type\":\"diagnosis\",\"source_ref\":\"$POD_DIAG_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"critical\",\"title\":\"demo drill incident\",\"resource\":{\"kind\":\"Pod\",\"namespace\":\"demo\",\"name\":\"demo-pod\"}}")"
INCIDENT_ID="$(jq -r '.id // 0' <<<"$INCIDENT")"
INCIDENT_VERSION="$(jq -r '.version // 0' <<<"$INCIDENT")"
if [[ -z "$INCIDENT_ID" || "$INCIDENT_ID" == "0" ]]; then
  fail incident-create "create failed: $INCIDENT"
else
  pass incident-create "incident id=$INCIDENT_ID version=$INCIDENT_VERSION"
  api PATCH "/api/v1/incidents/$INCIDENT_ID" "{\"expected_version\":$INCIDENT_VERSION,\"status\":\"confirmed\",\"comment\":\"investigating during demo drill\"}" >/dev/null
  NEXT_VERSION="$(api GET "/api/v1/incidents/$INCIDENT_ID" | jq -r '.version')"
  api POST "/api/v1/incidents/$INCIDENT_ID/notes" "{\"expected_version\":$NEXT_VERSION,\"content\":\"root cause confirmed: OOMKilled container in demo-pod\"}" >/dev/null
  NEXT_VERSION="$(api GET "/api/v1/incidents/$INCIDENT_ID" | jq -r '.version')"
  api PATCH "/api/v1/incidents/$INCIDENT_ID" "{\"expected_version\":$NEXT_VERSION,\"status\":\"resolved\",\"comment\":\"controlled rollout restart applied\"}" >/dev/null
  NEXT_VERSION="$(api GET "/api/v1/incidents/$INCIDENT_ID" | jq -r '.version')"
  api PUT "/api/v1/incidents/$INCIDENT_ID/postmortem" "{\"expected_version\":$NEXT_VERSION,\"content\":\"postmortem: demo drill\"}" >/dev/null
  FINAL_STATUS="$(api GET "/api/v1/incidents/$INCIDENT_ID" | jq -r '.status')"
  EXPORT_HEAD="$(curl -s -m 10 -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/incidents/$INCIDENT_ID/export" | head -c 120)"
  SUMMARY="$(api GET /api/v1/incidents/summary)"
  TOTAL_INC="$(jq -r '.total // 0' <<<"$SUMMARY")"
  if [[ "$FINAL_STATUS" == "resolved" ]] && [[ "$EXPORT_HEAD" == *"id"* || "$EXPORT_HEAD" == *"number"* || "$EXPORT_HEAD" == *","* ]]; then
    pass incident-journey "incident resolved, postmortem set, CSV export started with: $(echo "$EXPORT_HEAD" | head -c 60)…, summary total=$TOTAL_INC"
  else
    fail incident-journey "status=$FINAL_STATUS export_head=$(echo "$EXPORT_HEAD" | head -c 60) summary_total=$TOTAL_INC"
  fi
fi

# ---------- 10. cleanup ----------

scenario "Cleanup"
cleanup_stack
REMAINING="$(docker compose -f "$compose_file" ps -a --format '{{.Name}}' 2>/dev/null || true)"
if [[ -z "$REMAINING" ]]; then
  pass cleanup "isolated demo stack fully removed (down -v)"
else
  fail cleanup "leftovers: $REMAINING"
fi

# ---------- report ----------

PASSED="$(grep -c '|pass|' "$WORK/results.txt" || true)"
FAILED="$(grep -c '|fail|' "$WORK/results.txt" || true)"
{
  echo "{"
  echo "  \"schema\": \"aiops.demo-drill/v1\","
  echo "  \"run_id\": \"$RUN_ID\","
  echo "  \"images\": {"
  echo "    \"backend\": \"$BACKEND_IMAGE\","
  echo "    \"frontend\": \"$FRONTEND_IMAGE\","
  echo "    \"postgres\": \"$PG_IMAGE\","
  echo "    \"demo_mock\": \"$MOCK_IMAGE\""
  echo "  },"
  echo "  \"evidence\": {"
  echo "    \"cluster_id\": $CLUSTER_ID,"
  echo "    \"node_diagnosis\": { \"id\": $NODE_DIAG_ID, \"rule_id\": \"$NODE_DIAG_RULE\", \"severity\": \"$NODE_DIAG_SEV\" },"
  echo "    \"pod_diagnosis\": { \"id\": $POD_DIAG_ID, \"rule_id\": \"$POD_DIAG_RULE\", \"severity\": \"$(jq -r '.severity' <<<"$POD_DIAG")\" },"
  echo "    \"remediation_plan\": \"$PLAN_ID\","
  echo "    \"deployment_annotations\": { \"restarted_at\": \"$RESTART_AT\", \"remediation_id\": \"$REMED_ID\" },"
  echo "    \"incident\": { \"id\": $INCIDENT_ID, \"status\": \"$FINAL_STATUS\", \"version\": $NEXT_VERSION }"
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
echo "RESULT demo-drill: PASS=$PASSED FAIL=$FAILED"
echo "Report: $REPORT"
if [[ "$fail_any" == "1" || "$FAILED" != "0" ]]; then
  exit 1
fi
exit 0
