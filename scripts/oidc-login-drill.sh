#!/usr/bin/env bash
# M89: local OIDC provider login drill (identity track, no external network).
#
# Starts a repository-local OIDC provider (backend/cmd/oidc-provider) with a
# self-signed HTTPS certificate, starts a second instance of the platform
# backend (host binary) with OIDC enabled against the same local PostgreSQL,
# and drives the real Authorization Code + PKCE login over HTTP:
#
#   GET /api/v1/auth/oidc/login  -> 302 to provider /authorize (cookie set)
#   provider /authorize          -> 302 redirect_uri?code=..&state=.. (PKCE S256)
#   GET /api/v1/auth/oidc/callback -> 200 {access_token, user} + refresh cookie
#   GET /api/v1/auth/me          -> 200 with local role mapping
#
# Fail-closed paths covered: missing subject prelink (403), wrong nonce, wrong
# state, missing/unaccepted MFA evidence, unmapped groups, expired token,
# signing-key rotation (unknown kid), unsigned token, provider down at runtime
# (token endpoint unreachable -> 502; discovery cache expiry -> OIDC_UNAVAILABLE)
# and provider down at startup (server refuses to start).
#
# Prerequisites: local docker with the running platform postgres container
# (k8s-aiops-postgres-1, published 127.0.0.1:15432), Go toolchain, jq, python3.
#
# TLS trust: macOS Go validates HTTPS against the system keychain (it ignores
# SSL_CERT_FILE), so the drill temporarily trusts the IdP self-signed cert in
# the login keychain via `security add-trusted-cert` and removes it on exit.
# If the drill is killed hard (SIGKILL), remove the leftover trust with:
#   security delete-certificate -c aiops-drill-oidc-provider
#
# Exit 0 when every scenario matches expectations; exit 1 otherwise.
# Report: .artifacts/oidc-drill/report-<run>.json
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$(date +%Y%m%d-%H%M%S)-$(openssl rand -hex 3)"
WORK="$ROOT/.artifacts/oidc-drill/tmp-$RUN_ID"
ARTIFACTS="$ROOT/.artifacts/oidc-drill"
REPORT="$ARTIFACTS/report-$RUN_ID.json"
RESULTS_FILE="$WORK/results.txt"
IDP_PORT="${OIDC_DRILL_IDP_PORT:-9443}"
API_PORT="${OIDC_DRILL_API_PORT:-8090}"
FAIL_PORT="${OIDC_DRILL_FAIL_PORT:-8091}"
IDP_ISSUER="https://localhost:$IDP_PORT"
CLIENT_ID="aiops-platform"
REDIRECT_URI="https://localhost:$API_PORT/api/v1/auth/oidc/callback"
SIGNING_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

mkdir -p "$ARTIFACTS" "$WORK"
: > "$RESULTS_FILE"
fail_any=0

# ---------- helpers ----------

scenario() { echo; echo "== $1 =="; }

# results file lines: "<name>|pass|detail" or "<name>|fail|detail"
status_of() { awk -F'|' -v k="$1" '$1==k {print $2}' "$RESULTS_FILE" | head -1; }
detail_of() { awk -F'|' -v k="$1" '$1==k {sub(/^[^|]*\|[^|]*\|/, ""); print}' "$RESULTS_FILE" | head -1; }
pass() { echo "$1|pass|$2" >> "$RESULTS_FILE"; echo "  PASS: $2"; }
fail() { echo "$1|fail|$2" >> "$RESULTS_FILE"; echo "  FAIL: $2"; fail_any=1; }

die() { echo "FATAL: $*" >&2; cleanup; exit 1; }

url_param() { # url key -> prints URL-decoded value
  python3 - "$1" "$2" <<'PY'
import sys, urllib.parse as u
print(u.parse_qs(u.urlparse(sys.argv[1]).query).get(sys.argv[2], [''])[0])
PY
}

