import { afterEach, describe, expect, it, vi } from 'vitest'

import { getConfigMap, getCronJob, getDaemonSet, getDeployment, getHorizontalPodAutoscaler, getIngress, getJob, getLimitRange, getNode, getPersistentVolumeClaim, getPod, getPodLogs, getReplicaSet, getResourceQuota, getSecret, getService, getStatefulSet, getStorageClass, listConfigMaps, listCronJobs, listDaemonSets, listDeployments, listEndpointSlices, listEvents, listHorizontalPodAutoscalers, listIngresses, listJobs, listLimitRanges, listNodeMetrics, listNodes, listPersistentVolumeClaims, listPodMetrics, listPods, listReplicaSets, listResourceQuotas, listSecrets, listServices, listStatefulSets, listStorageClasses } from './kubernetes'

describe('Kubernetes API client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('keeps cluster and namespace explicit in a Pod query', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await listPods('token', 17, 'platform', 'api')
    const target = String(fetchMock.mock.calls[0]?.[0])
    expect(target).toContain('/clusters/17/pods?')
    expect(target).toContain('namespace=platform')
    expect(target).toContain('name=api')
  })

  it('uses fixed bounded Metrics API routes', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await listNodeMetrics('token', 17, 'worker')
    await listPodMetrics('token', 17, 'platform', 'api')
    const targets = fetchMock.mock.calls.map((call) => String(call[0]))
    expect(targets[0]).toContain('/clusters/17/metrics/nodes?name=worker')
    expect(targets[0]).toContain('limit=100')
    expect(targets[1]).toContain('/clusters/17/metrics/pods?namespace=platform&name=api')
    expect(targets[1]).toContain('limit=100')
  })

  it('requests bounded previous logs for a concrete Pod', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ logs: 'previous line', previous: true }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(getPodLogs('token', 3, 'prod', 'api-1', 'app', true)).resolves.toEqual({ logs: 'previous line', previous: true })
    const target = String(fetchMock.mock.calls[0]?.[0])
    expect(target).toContain('/clusters/3/pods/prod/api-1/logs?')
    expect(target).toContain('previous=true')
    expect(target).toContain('tail_lines=200')
  })

  it('keeps cluster, namespace and resource name explicit in an Event query', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    await listEvents('token', 11, 'payments', 'checkout')
    const target = String(fetchMock.mock.calls[0]?.[0])
    expect(target).toContain('/clusters/11/events?')
    expect(target).toContain('namespace=payments')
    expect(target).toContain('name=checkout')
    expect(target).toContain('limit=100')
  })

  it('applies a name filter to every workload inventory', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await listNodes('token', 9, 'worker')
    await listDeployments('token', 9, 'platform', 'api')
    await listServices('token', 9, 'platform', 'gateway')
    await listIngresses('token', 9, 'platform', 'edge')
    await listEndpointSlices('token', 9, 'platform', 'api')
    await listPersistentVolumeClaims('token', 9, 'platform', 'cache')
    await listStorageClasses('token', 9, 'fast')
    await listConfigMaps('token', 9, 'platform', 'runtime')
    const targets = fetchMock.mock.calls.map((call) => String(call[0]))
    expect(targets[0]).toContain('/clusters/9/nodes?name=worker')
    expect(targets[1]).toContain('/clusters/9/deployments?namespace=platform&name=api')
    expect(targets[2]).toContain('/clusters/9/services?namespace=platform&name=gateway')
    expect(targets[3]).toContain('/clusters/9/ingresses?namespace=platform&name=edge')
    expect(targets[4]).toContain('/clusters/9/endpointslices?namespace=platform&name=api')
    expect(targets[5]).toContain('/clusters/9/persistentvolumeclaims?namespace=platform&name=cache')
    expect(targets[6]).toContain('/clusters/9/storageclasses?name=fast')
    expect(targets[7]).toContain('/clusters/9/configmaps?namespace=platform&name=runtime')
  })

  it('uses fixed encoded paths for resource detail reads', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ metadata: { name: 'resource' } }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await getPod('token', 5, 'team one', 'api/canary')
    await getDeployment('token', 5, 'team one', 'api/canary')
    await getService('token', 5, 'team one', 'api/canary')
    await getNode('token', 5, 'worker/one')
    await getIngress('token', 5, 'team one', 'web/canary')
    await getPersistentVolumeClaim('token', 5, 'team one', 'cache/v2')
    await getStorageClass('token', 5, 'fast/local')
    await getConfigMap('token', 5, 'team one', 'runtime/v2')
    const targets = fetchMock.mock.calls.map((call) => String(call[0]))
    expect(targets[0]).toContain('/clusters/5/pods/team%20one/api%2Fcanary')
    expect(targets[1]).toContain('/clusters/5/deployments/team%20one/api%2Fcanary')
    expect(targets[2]).toContain('/clusters/5/services/team%20one/api%2Fcanary')
    expect(targets[3]).toContain('/clusters/5/nodes/worker%2Fone')
    expect(targets[4]).toContain('/clusters/5/ingresses/team%20one/web%2Fcanary')
    expect(targets[5]).toContain('/clusters/5/persistentvolumeclaims/team%20one/cache%2Fv2')
    expect(targets[6]).toContain('/clusters/5/storageclasses/fast%2Flocal')
    expect(targets[7]).toContain('/clusters/5/configmaps/team%20one/runtime%2Fv2')
  })

  it('uses fixed bounded list routes for M17 resources', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ items: [], total: 0, remaining: 0 }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await listStatefulSets('token', 21, 'team one', 'db')
    await listDaemonSets('token', 21, 'team one', 'agent')
    await listReplicaSets('token', 21, 'team one', 'api')
    await listJobs('token', 21, 'team one', 'backup')
    await listCronJobs('token', 21, 'team one', 'cleanup')
    await listHorizontalPodAutoscalers('token', 21, 'team one', 'web')
    await listResourceQuotas('token', 21, 'team one', 'quota')
    await listLimitRanges('token', 21, 'team one', 'defaults')
    await listSecrets('token', 21, 'team one', 'runtime')
    const targets = fetchMock.mock.calls.map((call) => String(call[0]))
    expect(targets).toEqual([
      expect.stringContaining('/clusters/21/statefulsets?namespace=team+one&name=db'),
      expect.stringContaining('/clusters/21/daemonsets?namespace=team+one&name=agent'),
      expect.stringContaining('/clusters/21/replicasets?namespace=team+one&name=api'),
      expect.stringContaining('/clusters/21/jobs?namespace=team+one&name=backup'),
      expect.stringContaining('/clusters/21/cronjobs?namespace=team+one&name=cleanup'),
      expect.stringContaining('/clusters/21/horizontalpodautoscalers?namespace=team+one&name=web'),
      expect.stringContaining('/clusters/21/resourcequotas?namespace=team+one&name=quota'),
      expect.stringContaining('/clusters/21/limitranges?namespace=team+one&name=defaults'),
      expect.stringContaining('/clusters/21/secrets?namespace=team+one&name=runtime'),
    ])
    for (const target of targets) expect(target).toContain('limit=100')
  })

  it('uses fixed encoded detail routes for M17 resources', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ metadata: { name: 'resource' } }), { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await getStatefulSet('token', 21, 'team one', 'db/v2')
    await getDaemonSet('token', 21, 'team one', 'agent/v2')
    await getReplicaSet('token', 21, 'team one', 'api/v2')
    await getJob('token', 21, 'team one', 'backup/v2')
    await getCronJob('token', 21, 'team one', 'cleanup/v2')
    await getHorizontalPodAutoscaler('token', 21, 'team one', 'web/v2')
    await getResourceQuota('token', 21, 'team one', 'quota/v2')
    await getLimitRange('token', 21, 'team one', 'defaults/v2')
    await getSecret('token', 21, 'team one', 'runtime/v2')
    const targets = fetchMock.mock.calls.map((call) => String(call[0]))
    expect(targets).toEqual([
      expect.stringContaining('/clusters/21/statefulsets/team%20one/db%2Fv2'),
      expect.stringContaining('/clusters/21/daemonsets/team%20one/agent%2Fv2'),
      expect.stringContaining('/clusters/21/replicasets/team%20one/api%2Fv2'),
      expect.stringContaining('/clusters/21/jobs/team%20one/backup%2Fv2'),
      expect.stringContaining('/clusters/21/cronjobs/team%20one/cleanup%2Fv2'),
      expect.stringContaining('/clusters/21/horizontalpodautoscalers/team%20one/web%2Fv2'),
      expect.stringContaining('/clusters/21/resourcequotas/team%20one/quota%2Fv2'),
      expect.stringContaining('/clusters/21/limitranges/team%20one/defaults%2Fv2'),
      expect.stringContaining('/clusters/21/secrets/team%20one/runtime%2Fv2'),
    ])
  })
})
