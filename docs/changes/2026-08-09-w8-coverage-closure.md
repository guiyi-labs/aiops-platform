# W8: Coverage Closure — 60% Gate Achieved + CI Gate Raised to 60%

- Date: 2026-08-09
- Status: Development Complete
- Scope: Test + CI gate — no product code change

## Summary

Closes the W8 remaining incremental target from polish-plan.md: global
coverage lifted from 59.1% → 60.03% (22317 statements, 13396 covered) and
the CI global coverage gate in `.github/workflows/ci.yml` is raised from
50% to 60%.

High-yield pure-logic helpers were targeted across 8 low-coverage packages:

| Package | File | Key Functions Covered |
|---|---|---|
| automation | helpers_test.go | unmarshalPolicyGates, unmarshalEvidenceSnapshot, bytesEqual, resource{Int,Bool,Str}, replicasInt, newIdentity, newCorrelationRequestID, safeExecutionError |
| automation | patch_test.go | buildChange, buildPatch (scale/restart/suspend/resume/image_update), buildRollbackPatch (full branch matrix), WithPlanTTL/ClaimTTL/Cooldown, Enabled defaults |
| automation | gate_context_test.go | buildGateContext Deployment/CronJob/read-error/rollback-point branches |
| correlation | logic_test.go | classifyConfidence, countRequiredFactors, sort/merge helpers, JSONB marshal/unmarshal (fixed EvidenceRef.RefID → ID bug) |
| auth | helpers_test.go | normalizeRoles, isAssignable, sessionFrom, hasRole, 3× TableName |
| authz | model_test.go | 2× TableName (ClusterGrant, NamespaceGrant) |
| alert | table_test.go | 2× TableName (Rule, Instance) |
| alertroute | model_test.go | 5× TableName (Receiver, Route, Silence, Inhibit, Delivery) |
| workspace | model_test.go | IsValidAuditAction + 5× TableName |
| cluster | table_test.go | 3× TableName + APIStatusError.Error + newCredentialReencryptionID |
| insight | snapshot_test.go | Kinds + Operations (sorted non-empty) |

## Verification

```
COVERED=13396 TOTAL=22317 PCT=60.026%
```

CI gate now enforces ≥60.0% baseline in `.github/workflows/ci.yml` awk block.