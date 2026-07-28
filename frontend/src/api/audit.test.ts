import { afterEach, describe, expect, it, vi } from 'vitest'

import { exportAuditLogs, listAuditLogs } from './audit'

describe('audit API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('forwards explicit audit filters', async () => {
    const response = { items: [], total: 0, remaining: 0 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listAuditLogs('token', { clusterID: 7, action: 'cluster.probe', result: 'failure' })).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/audit-logs?limit=100&cluster_id=7&action=cluster.probe&result=failure', expect.any(Object))
  })

  it('downloads a bounded CSV export with the same filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('id,action\r\n1,cluster.probe\r\n', {
      status: 200,
      headers: {
        'Content-Type': 'text/csv', 'Content-Disposition': 'attachment; filename="audit-logs-20260717.csv"',
        'X-Audit-Export-Rows': '1', 'X-Audit-Export-Total': '3', 'X-Audit-Export-Truncated': 'true',
      },
    }))
    vi.stubGlobal('fetch', fetchMock)
    const result = await exportAuditLogs('token', { action: 'cluster.probe', result: 'success' })
    expect(result.filename).toBe('audit-logs-20260717.csv')
    expect(result.rows).toBe(1)
    expect(result.total).toBe(3)
    expect(result.truncated).toBe(true)
    await expect(result.blob.text()).resolves.toContain('cluster.probe')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/audit-logs/export?limit=5000&action=cluster.probe&result=success', expect.objectContaining({ headers: expect.objectContaining({ Accept: 'text/csv' }) }))
  })
})
