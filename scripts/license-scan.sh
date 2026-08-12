#!/usr/bin/env bash
# M100-D: dependency license allowlist gate.
#
# Go: enumerates every module reachable from the backend binary
# (`go list -m -json all`), reads the LICENSE/COPYING file from the module
# cache, classifies it with the same heuristics as
# scripts/generate-license-report.ps1 and fails on UNKNOWN or non-allowlisted
# licenses.
# Frontend: `pnpm licenses list --prod --json` against the same allowlist.
#
# Exit 0 when every production dependency has an allowlisted license.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWLIST="MIT Apache-2.0 BSD-2-Clause BSD-3-Clause ISC MPL-2.0"

classify_license() {
  local text="$1"
  [[ -z "$text" ]] && { echo UNKNOWN; return; }
  if grep -q "Apache License" <<<"$text" && grep -q "Version 2.0" <<<"$text"; then echo Apache-2.0; return; fi
  if grep -q "Mozilla Public License" <<<"$text"; then echo MPL-2.0; return; fi
  if grep -q "GNU LESSER GENERAL PUBLIC LICENSE" <<<"$text"; then echo LGPL; return; fi
  if grep -q "GNU GENERAL PUBLIC LICENSE" <<<"$text"; then echo GPL; return; fi
  if grep -q "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee" <<<"$text"; then echo ISC; return; fi
  if grep -q "MIT License" <<<"$text" || grep -q "Permission is hereby granted, free of charge" <<<"$text"; then echo MIT; return; fi
  if grep -q "Redistributions of source code must retain" <<<"$text"; then
    if grep -q "Neither the name" <<<"$text"; then echo BSD-3-Clause; else echo BSD-2-Clause; fi
    return
  fi
  echo UNKNOWN
}

fail=0

echo "== Go module licenses =="
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  mod_path="$(echo "$line" | cut -f1)"
  mod_ver="$(echo "$line" | cut -f2)"
  mod_dir="$(echo "$line" | cut -f3)"
  [[ -z "$mod_path" || -z "$mod_dir" ]] && continue
  # Use the module's own license at the module root only; deeper third-party
  # license files (e.g. sonic's licenses/) must not be mistaken for the module
  # license, and matching the licenses/ directory yields UNKNOWN.
  license_file=""
  license_file="$(find "$mod_dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) 2>/dev/null | head -1)"
  if [[ -z "$license_file" ]]; then
    license=UNKNOWN
  else
    license="$(classify_license "$(tr '\n' ' ' < "$license_file")")"
  fi
  ok=0
  for allowed in $ALLOWLIST; do
    [[ "$license" == "$allowed" ]] && ok=1
  done
  if [[ "$ok" -eq 0 ]]; then
    echo "  LICENSE CHECK FAIL: $mod_path@$mod_ver -> $license"
    fail=1
  fi
  done < <(cd "$ROOT/backend" && go list -m -json all 2>/dev/null | jq -r 'select(.Main != true) | [.Path, (.Version // ""), (.Dir // "")] | @tsv')

echo
echo "== Frontend production licenses =="
if command -v pnpm >/dev/null 2>&1; then
  (cd "$ROOT/frontend" && pnpm licenses list --prod --json 2>/dev/null | ALLOWLIST="$ALLOWLIST" node "$ROOT/scripts/license-scan-parser.mjs") || fail=1
else
  echo "  pnpm not found" >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  echo "license scan: FAILED — review the dependency or extend the allowlist deliberately" >&2
  exit 1
fi
echo "license scan: clean"
