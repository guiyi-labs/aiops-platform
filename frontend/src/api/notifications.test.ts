import { afterEach, describe, expect, it, vi } from 'vitest'

import { listNotificationDeliveries, retryNotificationDelivery } from './notifications'

describe('notification delivery API', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('forwards bounded delivery filters', async () => {
    const response = { items: [], total: 0, remaining: 0 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listNotificationDeliveries('token', { diagnosisID: 7, eventType: 'diagnosis.created', status: 'dead' })).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/notification-deliveries?limit=100&diagnosis_id=7&event_type=diagnosis.created&status=dead', expect.any(Object))
  })

  it('queues a dead delivery retry', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(retryNotificationDelivery('token', 19)).resolves.toBeUndefined()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/notification-deliveries/19/retry', expect.objectContaining({ method: 'POST' }))
  })

  it('forwards an SLA escalation level filter', async () => {
    const response = { items: [], total: 0, remaining: 0 }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(listNotificationDeliveries('token', { incidentID: 10, eventType: 'incident.sla_escalated', escalationLevel: 2 })).resolves.toEqual(response)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/notification-deliveries?limit=100&incident_id=10&event_type=incident.sla_escalated&escalation_level=2', expect.any(Object))
  })
})
