#!/usr/bin/env bash
# M102+ (local track): reproducible demo drill — 登录 → 态势 → 根因 → 证据 →
# 受控动作 → 验证 → 事故复盘 → 告警/巡检提升为事故, against a fully isolated offline compose
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
      POSTGRES_PASSWORD: admin123
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
      DATABASE_URL: postgres://aiops:admin123@postgres:5432/aiops?sslmode=disable
      JWT_SIGNING_KEY: $JWT_KEY
      ACCESS_TOKEN_TTL: 15m
      REFRESH_TOKEN_TTL: 168h
      BOOTSTRAP_ADMIN_USERNAME: admin
      BOOTSTRAP_ADMIN_PASSWORD: admin123
      CREDENTIAL_ENCRYPTION_KEY: $CRED_KEY
      CREDENTIAL_KEY_VERSION: v1
      CREDENTIAL_DECRYPTION_KEYS: ""
      METRICS_HISTORY_ENABLED: "true"
      AI_ENABLED: "false"
      NOTIFICATION_ENABLED: "false"
      ALERT_ENABLED: "true"
      METRICS_COLLECTION_INTERVAL: 15s
      ALERT_POLL_INTERVAL: 2s
      ALERT_MIN_EVALUATION_INTERVAL: 15s
      SIGNAL_DIAGNOSIS_INGESTION: "true"
      SIGNAL_DIAGNOSIS_DRAIN_INTERVAL: 2s
      CORRELATION_INTERVAL: 30s
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
  -d '{"username":"admin","password":"admin123"}' 2>/dev/null || true)"
TOKEN="$(jq -r '.access_token // empty' <<<"$LOGIN" 2>/dev/null || true)"
ME="$(api GET /api/v1/auth/me)"
UNAME="$(jq -r '.username // empty' <<<"$ME")"
ROLE="$(jq -r '.roles[0] // empty' <<<"$ME")"
ADMIN_ID="$(jq -r '.id // 0' <<<"$ME")"
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


# ---------- 7. replay (回放，动作前) ----------

scenario "Replay (回放): read-only insight-chain replay before actions"
NODE_REPLAY="$(api GET "/api/v1/diagnoses/$NODE_DIAG_ID/replay")"
NODE_REPLAY_SCHEMA="$(jq -r '.schema // empty' <<<"$NODE_REPLAY")"
NODE_REPLAY_DIAG="$(jq -r '.diagnosis_id // 0' <<<"$NODE_REPLAY")"
NODE_REPLAY_STEPS="$(jq '.steps | length' <<<"$NODE_REPLAY")"
NODE_REPLAY_STAGES="$(jq -r '[.stages[].stage] | join(",")' <<<"$NODE_REPLAY")"
NODE_REPLAY_SORTED="$(jq -r '[.steps[].occurred_at] == ([.steps[].occurred_at] | sort)' <<<"$NODE_REPLAY")"
if [[ "$NODE_REPLAY_SCHEMA" == "aiops.diagnosis-replay/v1" && "$NODE_REPLAY_DIAG" == "$NODE_DIAG_ID" && "$NODE_REPLAY_STEPS" -ge 2 && "$NODE_REPLAY_STAGES" == *"diagnosis_created"* && "$NODE_REPLAY_STAGES" == *"evidence"* && "$NODE_REPLAY_SORTED" == "true" ]]; then
  pass replay-before "node replay schema ok steps=$NODE_REPLAY_STEPS stages=[$NODE_REPLAY_STAGES] time-sorted"
else
  fail replay-before "schema=$NODE_REPLAY_SCHEMA diag=$NODE_REPLAY_DIAG steps=$NODE_REPLAY_STEPS stages=[$NODE_REPLAY_STAGES] sorted=$NODE_REPLAY_SORTED"
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


# ---------- 9. replay after actions (回放，动作后) ----------

