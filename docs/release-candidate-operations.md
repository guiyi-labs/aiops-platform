# Release Candidate Install, Upgrade And Rollback

This runbook applies only to `vX.Y.Z-rc.N` packages. The release manifest
deliberately records `ga=false` and `productionReady=false` while the M89
production identity and M90 WAL/PITR/HA tracks remain unverified.

## Verify Before Use

Verify the checksum root before opening any archive, then verify its Cosign
signature against the exact tag workflow identity recorded in
`release-manifest.json`:

```bash
sha256sum -c SHA256SUMS
cosign verify-blob --bundle SHA256SUMS.bundle \
  --certificate-identity "https://github.com/guiyi-labs/aiops-platform/.github/workflows/release.yml@refs/tags/vX.Y.Z-rc.N" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  SHA256SUMS
```

For a local key-signed rehearsal, use the command recorded in its manifest.
That proves package integrity mechanics but does not establish the hosted
workflow identity.

## Required Inputs

- A Kubernetes cluster, `kubectl`, and either Helm 3 or Kustomize support.
- An operator-managed `aiops-secrets` Secret. Never commit or place its real
  values in release evidence.
- A registry reachable by every target node. Mirror both OCI archives under
  the image names and version recorded in the release manifest.
- A validated logical backup before every upgrade. Production PITR and HA are
  outside this RC boundary.

Example image mirroring with Skopeo:

```bash
skopeo copy oci-archive:aiops-platform-backend-vX.Y.Z-rc.N-linux-multiarch-oci.tar docker://REGISTRY/k8s-aiops-backend:vX.Y.Z-rc.N
skopeo copy oci-archive:aiops-platform-frontend-vX.Y.Z-rc.N-linux-multiarch-oci.tar docker://REGISTRY/k8s-aiops-frontend:vX.Y.Z-rc.N
```

## Helm Install And Upgrade

Create the target Namespace and Secret before installing. Supply immutable
registry references approved by the operator:

```bash
kubectl create namespace aiops-system
kubectl apply -f /secure/path/aiops-secret.yaml
helm install aiops ./aiops-platform-X.Y.Z-rc.N.tgz \
  --namespace aiops-system \
  --set backend.image.repository=REGISTRY/k8s-aiops-backend \
  --set backend.image.tag=vX.Y.Z-rc.N \
  --set frontend.image.repository=REGISTRY/k8s-aiops-frontend \
  --set frontend.image.tag=vX.Y.Z-rc.N \
  --wait --timeout 10m
```

For an upgrade, take and validate the database backup first, then run
`helm upgrade` with the same explicit image settings. Record the prior Helm
revision before changing anything:

```bash
helm history aiops -n aiops-system
helm upgrade aiops ./aiops-platform-X.Y.Z-rc.N.tgz -n aiops-system \
  --reuse-values \
  --set backend.image.tag=vX.Y.Z-rc.N \
  --set frontend.image.tag=vX.Y.Z-rc.N \
  --wait --timeout 10m
```

Rollback application resources with `helm rollback aiops PREVIOUS_REVISION`.
The backend applies forward database migrations at startup. Do not assume a
Helm rollback reverses schema or data changes. If the previous backend is not
compatible with the migrated schema, stop writers and restore the validated
pre-upgrade logical backup into an isolated database before cutover.

## Kustomize Install And Upgrade

Extract the versioned Kustomize archive into its own directory. Set the exact
mirrored image references in that copy, review the rendered result, then apply:

```bash
cd kubernetes
kustomize edit set image k8s-aiops-backend=REGISTRY/k8s-aiops-backend:vX.Y.Z-rc.N
kustomize edit set image k8s-aiops-frontend=REGISTRY/k8s-aiops-frontend:vX.Y.Z-rc.N
kubectl apply -f /secure/path/aiops-secret.yaml
kubectl kustomize . > /tmp/aiops-rendered.yaml
kubectl diff -f /tmp/aiops-rendered.yaml
kubectl apply -f /tmp/aiops-rendered.yaml
```

Upgrade by applying the newly rendered version after the backup check. Roll
back by reapplying the complete previous RC rendering. The same database
rollback boundary described for Helm applies.

## Offline Package

Verify the outer release first. Extract the offline archive, then verify its
internal payload before mirroring images or applying manifests:

```bash
tar -xzf aiops-platform-offline-vX.Y.Z-rc.N.tar.gz
cd aiops-platform-offline-vX.Y.Z-rc.N
sha256sum -c OFFLINE-SHA256SUMS
```

The archive contains OCI image archives, Helm and Kustomize packages, SPDX
SBOMs, the Secret template, this runbook, and the internal checksum list. Move
it through the approved air-gap transfer process, import the images into the
target registry, and then use either installation path above.

## Health, Authentication And Cleanup

Both installation paths must pass the same checks:

```bash
kubectl rollout status statefulset/postgres -n aiops-system --timeout=10m
kubectl rollout status deployment/backend -n aiops-system --timeout=10m
kubectl rollout status deployment/frontend -n aiops-system --timeout=10m
kubectl port-forward -n aiops-system service/frontend 18080:80
curl --fail http://127.0.0.1:18080/api/v1/health/ready
```

Perform one authenticated login using an ephemeral test credential and retain
only status, revision and digest evidence. Never retain the password, token,
Cookie, Secret, kubeconfig or response body in the release artifact.

Helm cleanup is `helm uninstall aiops -n aiops-system`. Kustomize cleanup is
`kubectl delete -k kubernetes`. Delete the Namespace only after confirming that
its PostgreSQL data and operator-managed Secrets are no longer required.
