import type { HealthResponse } from '../types/health'

export async function getReadiness(signal?: AbortSignal): Promise<HealthResponse> {
  const response = await fetch('/api/v1/health/ready', {
    headers: { Accept: 'application/json' },
    signal,
  })

  if (!response.ok) {
    throw new Error(`Readiness request failed with status ${response.status}`)
  }

  return response.json() as Promise<HealthResponse>
}
