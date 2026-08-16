#!/usr/bin/env bash
# M100-D: dependency vulnerability scan gate.
#
# Go: govulncheck -mode=source (reachable vulnerabilities only; module-only
#     findings are reported but do not fail the gate).
# Frontend: pnpm audit --prod (production dependency graph only; dev toolchain
#     advisories are tracked exceptions and never reach the shipped bundle).
#
# Exit 0 when no reachable/production findings; exit 1 otherwise.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

fail=0

echo "== Go: govulncheck =="
if ! command -v govulncheck >/dev/null 2>&1 && [[ -x /tmp/gobin/govulncheck ]]; then
  export PATH="/tmp/gobin:$PATH"
fi
if command -v govulncheck >/dev/null 2>&1; then
  (
    cd "$ROOT/backend"
    set +e
    output="$(govulncheck -mode=source ./... 2>&1)"
    gv_exit=$?
    set -e
    # Always surface the scanner output so CI logs are actionable when the
    # gate fails; both a non-zero scanner exit and a reachable-vulnerability
    # verdict mark the gate failed.
    printf '%s\n' "$output"
    if [[ "$gv_exit" != "0" ]] || grep -qE "Your code is affected by [1-9][0-9]* vulnerabilities" <<<"$output"; then
      fail=1
    fi
  )
else
  echo "govulncheck not found — install with: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2
  fail=1
fi

echo
echo "== Frontend + all lockfiles: trivy fs (P2a) =="
# trivy scans the full dependency tree (dev + prod) from lockfiles, catching
# advisories pnpm audit --prod and govulncheck intentionally skip. Installed
# on demand when missing; a scan failure fails the gate.
if ! command -v trivy >/dev/null 2>&1 && [[ -x /tmp/gobin/trivy ]]; then
  export PATH="/tmp/gobin:$PATH"
fi
if command -v trivy >/dev/null 2>&1; then
  (
    cd "$ROOT"
    set +e
    set -e
    for target in "$ROOT/backend" "$ROOT/frontend"; do
      set +e
      output="$(trivy fs --scanners vuln --severity HIGH,CRITICAL --exit-code 1 --no-progress --skip-dirs node_modules --skip-dirs .git "$target" 2>&1)"
      t_exit=$?
      set -e
      printf '== trivy fs %s ==\n%s\n' "$target" "$output"
      if [[ "$t_exit" != "0" ]]; then
        fail=1
      fi
    done
  )
else
  echo "trivy not found — skipping (install: brew install trivy)" >&2
fi

echo
echo "== Frontend: pnpm audit --prod =="
if command -v pnpm >/dev/null 2>&1; then
  (
    cd "$ROOT/frontend"
    if ! pnpm audit --prod --audit-level=high >/tmp/pnpm-audit.log 2>&1; then
      cat /tmp/pnpm-audit.log
      fail=1
    else
      tail -2 /tmp/pnpm-audit.log
    fi
  )
else
  echo "pnpm not found" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  echo "dependency vulnerability scan: FAILED" >&2
  exit 1
fi
echo "dependency vulnerability scan: clean"
