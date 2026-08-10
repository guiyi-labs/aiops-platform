import { describe, expect, it } from 'vitest'
import { fromDiagnosis, fromInspectionResult, fromOptimizationFinding, mergeFindingDetails, normalizeFindingSeverity } from './finding-detail'
import type { DiagnosisRecord } from '../types/diagnosis'
import type { InspectionResultView } from '../types/inspection'

describe('finding-detail compatibility adapters', () => {
  it('normalizes analyzer and diagnosis severity vocabularies', () => {
    expect(normalizeFindingSeverity('high')).toBe('critical')
    expect(normalizeFindingSeverity('medium')).toBe('warning')
    expect(normalizeFindingSeverity('low')).toBe('info')
    expect(normalizeFindingSeverity('unknown')).toBe('info')
  })

  it('promotes a legacy analyzer finding without losing the v1 fields', () => {
    const detail = fromOptimizationFinding({
      code: 'NO_MATCHING_PDB',
      severity: 'warning',
      summary: 'workload has no matching PDB',
      resource: { kind: 'Deployment', namespace: 'payments', name: 'api', uid: 'uid-1' },
      details: { remediation: 'Create a matching PodDisruptionBudget.' },
      observed_at: '2026-08-10T10:00:00Z',
    }, { framework: 'optimization', source: 'pdb' })

    expect(detail.schema_version).toBe('2')
    expect(detail.rule).toEqual({ rule_id: 'NO_MATCHING_PDB', framework: 'optimization', source: 'pdb' })
    expect(detail.resource).toMatchObject({ kind: 'Deployment', namespace: 'payments', name: 'api', uid: 'uid-1' })
    expect(detail.evidence).toHaveLength(1)
    expect(detail.evidence[0]).toMatchObject({ kind: 'resource_state', observed_at: '2026-08-10T10:00:00Z' })
    expect(detail.recommendations).toEqual([{ kind: 'advisory', text: 'Create a matching PodDisruptionBudget.' }])
    expect(detail.origin_ids).toEqual(['NO_MATCHING_PDB'])
  })

  it('maps diagnosis timeline and actions to the shared evidence vocabulary', () => {
    const record = {
      id: 12,
      cluster_id: 7,
      rule_id: 'pod_oom_killed',
      severity: 'high',
      resource: { kind: 'Pod', namespace: 'payments', name: 'api-0', uid: 'pod-1' },
      status: 'confirmed',
      summary: 'container was OOMKilled',
      root_causes: ['memory limit exceeded'],
      recommendations: ['review memory limit'],
      evidence: [],
      timeline: [{
        index: 0,
        category: 'event',
        type: 'Warning',
        source: 'kubelet',
        ref: 'E1',
        integrity: 'sha256:test',
        occurred_at: '2026-08-10T09:59:00Z',
        missing: true,
        missing_reason: 'event expired',
        summary: 'OOMKilled event',
      }],
      actions: [{
        kind: 'controlled_action',
        title: 'Restart deployment',
        action: 'deployment.rollout_restart',
        requires_dry_run: true,
        requires_confirmation: true,
      }],
      observed_at: '2026-08-10T10:00:00Z',
      sla_due_at: '2026-08-10T11:00:00Z',
      overdue: false,
      created_at: '2026-08-10T10:00:00Z',
      updated_at: '2026-08-10T10:00:00Z',
    } satisfies DiagnosisRecord

    const detail = fromDiagnosis(record)
    expect(detail.evidence[0]).toMatchObject({ id: 'E1', kind: 'event', missing: true, missing_reason: 'event expired' })
    expect(detail.recommendations).toContainEqual({ kind: 'controlled_action_available', text: 'Restart deployment', capability: 'deployment.rollout_restart' })
  })

  it('keeps inspection evidence reachable even when the payload is empty', () => {
    const result = {
      id: 4,
      task_id: 3,
      cluster_id: 7,
      rule_code: 'node_not_ready',
      signal_code: 'NODE_READY_FALSE',
      severity: 'critical',
      state: 'active',
      resource_kind: 'Node',
      resource_name: 'worker-a',
      fingerprint: 'fp-1',
      observed_at: '2026-08-10T10:00:00Z',
    } satisfies InspectionResultView

    const detail = fromInspectionResult(result)
    expect(detail.rule.source).toBe('NODE_READY_FALSE')
    expect(detail.evidence).toEqual([{ id: 'fp-1', kind: 'resource_state', observed_at: '2026-08-10T10:00:00Z', source: 'fp-1' }])
    expect(detail.origin_ids).toEqual(['node_not_ready', 'fp-1'])
  })

  it('merges duplicate resource findings and preserves every rule origin', () => {
    const base = {
      severity: 'warning',
      summary: 'base',
      resource: { kind: 'Deployment', namespace: 'payments', name: 'api' },
      observed_at: '2026-08-10T10:00:00Z',
    }
    const merged = mergeFindingDetails([
      fromOptimizationFinding({ ...base, code: 'RULE_A' }, { source: 'policy' }),
      fromOptimizationFinding({ ...base, code: 'RULE_B' }, { source: 'pdb' }),
    ])

    expect(merged).toHaveLength(1)
    expect(merged[0]?.origin_ids).toEqual(['RULE_A', 'RULE_B'])
    expect(merged[0]?.evidence).toHaveLength(1)
  })
})
