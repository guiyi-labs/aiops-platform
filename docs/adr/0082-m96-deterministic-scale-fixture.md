# ADR 0082 - M96 deterministic scale fixture and streaming artifact format

- Date: 2026-08-10
- Status: Accepted
- Milestone: M96
- Related: M40 topology, M49 read-only Kubernetes gateway, M50 metrics history, M91 virtual scroll

## Context

M96 needs repeatable evidence at 500 Nodes, 50,000 Pods and 100,000 Events.
The existing tests construct small in-memory values and do not prove that the
same fleet shape can be regenerated for topology, workload, global-search and
metrics-history measurements. Committing a large generated dataset would make
the repository noisy and would not prove that a clean runner can reproduce it.

## Decision

1. Store only a versioned JSON configuration in `backend/testdata/scale/`.
   The canonical configuration is `m96-v1`, uses a fixed seed and observation
   time, and requires two Events per Pod.
2. Generate NDJSON records through the existing bounded Kubernetes projections:
   Nodes, workload projections (Deployment/ReplicaSet/Service/Ingress), Pods,
   Events and metrics-history samples. Each Pod carries owner, node and search
   mappings; history samples carry the exact resource UID and series shape.
3. Write each record stream as deterministic gzip. The manifest stores counts,
   uncompressed and compressed byte counts, per-stream SHA-256, a config hash,
   coverage mappings and an aggregate dataset hash. A verifier reads every
   record without materializing the stream and rejects extra files, malformed
   JSON, changed counts, bytes or hashes.
4. Expose generation and verification through `backend/cmd/scale-fixture`.
   CI runs the canonical generator in report mode and uploads only the
   manifest; generated data stays in ignored artifact storage.

## Consequences

- Benchmark adapters can consume one stable fixture contract without coupling
  the fixture to a database or a live cluster.
- The output is intentionally an evidence source, not a claim about production
  capacity. API, timeout, cancellation and frontend budgets still require
  measured benchmark reports in later M96 increments.
- Changing the record schema, seed, counts or mapping requires a new dataset
  version and a new baseline artifact.
