import type { HiringStatus, Location } from './types'

export type StatusStyle = {
  label: string
  color: string
  stroke?: string
}

export const STATUS: Record<HiringStatus, StatusStyle> = {
  hiring: { label: 'Hiring', color: '#15803d' },
  open_application_only: { label: 'Open application only', color: '#b45309' },
  no_openings: { label: 'No openings', color: '#6b7280' },
  unknown: { label: 'Unknown', color: '#ffffff', stroke: '#6b7280' },
}

export type PointProperties = {
  id: number
  status: HiringStatus
  jobCount: number
}

export type PointFeature = {
  type: 'Feature'
  geometry: { type: 'Point'; coordinates: [number, number] }
  properties: PointProperties
}

export type PointCollection = {
  type: 'FeatureCollection'
  features: PointFeature[]
}

export function toFeatureCollection(locations: Location[], hiringOnly: boolean): PointCollection {
  return {
    type: 'FeatureCollection',
    features: locations
      .filter((location) => !hiringOnly || location.status === 'hiring')
      .map((location) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [location.lon, location.lat] },
        properties: {
          id: location.id,
          status: location.status,
          jobCount: location.jobs.filter((job) => !job.openApplication).length,
        },
      })),
  }
}

export function boundsOf(locations: Location[]): [[number, number], [number, number]] {
  if (locations.length === 0) {
    return [
      [-180, -85],
      [180, 85],
    ]
  }

  let west = Number.POSITIVE_INFINITY
  let south = Number.POSITIVE_INFINITY
  let east = Number.NEGATIVE_INFINITY
  let north = Number.NEGATIVE_INFINITY

  for (const location of locations) {
    west = Math.min(west, location.lon)
    south = Math.min(south, location.lat)
    east = Math.max(east, location.lon)
    north = Math.max(north, location.lat)
  }

  return [
    [west, south],
    [east, north],
  ]
}
