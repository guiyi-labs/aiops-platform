# M23 Safe Deployment Release Lifecycle E2E Fixtures

These resources validate the M23 release lifecycle contract in a disposable
kind cluster. They are intentionally separate from `deploy/demo-scenarios`
and must not be added to the retained defense demo.

- `release-target` is a two-replica Deployment pinned to
  `registry.k8s.io/pause:3.10` with `revisionHistoryLimit: 10` so that
  subsequent image updates and rollbacks produce a deterministic
  ReplicaSet revision graph the platform can inspect.
- The Deployment lives in the `aiops-m23-e2e` Namespace, which is created
  by `namespace.yaml` and torn down with the kind cluster by the
  disposable E2E script.

Run the complete image-update, rollback, restoration, RBAC and audit
acceptance workflow with:

```powershell
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m23-release-lifecycle-kind.ps1
```

The script uses the bundled kind binary, keeps the short-lived kubeconfig
and ServiceAccount token out of output, deletes the platform cluster
record, deletes the kind cluster and removes the temporary kubeconfig in
`finally`. Sanitized evidence is written below the ignored
`.artifacts/m23-release-lifecycle-kind` directory.
