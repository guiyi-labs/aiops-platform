import type { Deployment, EndpointSliceResource, IngressBackend, IngressResource, Pod, ServiceResource } from '../types/kubernetes'
import type { ResourceHealth } from './resource-health'
import { selectsPod } from './resource-health'

export interface IngressServiceReference {
  serviceName: string
  port: string
  host: string
  path: string
}

export type TopologyResourceKind = 'Ingress' | 'Service' | 'EndpointSlice' | 'Pod' | 'Deployment'
export interface TopologySelection { kind: TopologyResourceKind; namespace: string; name: string }
export interface TopologyResources {
  ingresses: IngressResource[]
  services: ServiceResource[]
  endpointSlices: EndpointSliceResource[]
  pods: Pod[]
  deployments: Deployment[]
}

export function ingressServiceBackends(ingress: IngressResource): IngressServiceReference[] {
  const references: IngressServiceReference[] = []
  if (ingress.spec.defaultBackend) references.push(backendReference(ingress.spec.defaultBackend, '*', 'default'))
  for (const rule of ingress.spec.rules ?? []) {
    for (const path of rule.http?.paths ?? []) references.push(backendReference(path.backend, rule.host || '*', path.path || '/'))
  }
  return references.filter((item) => item.serviceName !== '')
}

export function ingressSelectsService(ingress: IngressResource, service: ServiceResource): boolean {
  return sameNamespace(ingress, service) && ingressServiceBackends(ingress).some((item) => item.serviceName === service.metadata.name)
}

export function endpointSliceSelectsService(endpointSlice: EndpointSliceResource, service: ServiceResource): boolean {
  return sameNamespace(endpointSlice, service) && endpointSlice.serviceName !== undefined && endpointSlice.serviceName !== '' && endpointSlice.serviceName === service.metadata.name
}

export function endpointSliceSelectsPod(endpointSlice: EndpointSliceResource, pod: Pod): boolean {
  if (!sameNamespace(endpointSlice, pod)) return false
  return (endpointSlice.endpoints ?? []).some((endpoint) => endpoint.targetRef?.kind === 'Pod'
    && endpoint.targetRef.namespace === pod.metadata.namespace
    && endpoint.targetRef.name === pod.metadata.name)
}

export function serviceSelectsPod(service: ServiceResource, pod: Pod, endpointSlices: EndpointSliceResource[]): boolean {
  const serviceSlices = endpointSlices.filter((endpointSlice) => endpointSliceSelectsService(endpointSlice, service))
  if (serviceSlices.length > 0) return serviceSlices.some((endpointSlice) => endpointSliceSelectsPod(endpointSlice, pod))
  return selectsPod(service.metadata.namespace, service.spec.selector, pod)
}

export function endpointSliceHealth(endpointSlice: EndpointSliceResource): ResourceHealth {
  const endpoints = endpointSlice.endpoints ?? []
  if (endpoints.length === 0) return 'critical'
  if (endpoints.some((endpoint) => endpoint.conditions?.ready === false || endpoint.conditions?.serving === false || endpoint.conditions?.terminating === true)) return 'warning'
  return 'healthy'
}

export function topologyResourceKey(kind: TopologyResourceKind, resource: { metadata: { namespace?: string; name: string } }): string {
  return `${kind}/${resource.metadata.namespace ?? ''}/${resource.metadata.name}`
}

export function connectedTopologyKeys(selection: TopologySelection | null, resources: TopologyResources): Set<string> {
  if (!selection) return new Set<string>()
  const graph = new Map<string, Set<string>>()
  const register = (kind: TopologyResourceKind, resource: { metadata: { namespace?: string; name: string } }) => {
    const key = topologyResourceKey(kind, resource)
    if (!graph.has(key)) graph.set(key, new Set<string>())
    return key
  }
  const connect = (left: string, right: string) => {
    graph.get(left)?.add(right)
    graph.get(right)?.add(left)
  }

  resources.ingresses.forEach((item) => register('Ingress', item))
  resources.services.forEach((item) => register('Service', item))
  resources.endpointSlices.forEach((item) => register('EndpointSlice', item))
  resources.pods.forEach((item) => register('Pod', item))
  resources.deployments.forEach((item) => register('Deployment', item))

  for (const ingress of resources.ingresses) for (const service of resources.services) {
    if (ingressSelectsService(ingress, service)) connect(topologyResourceKey('Ingress', ingress), topologyResourceKey('Service', service))
  }
  for (const service of resources.services) {
    for (const endpointSlice of resources.endpointSlices) {
      if (endpointSliceSelectsService(endpointSlice, service)) connect(topologyResourceKey('Service', service), topologyResourceKey('EndpointSlice', endpointSlice))
    }
    for (const pod of resources.pods) {
      if (serviceSelectsPod(service, pod, resources.endpointSlices)) connect(topologyResourceKey('Service', service), topologyResourceKey('Pod', pod))
    }
  }
  for (const endpointSlice of resources.endpointSlices) for (const pod of resources.pods) {
    if (endpointSliceSelectsPod(endpointSlice, pod)) connect(topologyResourceKey('EndpointSlice', endpointSlice), topologyResourceKey('Pod', pod))
  }
  for (const deployment of resources.deployments) for (const pod of resources.pods) {
    if (selectsPod(deployment.metadata.namespace, deployment.spec.selector.matchLabels, pod)) connect(topologyResourceKey('Deployment', deployment), topologyResourceKey('Pod', pod))
  }

  const selectedKey = `${selection.kind}/${selection.namespace}/${selection.name}`
  if (!graph.has(selectedKey)) return new Set<string>()
  const visited = new Set([selectedKey])
  const queue = [selectedKey]
  while (queue.length > 0) {
    const current = queue.shift()!
    for (const neighbor of graph.get(current) ?? []) {
      if (visited.has(neighbor)) continue
      visited.add(neighbor)
      queue.push(neighbor)
    }
  }
  return visited
}

function backendReference(backend: IngressBackend, host: string, path: string): IngressServiceReference {
  return {
    serviceName: backend.service?.name ?? '',
    port: backend.service?.port.name || (backend.service?.port.number !== undefined ? String(backend.service.port.number) : '--'),
    host,
    path,
  }
}

function sameNamespace(left: { metadata: { namespace?: string } }, right: { metadata: { namespace?: string } }): boolean {
  return (left.metadata.namespace ?? '') === (right.metadata.namespace ?? '')
}
