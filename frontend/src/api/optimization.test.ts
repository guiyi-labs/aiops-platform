import { afterEach, describe, expect, it, vi } from 'vitest'

import { analyzeCapacity, analyzeCIS, analyzeDeprecatedAPI, analyzeFinOps, analyzeGitOps, analyzeHPA, analyzeImage, analyzeNetwork, analyzePolicy } from './optimization'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

describe('optimization API client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('asks the server to auto-collect the CIS bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 7, evaluated_at: '2026-08-01T10:00:00Z', total: 3, failed: 1, passed: 2,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeCIS('token', 7)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/cis/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    // Only cluster_id is sent: the server collects the bundle itself.
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 7 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.failed).toBe(1)
  })

  it('omits the cost rate unless one is supplied and normalises recommendations', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ cluster_id: 4, containers_evaluated: 0, containers_over_provisioned: 0, monthly_waste_usd: 0, cpu_idle_cores: 0, mem_idle_gb: 0, recommendations: null, evaluated_at: '2026-08-01T10:00:00Z' }))
      .mockResolvedValueOnce(jsonResponse({ cluster_id: 4, containers_evaluated: 1, containers_over_provisioned: 1, monthly_waste_usd: 12.5, cpu_idle_cores: 0.4, mem_idle_gb: 1.2, recommendations: [], evaluated_at: '2026-08-01T10:00:00Z' }))
    vi.stubGlobal('fetch', fetchMock)

    const withoutRate = await analyzeFinOps('token', 4)
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({ cluster_id: 4 })
    expect(withoutRate.recommendations).toEqual([])

    await analyzeFinOps('token', 4, { per_core_month: 30, per_gb_month: 4 })
    expect(JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))).toEqual({
      cluster_id: 4, rate: { per_core_month: 30, per_gb_month: 4 },
    })
  })

  it('sends the target version for the deprecated-API check', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 9, target_minor: 29, total: 2, removed: 1, deprecated: 0, clean: 1,
      findings: [{ code: 'DEPRECATED_API_REMOVED', severity: 'critical', summary: 'removed in 1.16', resource: { kind: 'Deployment', namespace: 'ops', name: 'legacy' }, observed_at: '2026-08-01T10:00:00Z' }],
      evaluated_at: '2026-08-01T10:00:00Z',
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeDeprecatedAPI('token', 9, '1.29')

    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/v1/optimization/deprecated-api/analyze')
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({ cluster_id: 9, target_version: '1.29' })
    expect(status.removed).toBe(1)
    expect(status.findings[0]?.resource.name).toBe('legacy')
  })

  it('surfaces a failed server-side collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 7 is unreachable' }, 502,
    )))

    await expect(analyzeCIS('token', 7)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 7 is unreachable',
    })
  })

  it('asks the server to auto-collect the network bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 12, evaluated_at: '2026-08-01T10:00:00Z', total: 4, failed: 2, passed: 2,
      namespaces_total: 3, pods_total: 9, policies_total: 2, services_total: 4,
      ingress_covered_pods: 6, egress_covered_pods: 0, isolated_namespaces: 1, exposed_services: 1,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeNetwork('token', 12)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/network/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 12 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.failed).toBe(2)
    expect(status.exposed_services).toBe(1)
  })

  it('surfaces a failed network collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 12 is unreachable' }, 502,
    )))

    await expect(analyzeNetwork('token', 12)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 12 is unreachable',
    })
  })

  it('asks the server to auto-collect the image bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 15, evaluated_at: '2026-08-02T10:00:00Z', total: 6, failed: 3, passed: 3,
      images_total: 5, containers_total: 11, mutable_tag_images: 2, unpinned_images: 3,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeImage('token', 15)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/image/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 15 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.mutable_tag_images).toBe(2)
    expect(status.unpinned_images).toBe(3)
  })

  it('surfaces a failed image collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 15 is unreachable' }, 502,
    )))

    await expect(analyzeImage('token', 15)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 15 is unreachable',
    })
  })

  it('asks the server to auto-collect the GitOps bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 18, evaluated_at: '2026-08-02T10:00:00Z', total: 8, failed: 2, passed: 6,
      resources_total: 8, drifted_resources: 1, unmanaged_resources: 1,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeGitOps('token', 18)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/gitops/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 18 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.drifted_resources).toBe(1)
    expect(status.unmanaged_resources).toBe(1)
  })

  it('surfaces a failed GitOps collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 18 is unreachable' }, 502,
    )))

    await expect(analyzeGitOps('token', 18)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 18 is unreachable',
    })
  })

  it('asks the server to auto-collect the capacity bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 21, evaluated_at: '2026-08-02T10:00:00Z', total: 2, failed: 1, passed: 1,
      cpu_capacity_nanocores: 8_000_000_000, mem_capacity_bytes: 17_179_869_184,
      cpu_current_pct: 0.8, mem_current_pct: 0.45,
      cpu_saturation_in_days: 4, mem_saturation_in_days: -1,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeCapacity('token', 21)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/capacity/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 21 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.cpu_capacity_nanocores).toBe(8_000_000_000)
    expect(status.cpu_saturation_in_days).toBe(4)
    expect(status.mem_saturation_in_days).toBe(-1)
  })

  it('surfaces a failed capacity collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 21 is unreachable' }, 502,
    )))

    await expect(analyzeCapacity('token', 21)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 21 is unreachable',
    })
  })

  it('asks the server to auto-collect the policy bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 24, evaluated_at: '2026-08-02T11:00:00Z', total: 11, failed: 8, passed: 3,
      workloads_total: 1, containers_total: 1, compliant_workloads: 0,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzePolicy('token', 24)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/policy/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 24 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.workloads_total).toBe(1)
    expect(status.compliant_workloads).toBe(0)
  })

  it('surfaces a failed policy collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 24 is unreachable' }, 502,
    )))

    await expect(analyzePolicy('token', 24)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 24 is unreachable',
    })
  })

  it('asks the server to auto-collect the HPA bundle and normalises null maps', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      cluster_id: 27, evaluated_at: '2026-08-02T12:00:00Z', total: 3, failed: 1, passed: 2,
      hpas_total: 1, at_max_replicas_count: 1, over_target_count: 0,
      by_severity: null, by_family: null, findings: null,
    }))
    vi.stubGlobal('fetch', fetchMock)

    const status = await analyzeHPA('token', 27)

    const [path, init] = fetchMock.mock.calls[0] ?? []
    expect(path).toBe('/api/v1/optimization/hpa/analyze')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cluster_id: 27 })
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer token', 'Content-Type': 'application/json' })
    // A nil slice/map server-side arrives as null and must not leak to views.
    expect(status.findings).toEqual([])
    expect(status.by_severity).toEqual({})
    expect(status.by_family).toEqual({})
    expect(status.hpas_total).toBe(1)
    expect(status.at_max_replicas_count).toBe(1)
  })

  it('surfaces a failed HPA collection as a stable API error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(
      { code: 'COLLECT_FAILED', message: 'cluster 27 is unreachable' }, 502,
    )))

    await expect(analyzeHPA('token', 27)).rejects.toMatchObject({
      status: 502, code: 'COLLECT_FAILED', message: 'cluster 27 is unreachable',
    })
  })
})