scenario "Replay after actions (回放): chain now includes activity + remediation"
POD_REPLAY="$(api GET "/api/v1/diagnoses/$POD_DIAG_ID/replay")"
POD_REPLAY_STAGES="$(jq -r '[.stages[].stage] | join(",")' <<<"$POD_REPLAY")"
POD_REPLAY_TYPES="$(jq -r '[.steps[].type] | join(",")' <<<"$POD_REPLAY")"
POD_REPLAY_SORTED="$(jq -r '[.steps[].occurred_at] == ([.steps[].occurred_at] | sort)' <<<"$POD_REPLAY")"
if [[ "$POD_REPLAY_STAGES" == *"activity"* && "$POD_REPLAY_STAGES" == *"remediation"* && "$POD_REPLAY_TYPES" == *"remediation_created"* && "$POD_REPLAY_TYPES" == *"remediation_executed"* && "$POD_REPLAY_SORTED" == "true" ]]; then
  pass replay-after "pod replay stages=[$POD_REPLAY_STAGES] types=[$POD_REPLAY_TYPES] time-sorted"
else
  fail replay-after "stages=[$POD_REPLAY_STAGES] types=[$POD_REPLAY_TYPES] sorted=$POD_REPLAY_SORTED"
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
  EVIDENCE="$(api GET "/api/v1/incidents/$INCIDENT_ID/evidence")"
  EV_TYPE="$(jq -r '.items[0].source_type // ""' <<<"$EVIDENCE")"
  EV_LINK="$(jq -r '.items[0].deep_link // ""' <<<"$EVIDENCE")"
  EV_TITLE="$(jq -r '.items[0].title // ""' <<<"$EVIDENCE")"
  if [[ "$FINAL_STATUS" == "resolved" ]] && [[ "$EXPORT_HEAD" == *"id"* || "$EXPORT_HEAD" == *"number"* || "$EXPORT_HEAD" == *","* ]]; then
    pass incident-journey "incident resolved, postmortem set, CSV export started with: $(echo "$EXPORT_HEAD" | head -c 60)…, summary total=$TOTAL_INC"
  else
    fail incident-journey "status=$FINAL_STATUS export_head=$(echo "$EXPORT_HEAD" | head -c 60) summary_total=$TOTAL_INC"
  fi
  if [[ "$EV_TYPE" == "diagnosis" ]] && [[ "$EV_LINK" == "/diagnoses" ]] && [[ -n "$EV_TITLE" ]]; then
    pass incident-evidence "evidence timeline returns diagnosis source (deep_link=/diagnoses): ${EV_TITLE:0:50}"
  else
    fail incident-evidence "evidence type=$EV_TYPE deep_link=$EV_LINK title=$EV_TITLE"
  fi
  BATCH="$(api POST /api/v1/incidents/batch-assign "{\"incident_ids\":[\"$INCIDENT_ID\"],\"assignee_user_id\":$ADMIN_ID,\"comment\":\"demo drill batch handoff\"}")"
  BATCH_ASSIGNED="$(jq -r '.assigned // 0' <<<"$BATCH")"
  BATCH_TOTAL="$(jq -r '.total // 0' <<<"$BATCH")"
  if [[ "$BATCH_ASSIGNED" -ge 1 && "$BATCH_TOTAL" -ge 1 ]]; then
    pass incident-batch-assign "batch assign endpoint handoff (assigned=$BATCH_ASSIGNED total=$BATCH_TOTAL)"
  else
    fail incident-batch-assign "batch response=$BATCH"
  fi
fi

# ---------- 10. alert -> incident (告警提升为事故) ----------

