export type MapView = {
  longitude: number
  latitude: number
  zoom: number
}

const storageKey = 'map-view'

export function loadView(raw: string | null): MapView | null {
  if (!raw) {
    return null
  }

  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    return null
  }

  if (typeof value !== 'object' || value === null) {
    return null
  }

  const { longitude, latitude, zoom } = value as Record<string, unknown>
  if (!isFiniteNumber(longitude) || !isFiniteNumber(latitude) || !isFiniteNumber(zoom)) {
    return null
  }
  if (longitude < -180 || longitude > 180) {
    return null
  }
  if (latitude < -90 || latitude > 90) {
    return null
  }
  if (zoom < 0 || zoom > 24) {
    return null
  }

  return { longitude, latitude, zoom }
}

export function saveView(view: MapView): void {
  localStorage.setItem(storageKey, JSON.stringify(view))
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}
