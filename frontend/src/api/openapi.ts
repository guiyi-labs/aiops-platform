// Generated OpenAPI contract facade (W10).
//
// The singular source of truth for API shapes is docs/api/openapi.yaml. This
// module re-exports the openapi-typescript generated types so leaf modules can
// consume the JSON payload types and operation parameter types directly:
//
//   - `paths` / `components` / `operations` — full generated namespace
//   - `fromRunbook(op)` / `runbookResponse(op)` — typed helpers that narrow a
//     matching OpenAPI operation to its canonical JSON response type.
//
// Never hand-edit these shapes; regenerate via `pnpm typegen` and let the CI
// typegen-sync gate (W10) catch drift.
import type { components, operations, paths } from './openapi.d'

/** Typed accessor: the JSON payload of the 200 response of `op`. */
export type OperationResponse<
  Op extends keyof operations,
  Status extends number = 200,
> = operations[Op]['responses'][Status & keyof operations[Op]['responses']] extends {
  content: { 'application/json': infer Payload }
}
  ? Payload
  : never

/** Raw generated contract namespaces. */
export type GeneratedPaths = paths
export type GeneratedComponents = components
export type GeneratedOperations = operations

/** Insight runbook response payload resolved from the generated contract. */
export type InsightRunbookContract = OperationResponse<'getInsightRunbook'>
export type InsightRunbookQuery = operations['getInsightRunbook']['parameters']['query']