scenario "Alert -> incident (告警提升为事故): firing alert rule promotes to incident workspace"
ALERT_RULE="$(api POST "/api/v1/clusters/$CLUSTER_ID/alert-rules" "{\"display_name\":\"High CPU demo-node\",\"resource_kind\":\"Node\",\"resource_name\":\"demo-node\",\"metric_name\":\"cpu\",\"operator\":\"gte\",\"threshold\":2000000000,\"for_seconds\":60,\"minimum_points\":2}")"
ALERT_RULE_ID="$(jq -r '.id // 0' <<<"$ALERT_RULE")"
FIRING_INSTANCE=""
if [[ -z "$ALERT_RULE_ID" || "$ALERT_RULE_ID" == "0" ]]; then
  fail alert-rule-create "rule create failed: $ALERT_RULE"
else
  pass alert-rule-create "alert rule id=$ALERT_RULE_ID (node cpu >= 2 cores)"
  # 后端告警调度器按 2s 轮询；mock 的 demo-node CPU 恒为 3500m (> 2 核)，必触发。
  for attempt in $(seq 1 90); do
    FIRING_INSTANCE="$(api GET "/api/v1/clusters/$CLUSTER_ID/alerts?state=firing&limit=50")"
    ALERT_INSTANCE_IDS="$(jq -r --argjson rid "$ALERT_RULE_ID" '[.[] | select(.rule_id == $rid)] | length' <<<"$FIRING_INSTANCE" 2>/dev/null || echo 0)"
    if [[ "$ALERT_INSTANCE_IDS" -ge 1 ]]; then
      break
    fi
    sleep 2
  done
  ALERT_INST_ID="$(jq -r --argjson rid "$ALERT_RULE_ID" '.[] | select(.rule_id == $rid) | .id' <<<"$FIRING_INSTANCE" 2>/dev/null | head -1 || true)"
  ALERT_DIAG_ID="$(jq -r --argjson rid "$ALERT_RULE_ID" '.[] | select(.rule_id == $rid) | .diagnosis_id' <<<"$FIRING_INSTANCE" 2>/dev/null | head -1 || true)"
  if [[ -n "$ALERT_INST_ID" && "$ALERT_INST_ID" != "0" ]]; then
    pass alert-fire "firing alert instance id=$ALERT_INST_ID diagnosis_id=$ALERT_DIAG_ID"
    ALERT_INC="$(api POST /api/v1/incidents "{\"source_type\":\"alert\",\"source_ref\":\"alert:$ALERT_INST_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"high\",\"title\":\"alert promoted incident\"}")"
    ALERT_INC_ID="$(jq -r '.id // 0' <<<"$ALERT_INC")"
    ALERT_INC_SEV="$(jq -r '.severity // empty' <<<"$ALERT_INC")"
    ALERT_INC_SRC="$(jq -r '.source_type // empty' <<<"$ALERT_INC")"
    if [[ -n "$ALERT_INC_ID" && "$ALERT_INC_ID" != "0" ]] && [[ "$ALERT_INC_SRC" == "alert" ]]; then
      pass alert-incident "incident $ALERT_INC_ID created from alert #$ALERT_INST_ID, severity=$ALERT_INC_SEV (enriched from diagnosis)"
      if [[ "$ALERT_INC_SEV" == "high" ]]; then
        pass alert-incident-severity "alert incident severity enriched to high from linked CPU-breach diagnosis"
      else
        fail alert-incident-severity "severity=$ALERT_INC_SEV expected high (CPU breach)"
      fi
      # 同一告警实例不可重复提升（dedup）。
      DUP="$(api POST /api/v1/incidents "{\"source_type\":\"alert\",\"source_ref\":\"alert:$ALERT_INST_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"high\",\"title\":\"dup\"}")"
      DUP_FAIL="$(jq -r '.code // empty' <<<"$DUP" 2>/dev/null || true)"
      if [[ "$DUP_FAIL" == *"SOURCE_ALREADY_USED"* ]] || [[ "$(jq -r '.id // 0' <<<"$DUP")" == "0" ]]; then
        pass alert-incident-dedup "duplicate alert promote rejected (SOURCE_ALREADY_USED)"
      else
        fail alert-incident-dedup "duplicate was not rejected: $DUP"
      fi
    else
      fail alert-incident "incident create failed: $ALERT_INC"
    fi
  else
    fail alert-fire "no firing alert instance after retries (rule_id=$ALERT_RULE_ID)"
  fi
