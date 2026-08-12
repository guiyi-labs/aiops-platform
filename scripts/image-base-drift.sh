#!/usr/bin/env bash
# M100-D: image base layer drift gate.
#
# Re-resolves the manifest digest of every base image pinned in
# docs/security/image-base-manifest.md and fails when the upstream digest no
# longer matches the committed manifest. This turns silent base-image drift
# (mutated tags, upstream re-tags) into a reviewed, explicit update.
#
# Resolution strategy: use the local docker daemon when the image is present,
# otherwise query the registry directly (Docker Hub v2 API). Multi-arch
# manifests use the manifest-list digest (same digest the Dockerfile resolves).
#
# Exit 0 when every digest matches the manifest; exit 1 listing drift.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$ROOT/docs/security/image-base-manifest.md"

if [[ ! -f "$MANIFEST" ]]; then
  echo "image-base-manifest.md missing" >&2
  exit 1
fi

resolve_digest() {
  local image="$1"
  local repo="${image%%:*}"
  local tag="${image#*:}"
  local digest=""
  if docker image inspect "$image" >/dev/null 2>&1; then
    digest="$(docker image inspect "$image" --format '{{range .RepoDigests}}{{.}}{{end}}' 2>/dev/null)"
    digest="${digest##*@}"
  fi
  if [[ -z "$digest" ]]; then
    local token
    token="$(curl -sf --max-time 10 "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/${repo}:pull" | sed -E 's/.*"token":"([^"]+)".*/\1/')"
    digest="$(curl -sf -D - -o /dev/null --max-time 15 \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json" \
      "https://registry-1.docker.io/v2/library/${repo}/manifests/${tag}" \
      | grep -i '^docker-content-digest:' | tr -d '\r' | awk '{print $2}')"
  fi
  printf '%s' "$digest"
}

drift=0
# entries are: image|digest
while IFS='|' read -r image expected; do
  image="$(echo "$image" | xargs)"
  expected="$(echo "$expected" | xargs)"
  [[ -z "$image" || -z "$expected" ]] && continue
  actual="$(resolve_digest "$image" || true)"
  if [[ "$actual" != "$expected" ]]; then
    echo "BASE IMAGE DRIFT: $image expected $expected got ${actual:-unresolvable}" >&2
    drift=1
  else
    echo "ok: $image $actual"
  fi
done < <(grep -E '^\| `[^`]+` \|' "$MANIFEST" | sed -E 's/^\| `([^`]+)` \|[^|]*\| `([^`]+)` \|$/\1|\2/')

if [[ "$drift" -ne 0 ]]; then
  echo "image base drift gate failed — update docs/security/image-base-manifest.md deliberately" >&2
  exit 1
fi
echo "image base drift gate: clean"
