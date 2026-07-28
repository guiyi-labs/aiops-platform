# Metrics Server kind Fixture

This optional fixture validates the platform's real metrics available path. It
is not part of the default platform or managed-cluster deployment.

- Upstream: Kubernetes SIGs Metrics Server
- Version: v0.8.0
- Source: `https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.0/components.yaml`
- SHA-256: `ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b`
- License: Apache License 2.0 (`https://github.com/kubernetes-sigs/metrics-server/blob/v0.8.0/LICENSE`)

`components-v0.8.0.yaml` is the byte-equivalent pinned upstream manifest. The
E2E script validates its checksum before applying it and adds
`--kubelet-insecure-tls` to the Deployment at runtime because the local kind
kubelet uses a development certificate. The fixed JSON Patch is stored in
`kind-patch.json` to avoid shell-specific inline JSON escaping. That flag is specific to this local
fixture and must not be copied to production deployment guidance.

The same patch replaces the runtime image with
`registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.0`.
The local network cannot reach the `registry.k8s.io` Artifact Registry redirect,
while this pinned Google Containers mirror is reachable. The upstream manifest
and its checksum remain unchanged; the evidence records the runtime image
separately.

Run through `scripts/e2e-metrics-kind.ps1`; do not include this directory in the
default Kustomize bases.