fi

# ---------- 11. inspection -> incident (巡检结果提升为事故) ----------

scenario "Inspection -> incident (巡检结果提升为事故): node_not_ready result promotes to incident workspace"
INSP_RUN="$(api POST /api/v1/aiops/inspection/run "{\"cluster_ids\":[$CLUSTER_ID],\"rule_codes\":[\"node_not_ready\"]}")"
INSP_TASK_ID="$(jq -r '.id // 0' <<<"$INSP_RUN")"
INSP_TASK_STATUS="$(jq -r '.status // empty' <<<"$INSP_RUN")"
if [[ -z "$INSP_TASK_ID" || "$INSP_TASK_ID" == "0" ]] || [[ "$INSP_TASK_STATUS" != "pending" && "$INSP_TASK_STATUS" != "running" ]]; then
  fail inspection-run "inspection run failed: $INSP_RUN"
else
  pass inspection-run "inspection task id=$INSP_TASK_ID accepted (status=$INSP_TASK_STATUS)"
  # mock 的 demo-node 恒 NotReady，node_not_ready 必命中；轮询任务直到 completed。
  for attempt in $(seq 1 60); do
    INSP_TASK="$(api GET "/api/v1/aiops/inspection/tasks/$INSP_TASK_ID")"
    INSP_TASK_STATUS="$(jq -r '.status // empty' <<<"$INSP_TASK")"
    [[ "$INSP_TASK_STATUS" == "completed" || "$INSP_TASK_STATUS" == "failed" ]] && break
    sleep 2
  done
  if [[ "$INSP_TASK_STATUS" != "completed" ]]; then
    fail inspection-task "inspection task not completed: $INSP_TASK"
  else
    pass inspection-task "inspection task completed (findings=$(jq -r '.finding_count' <<<"$INSP_TASK"))"
    INSP_RESULTS="$(api GET "/api/v1/aiops/inspection/results?task_id=$INSP_TASK_ID&cluster_id=$CLUSTER_ID")"
    INSP_RESULT_ID="$(jq -r --arg code node_not_ready '.items[] | select(.rule_code == $code) | .id' <<<"$INSP_RESULTS" | head -1 || true)"
    if [[ -n "$INSP_RESULT_ID" && "$INSP_RESULT_ID" != "0" ]]; then
      pass inspection-result "inspection result id=$INSP_RESULT_ID (node_not_ready, cluster=$CLUSTER_ID)"
      INSP_INC="$(api POST /api/v1/incidents "{\"source_type\":\"inspection\",\"source_ref\":\"inspection:$INSP_RESULT_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"info\",\"title\":\"inspection promoted incident\"}")"
      INSP_INC_ID="$(jq -r '.id // 0' <<<"$INSP_INC")"
      INSP_INC_SEV="$(jq -r '.severity // empty' <<<"$INSP_INC")"
      INSP_INC_SRC="$(jq -r '.source_type // empty' <<<"$INSP_INC")"
      if [[ -n "$INSP_INC_ID" && "$INSP_INC_ID" != "0" ]] && [[ "$INSP_INC_SRC" == "inspection" ]]; then
        pass inspection-incident "incident $INSP_INC_ID created from inspection result #$INSP_RESULT_ID, severity=$INSP_INC_SEV (enriched from result)"
        if [[ "$INSP_INC_SEV" == "critical" ]]; then
          pass inspection-incident-severity "inspection incident severity enriched to critical from node_not_ready result"
        else
          fail inspection-incident-severity "severity=$INSP_INC_SEV expected critical (node_not_ready)"
        fi
        # 同一巡检结果不可重复提升（dedup）。
        DUP="$(api POST /api/v1/incidents "{\"source_type\":\"inspection\",\"source_ref\":\"inspection:$INSP_RESULT_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"info\",\"title\":\"dup\"}")"
        DUP_FAIL="$(jq -r '.code // empty' <<<"$DUP" 2>/dev/null || true)"
        if [[ "$DUP_FAIL" == *"SOURCE_ALREADY_USED"* ]] || [[ "$(jq -r '.id // 0' <<<"$DUP")" == "0" ]]; then
          pass inspection-incident-dedup "duplicate inspection promote rejected (SOURCE_ALREADY_USED)"
        else
          fail inspection-incident-dedup "duplicate was not rejected: $DUP"
        fi
      else
        fail inspection-incident "incident create failed: $INSP_INC"
      fi
    else
      fail inspection-result "no node_not_ready result after task completion: $INSP_RESULTS"
    fi
  fi
