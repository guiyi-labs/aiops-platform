import { authorizedRequest } from './client'
import type { FleetHealthResponse } from '../types/fleet'

export function getFleetHealth(accessToken: string, limit = 20): Promise<FleetHealthResponse> {
  return authorizedRequest(`/api/v1/fleet/health?limit=${limit}`, accessToken)
}