wait_for_http() { # url -> wait up to 60s for HTTP 200
  local url="$1" attempt
  for attempt in $(seq 1 60); do
    if curl -s -k -m 2 -o /dev/null "$url"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

# ---------- OIDC flow drivers ----------

oidc_start_login() { # jar
  local jar="$1"
  local out="$WORK/login-body.out" hdr="$WORK/login-hdr.out"
  OIDC_LOGIN_HTTP="$(curl -s -m 10 -c "$jar" -o "$out" -D "$hdr" -w '%{http_code}' \
    "http://127.0.0.1:$API_PORT/api/v1/auth/oidc/login" 2>/dev/null || echo 000)"
  OIDC_LOGIN_BODY="$(cat "$out" 2>/dev/null || true)"
  OIDC_AUTH_URL="$(sed -n 's/^[Ll]ocation:[[:space:]]*//p' "$hdr" 2>/dev/null | tr -d '\r' | head -1)"
}

oidc_authorize() { # jar extra_query_params
  local jar="$1" extra="$2" url="$OIDC_AUTH_URL"
  if [[ -n "$extra" ]]; then
    url="${url}&${extra}"
  fi
  OIDC_REDIRECT_URL="$(curl -s -k -m 10 -b "$jar" -o /dev/null -w '%{redirect_url}' "$url" 2>/dev/null || true)"
  OIDC_CODE="$(url_param "$OIDC_REDIRECT_URL" code)"
  OIDC_STATE="$(url_param "$OIDC_REDIRECT_URL" state)"
}

oidc_callback() { # jar state
  local jar="$1" state="$2"
  local out="$WORK/callback-body.out"
  OIDC_CB_HTTP="$(curl -s -m 20 -b "$jar" -o "$out" -w '%{http_code}' \
    "http://127.0.0.1:$API_PORT/api/v1/auth/oidc/callback?code=$OIDC_CODE&state=$state" 2>/dev/null || echo 000)"
  OIDC_CB_BODY="$(cat "$out" 2>/dev/null || true)"
}

oidc_full_login() { # jar extra_query_params
  oidc_start_login "$1"
  oidc_authorize "$1" "$2"
  oidc_callback "$1" "$OIDC_STATE"
}

cb_error_code() { jq -r '.code // empty' <<<"$OIDC_CB_BODY" 2>/dev/null || true; }

# ---------- cleanup ----------

BACKEND_PID=""
IDP_PID=""
FAIL_PID=""

cleanup() {
  [[ -n "$FAIL_PID" ]] && kill "$FAIL_PID" 2>/dev/null || true
  [[ -n "$BACKEND_PID" ]] && kill "$BACKEND_PID" 2>/dev/null || true
  [[ -n "$IDP_PID" ]] && kill "$IDP_PID" 2>/dev/null || true
  security delete-certificate -c "aiops-drill-oidc-provider" "$HOME/Library/Keychains/login.keychain-db" >/dev/null 2>&1 || true
  wait 2>/dev/null || true
}
trap cleanup EXIT

# ---------- prerequisites ----------

PG_CONTAINER="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'postgres' | head -1 || true)"
if [[ -z "$PG_CONTAINER" ]]; then
  die "no running postgres container found; start the platform compose stack first"
fi
PG_HOST_PORT="$(docker port "$PG_CONTAINER" 5432/tcp 2>/dev/null | head -1 | sed -n 's/.*://p' || true)"
PG_HOST_PORT="${PG_HOST_PORT:-15432}"
HOST_DB_URL="postgres://aiops:admin123@127.0.0.1:$PG_HOST_PORT/aiops?sslmode=disable"
echo "postgres container: $PG_CONTAINER (host port $PG_HOST_PORT)"

# ---------- build ----------

echo "building oidc-provider and platform server..."
( cd "$ROOT/backend" && go build -o "$WORK/oidc-provider" ./cmd/oidc-provider )
( cd "$ROOT/backend" && go build -o "$WORK/server" ./cmd/server )

# ---------- start IdP ----------

"$WORK/oidc-provider" \
  -issuer "$IDP_ISSUER" \
  -listen "127.0.0.1:$IDP_PORT" \
  -client-id "$CLIENT_ID" \
  -redirect-uri "$REDIRECT_URI" \
  -cert-out "$WORK/idp-cert.pem" \
  -key-out "$WORK/idp-key.pem" \
  > "$WORK/idp.log" 2>&1 &
IDP_PID=$!
if ! wait_for_http "https://localhost:$IDP_PORT/healthz"; then
  cat "$WORK/idp.log" >&2 || true
  die "OIDC provider did not become healthy"
fi
echo "oidc provider healthy: $IDP_ISSUER"

if ! security add-trusted-cert -d -r trustRoot -k "$HOME/Library/Keychains/login.keychain-db" "$WORK/idp-cert.pem" >/dev/null 2>&1; then
  die "unable to trust the drill IdP certificate in the login keychain"
fi
echo "trusted drill IdP certificate in login keychain"

cp "$ROOT/.env" "$WORK/backend.env"
cat >> "$WORK/backend.env" <<EOF
HTTP_ADDR=:$API_PORT
DATABASE_URL=$HOST_DB_URL
OIDC_ENABLED=true
OIDC_ISSUER=$IDP_ISSUER
OIDC_CLIENT_ID=$CLIENT_ID
OIDC_REDIRECT_URI=$REDIRECT_URI
OIDC_CLAIM_USERNAME=preferred_username
OIDC_CLAIM_DISPLAY_NAME=name
OIDC_CLAIM_GROUPS=groups
OIDC_ALLOWED_SIGNING_ALGORITHMS=RS256
OIDC_GROUP_TO_ROLES='{"platform-operators":["operations_admin"]}'
OIDC_MFA_REQUIRED=true
OIDC_MFA_EVIDENCE_CLAIM=acr
OIDC_MFA_ACCEPTED_VALUES=phr
OIDC_SESSION_MAX_AGE=8h
OIDC_SESSION_REAUTHENTICATION=1h
OIDC_SESSION_REVOKE_ON_DISABLE=true
OIDC_BREAK_GLASS_ENABLED=true
OIDC_BREAK_GLASS_MAX_ACCOUNTS=1
OIDC_JWKS_CACHE_TTL=1m
OIDC_JWKS_REFRESH_TIMEOUT=2s
OIDC_AUTH_SESSION_SIGNING_KEY=$SIGNING_KEY
EOF

(
  set -a
  source "$WORK/backend.env"
  set +a
  export SSL_CERT_FILE="$WORK/idp-cert.pem"
  exec "$WORK/server"
) > "$WORK/backend.log" 2>&1 &
BACKEND_PID=$!
if ! wait_for_http "http://127.0.0.1:$API_PORT/api/v1/health/ready"; then
  tail -50 "$WORK/backend.log" >&2 || true
  die "platform backend did not become ready"
fi
grep -q "OIDC provider initialized" "$WORK/backend.log" || {
  tail -50 "$WORK/backend.log" >&2 || true
  die "platform backend did not initialize OIDC"
}
echo "platform backend ready on :$API_PORT with OIDC enabled"

# ---------- admin session, prelink setup ----------

set -a; source "$ROOT/.env"; set +a
ADMIN_BODY="$(curl -s -m 10 -X POST "http://127.0.0.1:$API_PORT/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$BOOTSTRAP_ADMIN_USERNAME\",\"password\":\"$BOOTSTRAP_ADMIN_PASSWORD\"}")"
ADMIN_TOKEN="$(jq -r '.access_token // empty' <<<"$ADMIN_BODY")"
if [[ -z "$ADMIN_TOKEN" ]]; then
  die "admin login failed: $ADMIN_BODY"
fi

# Reuse the drill user when present (e.g. rerunning after a previous drill);
# otherwise create it.
OIDC_USER_ID="$(docker exec "$PG_CONTAINER" psql -U aiops -d aiops -tAc \
  "SELECT id FROM users WHERE username='oidc-operator'" | tr -d '[:space:]')"
if [[ -z "$OIDC_USER_ID" ]]; then
  CREATE_BODY="$(curl -s -m 10 -X POST "http://127.0.0.1:$API_PORT/api/v1/users" \
    -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
    -d '{"username":"oidc-operator","password":"DrillPass12345!","display_name":"OIDC Operator","roles":["operations_admin"]}')"
  OIDC_USER_ID="$(jq -r '.id // empty' <<<"$CREATE_BODY")"