fi

# ---------- 12. signal -> incident (信号实例提升为事故) ----------

scenario "Signal -> incident (信号实例提升为事故): diagnosis signal occurrence promotes to incident workspace"
# demo 的 Node 诊断恒 critical；drain worker（2s 轮询）会把诊断归一化进
# signal_occurrences（producer=diagnosis, diag.node.not_ready.v1）。
DIAG_SIGNAL_ID=""
for attempt in $(seq 1 60); do
  DIAG_SIGNALS="$(api GET "/api/v1/aiops/signals?cluster_id=$CLUSTER_ID&signal_id=diag.node.not_ready.v1&limit=10")"
  DIAG_SIGNAL_ID="$(jq -r '.items[] | select(.state == "active") | .id' <<<"$DIAG_SIGNALS" | head -1 || true)"
  [[ -n "$DIAG_SIGNAL_ID" ]] && break
  sleep 2
done
if [[ -n "$DIAG_SIGNAL_ID" && "$DIAG_SIGNAL_ID" != "0" ]]; then
  pass signal-ingest "diagnosis signal occurrence id=$DIAG_SIGNAL_ID (diag.node.not_ready.v1, producer=diagnosis)"
  SIGNAL_INC="$(api POST /api/v1/incidents "{\"source_type\":\"signal\",\"source_ref\":\"signal:$DIAG_SIGNAL_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"info\",\"title\":\"signal promoted incident\"}")"
  SIGNAL_INC_ID="$(jq -r '.id // 0' <<<"$SIGNAL_INC")"
  SIGNAL_INC_SEV="$(jq -r '.severity // empty' <<<"$SIGNAL_INC")"
  SIGNAL_INC_SRC="$(jq -r '.source_type // empty' <<<"$SIGNAL_INC")"
  if [[ -n "$SIGNAL_INC_ID" && "$SIGNAL_INC_ID" != "0" ]] && [[ "$SIGNAL_INC_SRC" == "signal" ]]; then
    pass signal-incident "incident $SIGNAL_INC_ID created from signal #$DIAG_SIGNAL_ID, severity=$SIGNAL_INC_SEV (enriched from occurrence)"
    if [[ "$SIGNAL_INC_SEV" == "critical" ]]; then
      pass signal-incident-severity "signal incident severity enriched to critical from diag.node.not_ready.v1"
    else
      fail signal-incident-severity "severity=$SIGNAL_INC_SEV expected critical (diag.node.not_ready.v1)"
    fi
    # 同一信号不可重复提升（dedup）。
    DUP="$(api POST /api/v1/incidents "{\"source_type\":\"signal\",\"source_ref\":\"signal:$DIAG_SIGNAL_ID\",\"cluster_id\":$CLUSTER_ID,\"severity\":\"info\",\"title\":\"dup\"}")"
    DUP_FAIL="$(jq -r '.code // empty' <<<"$DUP" 2>/dev/null || true)"
    if [[ "$DUP_FAIL" == *"SOURCE_ALREADY_USED"* ]] || [[ "$(jq -r '.id // 0' <<<"$DUP")" == "0" ]]; then
      pass signal-incident-dedup "duplicate signal promote rejected (SOURCE_ALREADY_USED)"
    else
      fail signal-incident-dedup "duplicate was not rejected: $DUP"
    fi
  else
    fail signal-incident "incident create failed: $SIGNAL_INC"
  fi
