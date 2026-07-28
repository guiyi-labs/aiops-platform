import type {
  CreateSavedGlobalSearchFilter,
  GlobalSearchKind,
  GlobalSearchParameters,
  GlobalSearchResponse,
  SavedGlobalSearchFilter,
  SavedGlobalSearchFilterList,
  UpdateSavedGlobalSearchFilter,
} from '../types/global-search'
import { authorizedRequest } from './client'

const kindQueryValue: Record<GlobalSearchKind, string> = {
  Pod: 'pods',
  Deployment: 'deployments',
  Service: 'services',
  Ingress: 'ingresses',
}

export function searchFleetResources(accessToken: string, parameters: GlobalSearchParameters, signal?: AbortSignal): Promise<GlobalSearchResponse> {
  const query = new URLSearchParams({ q: parameters.query })
  if (parameters.namespace) query.set('namespace', parameters.namespace)
  query.set('kinds', parameters.kinds.map((kind) => kindQueryValue[kind]).join(','))
  query.set('cluster_limit', String(parameters.clusterLimit ?? 20))
  query.set('limit', String(parameters.limit ?? 50))
  return authorizedRequest<GlobalSearchResponse>(`/api/v1/fleet/resources/search?${query.toString()}`, accessToken, { signal })
}

const savedFilterPath = '/api/v1/fleet/resources/search/filters'

export function listSavedGlobalSearchFilters(accessToken: string): Promise<SavedGlobalSearchFilterList> {
  return authorizedRequest<SavedGlobalSearchFilterList>(savedFilterPath, accessToken)
}

export function createSavedGlobalSearchFilter(accessToken: string, input: CreateSavedGlobalSearchFilter): Promise<SavedGlobalSearchFilter> {
  return authorizedRequest<SavedGlobalSearchFilter>(savedFilterPath, accessToken, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function updateSavedGlobalSearchFilter(accessToken: string, id: number, input: UpdateSavedGlobalSearchFilter): Promise<SavedGlobalSearchFilter> {
  return authorizedRequest<SavedGlobalSearchFilter>(`${savedFilterPath}/${id}`, accessToken, {
    method: 'PATCH',
    body: JSON.stringify(input),
  })
}

export function deleteSavedGlobalSearchFilter(accessToken: string, id: number): Promise<void> {
  return authorizedRequest<void>(`${savedFilterPath}/${id}`, accessToken, { method: 'DELETE' })
}