fi
if [[ -z "$OIDC_USER_ID" ]]; then
  die "create oidc-operator user failed: $CREATE_BODY"
fi
docker exec "$PG_CONTAINER" psql -U aiops -d aiops -tAc \
  "INSERT INTO external_identities (user_id, issuer, subject) VALUES ($OIDC_USER_ID, '$IDP_ISSUER', 'sub-oidc-operator') ON CONFLICT DO NOTHING;" \
  >/dev/null || die "prelink external identity failed"
echo "prelinked oidc-operator (user id $OIDC_USER_ID, subject sub-oidc-operator)"

# ---------- scenarios ----------

scenario "S1 happy-path OIDC login"
oidc_full_login "$WORK/jar-happy" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr"
if [[ "$OIDC_LOGIN_HTTP" == "302" ]] && [[ -n "$OIDC_AUTH_URL" ]] \
   && [[ "$OIDC_CB_HTTP" == "200" ]]; then
  HAPPY_CODE="$OIDC_CODE"
  OIDC_ACCESS_TOKEN="$(jq -r '.access_token // empty' <<<"$OIDC_CB_BODY")"
  ME_BODY="$(curl -s -m 10 -H "Authorization: Bearer $OIDC_ACCESS_TOKEN" \
    "http://127.0.0.1:$API_PORT/api/v1/auth/me")"
  if [[ -n "$OIDC_ACCESS_TOKEN" ]] \
     && [[ "$(jq -r '.username' <<<"$ME_BODY")" == "oidc-operator" ]] \
     && jq -e '.roles | index("operations_admin")' <<<"$ME_BODY" >/dev/null; then
    pass S1 "login 302 -> callback 200 -> /me 200 with operations_admin"
  else
    fail S1 "/me mismatch: $ME_BODY"
  fi