else
  fail signal-ingest "no active diag.node.not_ready.v1 signal after retries: $DIAG_SIGNALS"
fi

# ---------- 13. correlation -> incident (关联案例提升为事故) ----------

scenario "Correlation case -> incident (关联案例提升为事故): correlation case promotes to incident workspace"
# 关联引擎（M43+）将 diag.node.not_ready.v1 信号归一为 maintenance_causes_node_failure
# 冷启动案例；demo compose 把 CORRELATION_INTERVAL 压到 30s 保证轮询窗口内出案例。
CORR_CASE_ID=""
for attempt in $(seq 1 60); do
  CORR_CASES="$(api GET "/api/v1/aiops/correlation/cases?cluster_id=$CLUSTER_ID&limit=20")"
  CORR_CASE_ID="$(jq -r '.items[] | select(.status == "active") | .id' <<<"$CORR_CASES" | head -1 || true)"
  [[ -n "$CORR_CASE_ID" && "$CORR_CASE_ID" != "0" ]] && break
  sleep 2
done
if [[ -n "$CORR_CASE_ID" && "$CORR_CASE_ID" != "0" ]]; then
  pass correlation-case-ingest "correlation case id=$CORR_CASE_ID (cluster #$CLUSTER_ID)"
  CORR_INC="$(api POST /api/v1/incidents "{\"source_type\":\"correlation\",\"source_ref\":\"correlation:$CORR_CASE_ID\",\"cluster_id\":$CLUSTER_ID,\"title\":\"correlation promoted incident\"}")"
  CORR_INC_ID="$(jq -r '.id // 0' <<<"$CORR_INC")"
  CORR_INC_SRC="$(jq -r '.source_type // empty' <<<"$CORR_INC")"
  CORR_INC_SEV="$(jq -r '.severity // empty' <<<"$CORR_INC")"
  if [[ -n "$CORR_INC_ID" && "$CORR_INC_ID" != "0" ]] && [[ "$CORR_INC_SRC" == "correlation" ]]; then
    pass correlation-incident "incident $CORR_INC_ID created from correlation case #$CORR_CASE_ID, severity=$CORR_INC_SEV (enriched)"
    if [[ -n "$CORR_INC_SEV" ]]; then
      pass correlation-incident-severity "correlation incident severity enriched to $CORR_INC_SEV from case confidence"
    else
      fail correlation-incident-severity "severity missing on correlation incident"
    fi
    # 同一关联案例不可重复提升（dedup）。
    DUP="$(api POST /api/v1/incidents "{\"source_type\":\"correlation\",\"source_ref\":\"correlation:$CORR_CASE_ID\",\"cluster_id\":$CLUSTER_ID,\"title\":\"dup\"}")"
    DUP_FAIL="$(jq -r '.code // empty' <<<"$DUP" 2>/dev/null || true)"
    if [[ "$DUP_FAIL" == *"SOURCE_ALREADY_USED"* ]] || [[ "$(jq -r '.id // 0' <<<"$DUP")" == "0" ]]; then
      pass correlation-incident-dedup "duplicate correlation promote rejected (SOURCE_ALREADY_USED)"
    else
      fail correlation-incident-dedup "duplicate was not rejected: $DUP"
    fi
    # M108 Block 2: case view links back to the incident workspace
    # (bidirectional deep link enrichment).
    LINKED="$(api GET "/api/v1/aiops/correlation/cases/$CORR_CASE_ID")"
    LINKED_INC_ID="$(jq -r '.incident.id // 0' <<<"$LINKED")"
    if [[ -n "$LINKED_INC_ID" && "$LINKED_INC_ID" != "0" ]] && [[ "$LINKED_INC_ID" == "$CORR_INC_ID" ]]; then
      pass correlation-incident-deeplink "case view links incident #$LINKED_INC_ID (bidirectional deep link)"
    else
      fail correlation-incident-deeplink "case view incident = ${LINKED_INC_ID:-missing}, want $CORR_INC_ID"
    fi
  else
    fail correlation-incident "incident create failed: $CORR_INC"
  fi
