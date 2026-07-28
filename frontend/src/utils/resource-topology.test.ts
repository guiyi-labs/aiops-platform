import { describe, expect, it } from 'vitest'

import type { Deployment, EndpointSliceResource, IngressResource, Pod, ServiceResource } from '../types/kubernetes'
import { connectedTopologyKeys, endpointSliceHealth, endpointSliceSelectsPod, endpointSliceSelectsService, ingressSelectsService, ingressServiceBackends, serviceSelectsPod } from './resource-topology'

function service(namespace = 'team-a', name = 'api', selector: Record<string, string> = { app: 'api' }): ServiceResource {
  return { metadata: { namespace, name }, spec: { type: 'ClusterIP', selector, ports: [{ protocol: 'TCP', port: 80 }] } }
}

function pod(namespace = 'team-a', name = 'api-1'): Pod {
  return { metadata: { namespace, name, labels: { app: 'api' } }, spec: { containers: [] }, status: { phase: 'Running' } }
}

function endpointSlice(namespace = 'team-a', serviceName = 'api', targetNamespace = namespace, targetName = 'api-1', targetKind = 'Pod'): EndpointSliceResource {
  return {
    metadata: { namespace, name: `${serviceName}-slice` },
    addressType: 'IPv4',
    serviceName,
    ports: [{ name: 'http', protocol: 'TCP', port: 8080 }],
    endpoints: [{ addresses: ['10.0.0.8'], conditions: { ready: true }, targetRef: { kind: targetKind, namespace: targetNamespace, name: targetName } }],
  }
}

function ingress(namespace = 'team-a'): IngressResource {
  return {
    metadata: { namespace, name: 'edge' },
    spec: {
      defaultBackend: { service: { name: 'fallback', port: { number: 8080 } } },
      rules: [{ host: 'api.example.test', http: { paths: [{ path: '/api', backend: { service: { name: 'api', port: { name: 'http' } } } }] } }],
    },
    status: { loadBalancer: {} },
  }
}

function deployment(): Deployment {
  return {
    metadata: { namespace: 'team-a', name: 'api' },
    spec: { replicas: 1, selector: { matchLabels: { app: 'api' } }, template: { spec: { containers: [] } } },
    status: { replicas: 1, readyReplicas: 1, availableReplicas: 1, updatedReplicas: 1, unavailableReplicas: 0 },
  }
}

describe('resource topology', () => {
  it('maps Ingress backends only to same-Namespace Services and preserves named and numeric ports', () => {
    const item = ingress()
    expect(ingressServiceBackends(item)).toEqual([
      { serviceName: 'fallback', port: '8080', host: '*', path: 'default' },
      { serviceName: 'api', port: 'http', host: 'api.example.test', path: '/api' },
    ])
    expect(ingressSelectsService(item, service('team-a', 'api'))).toBe(true)
    expect(ingressSelectsService(item, service('team-b', 'api'))).toBe(false)
  })

  it('maps EndpointSlices to Services by exact standard label identity', () => {
    const slice = endpointSlice()
    expect(endpointSliceSelectsService(slice, service())).toBe(true)
    expect(endpointSliceSelectsService(slice, service('team-a', 'worker'))).toBe(false)
    expect(endpointSliceSelectsService(slice, service('team-b', 'api'))).toBe(false)
  })

  it('maps EndpointSlices only to exact same-Namespace Pod targetRefs', () => {
    expect(endpointSliceSelectsPod(endpointSlice(), pod())).toBe(true)
    expect(endpointSliceSelectsPod(endpointSlice('team-a', 'api', 'team-b'), pod())).toBe(false)
    expect(endpointSliceSelectsPod(endpointSlice('team-a', 'api', 'team-a', 'api-2'), pod())).toBe(false)
    expect(endpointSliceSelectsPod(endpointSlice('team-a', 'api', 'team-a', 'api-1', 'Node'), pod())).toBe(false)
  })

  it('keeps not-ready endpoints represented and grades their slice as unhealthy', () => {
    const slice = endpointSlice()
    slice.endpoints[0]!.conditions = { ready: false, serving: true }
    expect(slice.endpoints).toHaveLength(1)
    expect(endpointSliceHealth(slice)).toBe('warning')
  })

  it('falls back to a complete Service selector only when no matching EndpointSlice exists', () => {
    const item = service()
    const selectedPod = pod()
    expect(serviceSelectsPod(item, selectedPod, [])).toBe(true)
    expect(serviceSelectsPod(item, selectedPod, [endpointSlice('team-a', 'worker')])).toBe(true)
    expect(serviceSelectsPod(item, selectedPod, [endpointSlice('team-a', 'api', 'team-a', 'api-2')])).toBe(false)
    expect(serviceSelectsPod(service('team-a', 'api', {}), selectedPod, [])).toBe(false)
    expect(serviceSelectsPod(item, pod('team-b'), [])).toBe(false)
  })

  it('walks the complete Ingress-to-Pod topology while retaining Deployment relationships', () => {
    const keys = connectedTopologyKeys(
      { kind: 'Ingress', namespace: 'team-a', name: 'edge' },
      { ingresses: [ingress()], services: [service()], endpointSlices: [endpointSlice()], pods: [pod()], deployments: [deployment()] },
    )
    expect([...keys].sort()).toEqual([
      'Deployment/team-a/api',
      'EndpointSlice/team-a/api-slice',
      'Ingress/team-a/edge',
      'Pod/team-a/api-1',
      'Service/team-a/api',
    ])
  })
})