else
  fail S1 "login_http=$OIDC_LOGIN_HTTP callback_http=$OIDC_CB_HTTP body=$OIDC_CB_BODY"
fi
DETAIL_S1="$(detail_of S1)"

scenario "S2 missing subject prelink fails closed"
oidc_full_login "$WORK/jar-noprelink" \
  "user=sub-ghost&username=ghost&display_name=Ghost&groups=platform-operators&acr=phr"
if [[ "$OIDC_CB_HTTP" == "403" ]] && [[ "$(cb_error_code)" == "OIDC_SUBJECT_NOT_PRELINKED" ]]; then
  pass S2 "callback 403 OIDC_SUBJECT_NOT_PRELINKED"
else
  fail S2 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S2="$(detail_of S2)"

scenario "S3 nonce tampering fails closed"
oidc_full_login "$WORK/jar-nonce" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr&fail=wrong_nonce"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S3 "callback 502 OIDC_LOGIN_FAILED (nonce mismatch)"
else
  fail S3 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S3="$(detail_of S3)"

scenario "S4 state mismatch fails closed"
oidc_start_login "$WORK/jar-state"
oidc_authorize "$WORK/jar-state" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr"
oidc_callback "$WORK/jar-state" "tampered-$OIDC_STATE"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S4 "callback 502 OIDC_LOGIN_FAILED (state mismatch)"
else
  fail S4 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S4="$(detail_of S4)"

scenario "S5 missing MFA evidence fails closed"
oidc_full_login "$WORK/jar-nomfa" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&fail=no_mfa"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S5 "callback 502 OIDC_LOGIN_FAILED (MFA evidence missing)"
else
  fail S5 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S5="$(detail_of S5)"

scenario "S6 unaccepted MFA evidence fails closed"
oidc_full_login "$WORK/jar-lowmfa" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=loa1"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S6 "callback 502 OIDC_LOGIN_FAILED (acr=loa1 not accepted)"
else
  fail S6 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S6="$(detail_of S6)"

scenario "S7 unmapped groups fail closed"
oidc_full_login "$WORK/jar-groups" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=outsiders&acr=phr"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S7 "callback 502 OIDC_LOGIN_FAILED (groups map to no role)"
else
  fail S7 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S7="$(detail_of S7)"

scenario "S8 rotated signing key fails closed"
oidc_full_login "$WORK/jar-key" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr&fail=wrong_key"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S8 "callback 502 OIDC_LOGIN_FAILED (unknown kid)"
else
  fail S8 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S8="$(detail_of S8)"

