import { describe, expect, test } from 'bun:test'
import { boundsOf, toFeatureCollection } from './geo'
import type { Job, Location } from './types'

function job(overrides: Partial<Job>): Job {
  return {
    id: 1,
    title: 'Job',
    url: 'https://example.com/jobs/1',
    isNew: false,
    openApplication: false,
    remote: '',
    otherPlaces: [],
    ...overrides,
  }
}

function location(overrides: Partial<Location> & Pick<Location, 'id'>): Location {
  return {
    companyId: 1,
    companyName: 'Alpha Oy',
    kind: 'head_office',
    street: 'Street 1',
    postalCode: '00100',
    city: 'Helsinki',
    lat: 60.17,
    lon: 24.94,
    approximate: false,
    careersUrl: null,
    status: 'hiring',
    jobs: [],
    ...overrides,
  }
}

describe('toFeatureCollection', () => {
  test('keeps only hiring locations when hiring only is on', () => {
    const collection = toFeatureCollection(
      [
        location({ id: 1, status: 'hiring' }),
        location({ id: 2, status: 'no_openings' }),
        location({ id: 3, status: 'unknown' }),
      ],
      true,
    )

    expect(collection.features).toHaveLength(1)
    expect(collection.features[0].properties.id).toBe(1)
  })

  test('keeps every location when hiring only is off', () => {
    const collection = toFeatureCollection(
      [location({ id: 1, status: 'hiring' }), location({ id: 2, status: 'open_application_only' })],
      false,
    )

    expect(collection.features).toHaveLength(2)
  })

  test('uses longitude then latitude as coordinates', () => {
    const collection = toFeatureCollection([location({ id: 1, lon: 24.94, lat: 60.17 })], false)

    expect(collection.features[0].geometry.coordinates).toEqual([24.94, 60.17])
  })

  test('counts real jobs and excludes open applications', () => {
    const collection = toFeatureCollection(
      [
        location({
          id: 1,
          jobs: [job({ id: 1 }), job({ id: 2 }), job({ id: 3, openApplication: true })],
        }),
      ],
      false,
    )

    expect(collection.features[0].properties.jobCount).toBe(2)
  })
})

describe('boundsOf', () => {
  test('wraps a single location', () => {
    expect(boundsOf([location({ id: 1, lon: 24.94, lat: 60.17 })])).toEqual([
      [24.94, 60.17],
      [24.94, 60.17],
    ])
  })

  test('covers every location', () => {
    expect(
      boundsOf([
        location({ id: 1, lon: 24.94, lat: 60.17 }),
        location({ id: 2, lon: 23.76, lat: 61.5 }),
        location({ id: 3, lon: 25.04, lat: 60.29 }),
      ]),
    ).toEqual([
      [23.76, 60.17],
      [25.04, 61.5],
    ])
  })
})
