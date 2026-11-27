import type { LocationsResponse } from './types'

export async function fetchLocations(signal: AbortSignal): Promise<LocationsResponse> {
  const response = await fetch('/api/locations', { signal })
  if (!response.ok) {
    throw new Error(`Failed to load locations (HTTP ${response.status})`)
  }
  return (await response.json()) as LocationsResponse
}
