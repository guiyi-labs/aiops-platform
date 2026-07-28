import { afterEach, describe, expect, it, vi } from 'vitest'

import { addAIExplanationFeedback, addDiagnosisFeedback, assignDiagnosis, diagnoseDeployment, diagnoseHorizontalPodAutoscaler, diagnoseIngress, diagnoseNode, diagnosePersistentVolumeClaim, diagnosePod, diagnoseService, executeRemediation, generateDiagnosisExplanation, getAIQualitySummary, getAIRuntimeStatus, getDiagnosis, getDiagnosisSummary, listControlledOperations, listDiagnosisExplanations, listRemediationPlans, previewControlledOperation, previewRemediation, transitionDiagnosis } from './diagnosis'

describe('diagnosis API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('diagnoses an explicit Pod in an explicit cluster', async () => {
    const record = { id: 9, cluster_id: 3, rule_id: 'pod.image_pull_backoff.v1', evidence: [] }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(record), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(diagnosePod('token', 3, 'demo', 'broken-api')).resolves.toEqual(record)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'Pod', namespace: 'demo', name: 'broken-api' }),
    }))
  })

  it('diagnoses a Service and reads persisted evidence', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 10 }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 10, evidence: [{ type: 'endpoints' }] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await diagnoseService('token', 3, 'demo', 'api')
    await getDiagnosis('token', 10)
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'Service', namespace: 'demo', name: 'api' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/diagnoses/10', expect.any(Object))
  })

  it('diagnoses a Node and Deployment through the shared resource endpoint', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ id: 11 }), { status: 201 })))
    vi.stubGlobal('fetch', fetchMock)
    await diagnoseNode('token', 3, 'worker-1')
    await diagnoseDeployment('token', 3, 'demo', 'api')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'Node', namespace: '', name: 'worker-1' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'Deployment', namespace: 'demo', name: 'api' }),
    }))
  })

  it('diagnoses M18 namespaced resources through canonical resource kinds', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ id: 12 }), { status: 201 })))
    vi.stubGlobal('fetch', fetchMock)
    await diagnoseIngress('token', 3, 'demo', 'gateway')
    await diagnosePersistentVolumeClaim('token', 3, 'demo', 'data')
    await diagnoseHorizontalPodAutoscaler('token', 3, 'demo', 'api')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'Ingress', namespace: 'demo', name: 'gateway' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'PersistentVolumeClaim', namespace: 'demo', name: 'data' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/clusters/3/diagnoses', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ resource_kind: 'HorizontalPodAutoscaler', namespace: 'demo', name: 'api' }),
    }))
  })

  it('updates workflow, records feedback and reads summary', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ id: 10, status: 'confirmed', recent: [] }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await transitionDiagnosis('token', 10, 'confirmed', 'issue reproduced')
    await addDiagnosisFeedback('token', 10, 'accurate', 'matched the incident')
    await getDiagnosisSummary('token')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/diagnoses/10', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ status: 'confirmed', comment: 'issue reproduced' }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/diagnoses/10/feedback', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/diagnoses/summary', expect.any(Object))
  })

  it('transfers diagnosis ownership without exposing user data in the route', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 10, assignee: { id: 2 } }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await assignDiagnosis('token', 10, 2, 'handoff to on-call')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/diagnoses/10/assignment', expect.objectContaining({
      method: 'PATCH', body: JSON.stringify({ assignee_user_id: 2, comment: 'handoff to on-call' }),
    }))
  })

  it('lists and generates cited AI explanations', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 4, citations: [{ evidence_id: 'E1' }] }), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    await listDiagnosisExplanations('token', 10)
    await generateDiagnosisExplanation('token', 10)
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/diagnoses/10/explanations', expect.any(Object))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/diagnoses/10/explanations', expect.objectContaining({ method: 'POST' }))
  })

  it('reads AI runtime budget without exposing configuration secrets', async () => {
    const status = { enabled: true, model: 'test-model', used_tokens_today: 250, remaining_tokens: 750 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(status), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(getAIRuntimeStatus('token')).resolves.toEqual(status)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/ai/status', expect.any(Object))
  })

  it('submits one explanation rating and reads aggregate quality', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ feedback: { id: 5, verdict: 'helpful' }, summary: { total: 1, helpful: 1 } }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ total_feedback: 1, helpful: 1, helpful_rate: 1, by_model: [] }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await addAIExplanationFeedback('token', 4, 'helpful', 'evidence was clear')
    await getAIQualitySummary('token')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/ai/explanations/4/feedback', expect.objectContaining({ method: 'POST', body: JSON.stringify({ verdict: 'helpful', comment: 'evidence was clear' }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/ai/quality', expect.any(Object))
  })

  it('previews, lists and explicitly executes a remediation plan', async () => {
    const plan = { id: '12345678-1234-4234-8234-123456789abc', action: 'deployment.rollout_restart', confirmation_token: 'confirm-once' }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(plan), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [plan], total: 1, remaining: 0 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ...plan, status: 'succeeded', confirmation_token: undefined }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await previewRemediation('token', 10, 'deployment.rollout_restart', 'api')
    await listRemediationPlans('token', 10)
    await executeRemediation('token', plan.id, 'confirm-once', 'remediation-request-1')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/diagnoses/10/remediations/preview', expect.objectContaining({ method: 'POST', body: JSON.stringify({ action: 'deployment.rollout_restart', target_name: 'api' }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/diagnoses/10/remediations', expect.any(Object))
    expect(fetchMock).toHaveBeenNthCalledWith(3, `/api/v1/remediations/${plan.id}/execute`, expect.objectContaining({
      method: 'POST', headers: expect.objectContaining({ 'Idempotency-Key': 'remediation-request-1' }), body: JSON.stringify({ confirmation_token: 'confirm-once' }),
    }))
  })

  it('serializes only fixed controlled operation parameters', async () => {
    const plan = { id: '12345678-1234-4234-8234-123456789abc', action: 'deployment.scale' }
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify(plan), { status: 201 })))
    vi.stubGlobal('fetch', fetchMock)
    await previewControlledOperation('token', 7, { action: 'deployment.scale', namespace: 'demo', target_name: 'api', desired_replicas: 3 })
    await previewControlledOperation('token', 7, { action: 'cronjob.suspend', namespace: 'demo', target_name: 'cleanup' })
    await previewControlledOperation('token', 7, { action: 'cronjob.resume', namespace: 'demo', target_name: 'cleanup' })
    await listControlledOperations('token', 7, 'demo', 'CronJob', 'cleanup')
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/clusters/7/operations/preview', expect.objectContaining({ method: 'POST', body: JSON.stringify({ action: 'deployment.scale', namespace: 'demo', target_name: 'api', desired_replicas: 3 }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/clusters/7/operations/preview', expect.objectContaining({ method: 'POST', body: JSON.stringify({ action: 'cronjob.suspend', namespace: 'demo', target_name: 'cleanup' }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/clusters/7/operations/preview', expect.objectContaining({ method: 'POST', body: JSON.stringify({ action: 'cronjob.resume', namespace: 'demo', target_name: 'cleanup' }) }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/v1/clusters/7/operations?namespace=demo&target_kind=CronJob&target_name=cleanup', expect.any(Object))
  })
})