scenario "S9 expired ID token fails closed"
oidc_full_login "$WORK/jar-expired" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr&fail=expired"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S9 "callback 502 OIDC_LOGIN_FAILED (expired)"
else
  fail S9 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S9="$(detail_of S9)"

scenario "S10 unsigned token fails closed"
oidc_full_login "$WORK/jar-unsigned" \
  "user=sub-oidc-operator&username=oidc-operator&display_name=OIDC+Operator&groups=platform-operators&acr=phr&fail=unsigned"
if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
  pass S10 "callback 502 OIDC_LOGIN_FAILED (unsigned token)"
else
  fail S10 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
fi
DETAIL_S10="$(detail_of S10)"

scenario "S11 audit trail records OIDC login success"
AUDIT_BODY="$(curl -s -m 10 -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://127.0.0.1:$API_PORT/api/v1/audit-logs?action=auth.oidc.callback&result=success")"
if jq -e '.items[] | select(.action=="auth.oidc.callback" and .result=="success" and .status_code==200)' <<<"$AUDIT_BODY" >/dev/null; then
  pass S11 "audit log records auth.oidc.callback success (status 200)"
else
  fail S11 "audit log missing expected entry: $AUDIT_BODY"
fi
DETAIL_S11="$(detail_of S11)"

# ---------- provider-down scenarios ----------

scenario "S12 provider down at runtime (token endpoint unreachable)"
kill "$IDP_PID" 2>/dev/null || true
IDP_PID=""
oidc_start_login "$WORK/jar-down"
if [[ "$OIDC_LOGIN_HTTP" != "302" ]]; then
  fail S12 "login_http=$OIDC_LOGIN_HTTP (want 302 from cached discovery)"
else
  OIDC_CODE="$HAPPY_CODE"
  oidc_callback "$WORK/jar-down" "$OIDC_STATE"
  if [[ "$OIDC_CB_HTTP" == "502" ]] && [[ "$(cb_error_code)" == "OIDC_LOGIN_FAILED" ]]; then
    pass S12 "callback 502 OIDC_LOGIN_FAILED (token endpoint unreachable)"
  else
    fail S12 "callback_http=$OIDC_CB_HTTP code=$(cb_error_code) body=$OIDC_CB_BODY"
  fi
fi
DETAIL_S12="$(detail_of S12)"

scenario "S13 provider down at runtime (discovery cache expiry)"
found_unavailable=0
for _ in $(seq 1 24); do
  oidc_start_login "$WORK/jar-unavail"
  if [[ "$OIDC_LOGIN_HTTP" == "502" ]] && grep -q "OIDC_UNAVAILABLE" <<<"$OIDC_LOGIN_BODY"; then
    found_unavailable=1
    break
  fi
  sleep 5
done
if [[ "$found_unavailable" == "1" ]]; then
  pass S13 "login 502 OIDC_UNAVAILABLE after discovery cache expiry"
else
  fail S13 "login_http=$OIDC_LOGIN_HTTP body=$OIDC_LOGIN_BODY (want OIDC_UNAVAILABLE within 120s)"
fi
DETAIL_S13="$(detail_of S13)"

scenario "S14 provider down at startup (server refuses to start)"
(
  set -a
  source "$WORK/backend.env"
  set +a
  export HTTP_ADDR=":$FAIL_PORT"
  export SSL_CERT_FILE="$WORK/idp-cert.pem"
  exec "$WORK/server"
) > "$WORK/backend-fail.log" 2>&1 &
FAIL_PID=$!
for _ in $(seq 1 40); do
  if ! kill -0 "$FAIL_PID" 2>/dev/null; then
    break
  fi
  sleep 0.5
done
rc=0
if kill -0 "$FAIL_PID" 2>/dev/null; then
  kill "$FAIL_PID" 2>/dev/null || true
  FAIL_PID=""
  fail S14 "server stayed alive with provider down"
