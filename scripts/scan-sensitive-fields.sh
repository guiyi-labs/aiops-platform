#!/usr/bin/env bash
# M100-C: sensitive-field static scan gate.
#
# Scans every git-tracked file for secret material that must never reach the
# repository: private keys, inline kubeconfig credential data, cloud/API
# tokens, JWTs, committed credential files, and non-placeholder password
# assignments. False positives are suppressed via
# scripts/scan-sensitive-fields.allowlist (extended regexes matched against
# "path:lineno"; "#" comments allowed; a bare "path" entry matches the whole
# file).
#
# Exit 0 when clean; exit 1 listing every finding. Run from anywhere in the
# repo; CI invokes it as part of the gate suite.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ALLOWLIST="$ROOT/scripts/scan-sensitive-fields.allowlist"
[[ -f "$ALLOWLIST" ]] || ALLOWLIST=/dev/null

read_allowlist() {
  sed 's/[[:space:]]*#.*$//' "$ALLOWLIST" 2>/dev/null | grep -vE '^[[:space:]]*$' || true
}

is_allowed() {
  local location="$1"
  local path="${location%%:*}"
  while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue
    if [[ "$entry" == *:* ]]; then
      [[ "$location" == "$entry" ]] && return 0
    else
      [[ "$path" == "$entry" ]] && return 0
    fi
  done < <(read_allowlist)
  return 1
}

FINDINGS=()
FILES=0

report() {
  local location="$1" reason="$2"
  if is_allowed "$location"; then
    return
  fi
  FINDINGS+=("$location: $reason")
}

scan_file() {
  local file="$1"
  local line
  while IFS= read -r line; do
    local path="${line%%:*}"
    local rest="${line#*:}"
    local lineno="${rest%%:*}"
    local content="${rest#*:}"
    if [[ "$path" != "$file" ]]; then
      continue
    fi
    case "$content" in
      *"BEGIN "*"PRIVATE KEY"*)
        report "$file:$lineno" "private key material"
        ;;
    esac
    if [[ "$content" =~ (client-certificate-data|client-key-data|certificate-authority-data):[[:space:]]*[A-Za-z0-9+/=]{20,} ]]; then
      report "$file:$lineno" "inline kubeconfig credential data"
    fi
    if [[ "$content" =~ (AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|xox[baprs]-[A-Za-z0-9-]{10,}|sk-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{30,}) ]]; then
      report "$file:$lineno" "cloud/API token"
    fi
    if [[ "$content" =~ eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,} ]]; then
      report "$file:$lineno" "JWT"
    fi
    if [[ "$content" =~ ^(export[[:space:]]+)?[A-Z0-9_]*PASSWORD=([^[:space:]]*)$ ]]; then
      local value="${BASH_REMATCH[2]}"
      case "$value" in
        "" | change_me | change_me_now | change-me | example | your-* | \*\*\*)
          ;;
        \$* | \<* | \"* | \'*)
          ;;
        *)
          report "$file:$lineno" "non-placeholder password assignment"
          ;;
      esac
    fi
  done < <(grep -HnE 'BEGIN .*PRIVATE KEY|client-(certificate|key)-data:|certificate-authority-data:|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|xox[baprs]-[A-Za-z0-9-]{10,}|sk-[A-Za-z0-9-]{20,}|AIza[0-9A-Za-z_-]{30,}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}|^[[:space:]]*(export[[:space:]]+)?[A-Z0-9_]*PASSWORD=' "$file" || true)
}

while IFS= read -r file; do
  FILES=$((FILES + 1))
  scan_file "$file"
done < <(git ls-files)

# Tracked credential/key files.
while IFS= read -r file; do
  report "$file" "tracked credential/key file"
done < <(git ls-files | grep -E '\.(pem|key|p12|pfx|jks|keystore)$' || true)

if [[ ${#FINDINGS[@]} -gt 0 ]]; then
  printf 'sensitive-field scan: %d finding(s)\n' "${#FINDINGS[@]}" >&2
  printf '%s\n' "${FINDINGS[@]}" >&2
  exit 1
fi
printf 'sensitive-field scan: clean (%d tracked files)\n' "$FILES"
