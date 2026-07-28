# M9 Isolated Diagnosis E2E Fixtures

These resources validate the M8 Node and Deployment diagnosis rules in a
disposable kind cluster. They are intentionally separate from
`deploy/demo-scenarios` and must not be added to the retained defense demo.

- `stalled-deployment` requests two replicas with a node selector that no Node
  has, producing zero Ready and Available replicas without relying on an image
  registry failure.
- `synthetic-not-ready` is unschedulable and receives its Ready=False Condition
  through the status subresource in `scripts/e2e-diagnosis-kind.ps1`.
- `kind.yaml` has no fixed cluster name so every run can use an isolated,
  timestamped cluster.

Run the complete create, diagnose, evidence-check and cleanup workflow with:

```powershell
$env:AIOPS_ADMIN_PASSWORD = '<local-development-password>'
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-diagnosis-kind.ps1
```

The script uses the bundled kind binary, keeps the short-lived kubeconfig and
ServiceAccount token out of output, deletes the platform cluster record, deletes
the kind cluster and removes the temporary kubeconfig in `finally`. Sanitized
evidence is written below ignored `.artifacts/diagnosis-e2e`.
