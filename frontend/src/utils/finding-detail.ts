import type { DiagnosisEvidence, DiagnosisRecord, DiagnosisTimelineEntry } from '../types/diagnosis'
import type { FindingDetailV2, FindingEvidenceKind, FindingEvidenceRef, FindingRecommendation, FindingResource } from '../types/finding'
import type { InspectionResultView } from '../types/inspection'
import type { FinOpsRecommendation, OptimizationFinding } from '../types/optimization'

type LegacyFinding = OptimizationFinding & { domain?: string }

interface FindingContext {
  framework?: string
  source?: string
}

export function normalizeFindingSeverity(severity: string): 'info' | 'warning' | 'critical' {
  switch (severity.toLowerCase()) {
    case 'critical':
    case 'high':
      return 'critical'
    case 'warning':
    case 'medium':
      return 'warning'
    default:
      return 'info'
  }
}

function evidenceKind(value: string): FindingEvidenceKind {
  const allowed: FindingEvidenceKind[] = ['resource_state', 'event', 'log', 'alert', 'change', 'automation']
  return allowed.includes(value as FindingEvidenceKind) ? value as FindingEvidenceKind : 'resource_state'
}

function resourceFrom(value: { kind: string; namespace?: string; name: string; uid?: string; resource_version?: string }): FindingResource {
  return {
    kind: value.kind,
    namespace: value.namespace,
    name: value.name,
    uid: value.uid,
    resource_version: value.resource_version,
  }
}

function recommendationFromDetails(details: Record<string, string> | undefined): FindingRecommendation[] {
  const text = details?.remediation?.trim()
  return text ? [{ kind: 'advisory', text }] : []
}

function legacyEvidence(finding: LegacyFinding): FindingEvidenceRef[] {
  const resource = `${finding.resource.kind}/${finding.resource.namespace ? `${finding.resource.namespace}/` : ''}${finding.resource.name}`
  return [{
    id: `resource:${resource}@${finding.observed_at}`,
    kind: 'resource_state',
    summary: finding.summary,
    observed_at: finding.observed_at,
    source: resource,
  }]
}

export function fromOptimizationFinding(finding: LegacyFinding, context: FindingContext = {}): FindingDetailV2 {
  return {
    schema_version: '2',
    rule: { rule_id: finding.code, framework: context.framework, source: context.source },
    code: finding.code,
    severity: normalizeFindingSeverity(finding.severity),
    summary: finding.summary,
    resource: resourceFrom(finding.resource),
    details: finding.details,
    observed_at: finding.observed_at,
    evidence: legacyEvidence(finding),
    recommendations: recommendationFromDetails(finding.details),
    origin_ids: [finding.code],
  }
}

export function fromFinOpsRecommendation(recommendation: FinOpsRecommendation, observedAt?: string): FindingDetailV2 {
  const resource = resourceFrom({
    kind: recommendation.workload_kind,
    namespace: recommendation.namespace,
    name: recommendation.workload_name,
  })
  const observedAtValue = observedAt || new Date().toISOString()
  return {
    schema_version: '2',
    rule: { rule_id: recommendation.code, framework: 'finops', source: 'finops' },
    code: recommendation.code,
    severity: normalizeFindingSeverity(recommendation.severity),
    summary: recommendation.rationale,
    resource,
    details: { container: recommendation.container_name, replicas: String(recommendation.replicas) },
    observed_at: observedAtValue,
    evidence: [{
      id: `resource:${recommendation.workload_kind}/${recommendation.namespace}/${recommendation.workload_name}`,
      kind: 'resource_state',
      summary: recommendation.rationale,
      observed_at: observedAtValue,
      source: `${recommendation.workload_kind}/${recommendation.namespace}/${recommendation.workload_name}`,
    }],
    recommendations: [{ kind: 'advisory', text: recommendation.rationale }],
    origin_ids: [recommendation.code],
  }
}

