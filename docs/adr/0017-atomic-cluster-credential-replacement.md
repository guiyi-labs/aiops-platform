# ADR 0017: Atomic Cluster Credential Replacement

- Status: Accepted
- Date: 2026-07-17

## Context

Kubernetes service-account tokens and client certificates expire or are deliberately rotated. Deleting and recreating a registered cluster would lose its stable platform identity and related history; overwriting only the credential row could leave the API Server metadata, cached client and probe state inconsistent.

## Decision

Add a system-admin-only replacement operation that accepts a kubeconfig but never returns it. The service applies the same strict parser used for onboarding, encrypts with a fresh AES-GCM nonce and current key version, then invokes one repository transaction. The transaction locks the cluster, replaces encrypted credential and API Server, clears stale Kubernetes version and probe time, derives `disabled` or `unknown` status from the existing enabled flag, and writes all three Conditions as `Unknown / CredentialsUpdated`.

Only after commit does the service invalidate the cached Kubernetes client. Replacement does not automatically contact the target API; an explicit probe is required. The unified audit records `cluster.credentials.rotate` with cluster identity and result but never receives the request body.

## Consequences

- Cluster ID and diagnosis/audit relationships remain stable across credential rotation.
- Parse, encryption or transaction failure preserves the previous usable credential and cache entry.
- Successful replacement intentionally removes stale readiness claims until a new probe runs.
- Operators must explicitly probe and then investigate an unreachable result; replacement itself proves only syntactic validity and secure storage.
- Bulk re-encryption when the application master key changes remains a separate migration/operations feature.