else
  fail correlation-case-ingest "no active correlation case after retries: $CORR_CASES"
fi

# M108 验收「2/2 信号归并」：demo 的第二个诊断信号（pod.oom_killed → rollout
# causes pod failure 规则）也应归一出独立案例并可提升为事故。冷启动路径不依赖
# change event，CORRELATION_INTERVAL=30s 保证轮询窗口内出案例。
CORR_CASE2_ID=""
for attempt in $(seq 1 60); do
  CORR_CASES2="$(api GET "/api/v1/aiops/correlation/cases?cluster_id=$CLUSTER_ID&limit=20")"
  CORR_CASE2_ID="$(jq -r --argjson exclude "${CORR_CASE_ID:-0}" '.items[] | select(.status == "active" and .id != $exclude) | .id' <<<"$CORR_CASES2" | head -1 || true)"
  [[ -n "$CORR_CASE2_ID" && "$CORR_CASE2_ID" != "0" ]] && break
  sleep 2
done
if [[ -n "$CORR_CASE2_ID" && "$CORR_CASE2_ID" != "0" ]]; then
  pass correlation-case-merge "second correlation case id=$CORR_CASE2_ID (2/2 signals correlated)"
  CORR2_INC="$(api POST /api/v1/incidents "{\"source_type\":\"correlation\",\"source_ref\":\"correlation:$CORR_CASE2_ID\",\"cluster_id\":$CLUSTER_ID,\"title\":\"correlation promoted incident 2\"}")"
  CORR2_INC_ID="$(jq -r '.id // 0' <<<"$CORR2_INC")"
  CORR2_INC_SRC="$(jq -r '.source_type // empty' <<<"$CORR2_INC")"
  if [[ -n "$CORR2_INC_ID" && "$CORR2_INC_ID" != "0" ]] && [[ "$CORR2_INC_SRC" == "correlation" ]]; then
    pass correlation-incident-merge "incident $CORR2_INC_ID created from correlation case #$CORR_CASE2_ID (2/2 promoted)"
  else
    fail correlation-incident-merge "incident create failed for case #$CORR_CASE2_ID: $CORR2_INC"
  fi
else
  fail correlation-case-merge "no second correlation case after retries: $CORR_CASES2"
fi

# ---------- 14. cleanup ----------

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
  echo "    \"incident\": { \"id\": $INCIDENT_ID, \"status\": \"$FINAL_STATUS\", \"version\": $NEXT_VERSION },"
  echo "    \"inspection_incident\": { \"result_id\": ${INSP_RESULT_ID:-0}, \"incident_id\": ${INSP_INC_ID:-0}, \"severity\": \"${INSP_INC_SEV:-}\" },"
  echo "    \"signal_incident\": { \"signal_id\": ${DIAG_SIGNAL_ID:-0}, \"incident_id\": ${SIGNAL_INC_ID:-0}, \"severity\": \"${SIGNAL_INC_SEV:-}\" },"
  echo "    \"correlation_incident\": { \"case_id\": ${CORR_CASE_ID:-0}, \"incident_id\": ${CORR_INC_ID:-0}, \"severity\": \"${CORR_INC_SEV:-}\" },"
  echo "    \"correlation_incident_2\": { \"case_id\": ${CORR_CASE2_ID:-0}, \"incident_id\": ${CORR2_INC_ID:-0} }"
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