else
  set +e
  wait "$FAIL_PID" 2>/dev/null
  rc=$?
  set -e
  FAIL_PID=""
  if [[ "$rc" -ne 0 ]] && grep -q "initialize OIDC provider" "$WORK/backend-fail.log"; then
    pass S14 "server exited $rc with 'initialize OIDC provider' fatal"
  else
    fail S14 "exit=$rc log=$(tail -3 "$WORK/backend-fail.log")"
  fi
fi
DETAIL_S14="$(detail_of S14)"

# ---------- report ----------

{
  echo "run_id: $RUN_ID"
  echo "idp_issuer: $IDP_ISSUER"
  echo "platform: http://127.0.0.1:$API_PORT (OIDC enabled)"
  echo "oidc_user: oidc-operator (user_id $OIDC_USER_ID, subject sub-oidc-operator)"
  echo "scenario results:"
  for name in S1 S2 S3 S4 S5 S6 S7 S8 S9 S10 S11 S12 S13 S14; do
    echo "  $name: $(status_of "$name") - $(detail_of "$name")"
  done
} > "$WORK/summary.txt"

cat > "$REPORT" <<JSON
{
  "run_id": "$RUN_ID",
  "idp_issuer": "$IDP_ISSUER",
  "platform": "http://127.0.0.1:$API_PORT",
  "oidc_user": {"username": "oidc-operator", "user_id": $OIDC_USER_ID, "subject": "sub-oidc-operator"},
  "scenarios": {
    "s1_happy_path": {"expected": "302 to provider, PKCE code exchange, 200 with access_token, /me 200 with local operations_admin role", "result": "$(status_of S1)", "observed": "$(detail_of S1)"},
    "s2_missing_prelink": {"expected": "403 OIDC_SUBJECT_NOT_PRELINKED", "result": "$(status_of S2)", "observed": "$(detail_of S2)"},
    "s3_nonce_tampering": {"expected": "502 OIDC_LOGIN_FAILED on nonce mismatch", "result": "$(status_of S3)", "observed": "$(detail_of S3)"},
    "s4_state_mismatch": {"expected": "502 OIDC_LOGIN_FAILED on state mismatch", "result": "$(status_of S4)", "observed": "$(detail_of S4)"},
    "s5_missing_mfa": {"expected": "502 OIDC_LOGIN_FAILED when acr absent", "result": "$(status_of S5)", "observed": "$(detail_of S5)"},
    "s6_unaccepted_mfa": {"expected": "502 OIDC_LOGIN_FAILED when acr not accepted", "result": "$(status_of S6)", "observed": "$(detail_of S6)"},
    "s7_unmapped_groups": {"expected": "502 OIDC_LOGIN_FAILED when groups map to no role", "result": "$(status_of S7)", "observed": "$(detail_of S7)"},
    "s8_rotated_key": {"expected": "502 OIDC_LOGIN_FAILED when kid unknown to JWKS", "result": "$(status_of S8)", "observed": "$(detail_of S8)"},
    "s9_expired_token": {"expected": "502 OIDC_LOGIN_FAILED when exp past", "result": "$(status_of S9)", "observed": "$(detail_of S9)"},
    "s10_unsigned_token": {"expected": "502 OIDC_LOGIN_FAILED on signature verification failure", "result": "$(status_of S10)", "observed": "$(detail_of S10)"},
    "s11_audit_trail": {"expected": "audit log records auth.oidc.callback success for oidc-operator", "result": "$(status_of S11)", "observed": "$(detail_of S11)"},
    "s12_provider_down_token_endpoint": {"expected": "login still 302 from cached discovery; callback 502 OIDC_LOGIN_FAILED", "result": "$(status_of S12)", "observed": "$(detail_of S12)"},
    "s13_provider_down_discovery_expiry": {"expected": "login 502 OIDC_UNAVAILABLE after 1m discovery/JWKS cache TTL", "result": "$(status_of S13)", "observed": "$(detail_of S13)"},
    "s14_provider_down_startup": {"expected": "server exits non-zero with 'initialize OIDC provider' fatal", "result": "$(status_of S14)", "observed": "$(detail_of S14)"}
  },
  "notes": "Local drill only. The IdP is a repository tool (backend/cmd/oidc-provider), never a production provider. Production OIDC/MFA acceptance still requires an organization-approved provider (M89 authorization track)."
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
