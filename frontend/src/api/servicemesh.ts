import { authorizedRequest } from './client'
import type { VirtualServiceView, DestinationRuleView, TrafficMetrics } from '../types/servicemesh'

export function listVirtualServices(token: string, clusterId: number, params?: { namespace?: string; name?: string; limit?: number }): Promise<{ items: VirtualServiceView[]; total: number; truncated: boolean }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterId}/service-mesh/virtual-services${q ? `?${q}` : ''}`, token)
}

export function getVirtualService(token: string, clusterId: number, namespace: string, name: string): Promise<VirtualServiceView> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/service-mesh/virtual-services/${namespace}/${name}`, token)
}

export function listDestinationRules(token: string, clusterId: number, params?: { namespace?: string; name?: string; limit?: number }): Promise<{ items: DestinationRuleView[]; total: number; truncated: boolean }> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterId}/service-mesh/destination-rules${q ? `?${q}` : ''}`, token)
}

export function getDestinationRule(token: string, clusterId: number, namespace: string, name: string): Promise<DestinationRuleView> {
  return authorizedRequest(`/api/v1/clusters/${clusterId}/service-mesh/destination-rules/${namespace}/${name}`, token)
}

export function getTrafficMetrics(token: string, clusterId: number, params?: { namespace?: string; service_name?: string; top_k?: number }): Promise<TrafficMetrics> {
  const sp = new URLSearchParams()
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
    }
  }
  const q = sp.toString()
  return authorizedRequest(`/api/v1/clusters/${clusterId}/service-mesh/traffic-metrics${q ? `?${q}` : ''}`, token)
}
