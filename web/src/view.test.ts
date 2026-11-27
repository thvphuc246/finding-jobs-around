import { describe, expect, test } from 'bun:test'
import { loadView } from './view'

describe('loadView', () => {
  test('round-trips a saved view', () => {
    const view = { longitude: 24.94, latitude: 60.17, zoom: 10 }

    expect(loadView(JSON.stringify(view))).toEqual(view)
  })

  test('returns null when nothing is saved', () => {
    expect(loadView(null)).toBeNull()
  })

  test('returns null for broken JSON', () => {
    expect(loadView('{not json')).toBeNull()
  })

  test('returns null for non-numeric values', () => {
    expect(loadView(JSON.stringify({ longitude: '24.94', latitude: 60.17, zoom: 10 }))).toBeNull()
  })

  test('returns null for a non-finite value', () => {
    expect(loadView('{"longitude":1e999,"latitude":60.17,"zoom":10}')).toBeNull()
  })

  test('returns null for a latitude outside the world', () => {
    expect(loadView(JSON.stringify({ longitude: 24.94, latitude: 95, zoom: 10 }))).toBeNull()
  })

  test('returns null for a longitude outside the world', () => {
    expect(loadView(JSON.stringify({ longitude: 200, latitude: 60.17, zoom: 10 }))).toBeNull()
  })

  test('returns null for an out-of-range zoom', () => {
    expect(loadView(JSON.stringify({ longitude: 24.94, latitude: 60.17, zoom: 30 }))).toBeNull()
  })
})