function diagnosisEvidence(item: DiagnosisEvidence | DiagnosisTimelineEntry): FindingEvidenceRef {
  if ('category' in item) {
    return {
      id: item.ref,
      kind: evidenceKind(item.category),
      summary: item.summary || item.type,
      observed_at: item.occurred_at,
      missing: item.missing,
      missing_reason: item.missing_reason,
      source: item.source,
    }
  }
  return {
    id: `${item.type}:${item.source}`,
    kind: evidenceKind(item.type),
    summary: item.type,
    source: item.source,
  }
}

export function fromDiagnosis(record: DiagnosisRecord): FindingDetailV2 {
  const evidence = record.timeline?.length ? record.timeline.map(diagnosisEvidence) : record.evidence.map(diagnosisEvidence)
  const recommendations: FindingRecommendation[] = record.recommendations.map((text) => ({ kind: 'advisory', text }))
  for (const action of record.actions ?? []) {
    recommendations.push({
      kind: action.kind === 'controlled_action' ? 'controlled_action_available' : 'advisory',
      text: action.title,
      capability: action.action,
    })
  }
  return {
    schema_version: '2',
    rule: { rule_id: record.rule_id, framework: 'diagnosis', source: 'diagnosis' },
    code: record.rule_id,
    severity: normalizeFindingSeverity(record.severity),
    summary: record.summary,
    resource: resourceFrom(record.resource),
    observed_at: record.observed_at,
    evidence,
    recommendations,
    origin_ids: [record.rule_id],
  }
}

export function fromInspectionResult(result: InspectionResultView): FindingDetailV2 {
  const resource = resourceFrom({
    kind: result.resource_kind || 'Unknown',
    namespace: result.namespace,
    name: result.resource_name || 'Unknown',
    uid: result.resource_uid,
  })
  const evidence: FindingEvidenceRef[] = Object.entries(result.evidence ?? {}).map(([key, value]) => ({
    id: `${result.fingerprint}:${key}`,
    kind: key === 'event' ? 'event' : 'resource_state',
    summary: typeof value === 'string' ? value : `${key} evidence`,
    observed_at: result.observed_at,
    source: result.fingerprint,
  }))
  if (!evidence.length) {
    evidence.push({ id: result.fingerprint, kind: 'resource_state', observed_at: result.observed_at, source: result.fingerprint })
  }
  return {
    schema_version: '2',
    rule: { rule_id: result.rule_code, framework: 'inspection', source: result.signal_code },
    code: result.rule_code,
    severity: normalizeFindingSeverity(result.severity),
    summary: `${result.signal_code} · ${result.state}`,
    resource,
    observed_at: result.observed_at,
    evidence,
    recommendations: [],
    origin_ids: [result.rule_code, result.fingerprint],
  }
}

export function mergeFindingDetails(findings: FindingDetailV2[]): FindingDetailV2[] {
  const groups = new Map<string, FindingDetailV2>()
  for (const finding of findings) {
    const key = [
      finding.resource.kind,
      finding.resource.namespace ?? '',
      finding.resource.name,
      finding.resource.uid ?? '',
      finding.severity,
    ].join('\u0000')
    const existing = groups.get(key)
    if (!existing) {
      groups.set(key, {
        ...finding,
        evidence: [...finding.evidence],
        recommendations: [...finding.recommendations],
        origin_ids: [...finding.origin_ids],
      })
      continue
    }
    const originIDs = new Set([...existing.origin_ids, ...finding.origin_ids, finding.rule.rule_id])
    const evidenceIDs = new Set(existing.evidence.map((item) => item.id))
    const recommendationIDs = new Set(existing.recommendations.map((item) => `${item.kind}\u0000${item.text}\u0000${item.capability ?? ''}`))
    existing.origin_ids = [...originIDs].filter(Boolean)
    existing.evidence.push(...finding.evidence.filter((item) => !evidenceIDs.has(item.id)))
    existing.recommendations.push(...finding.recommendations.filter((item) => !recommendationIDs.has(`${item.kind}\u0000${item.text}\u0000${item.capability ?? ''}`)))
  }
  return [...groups.values()]
}
