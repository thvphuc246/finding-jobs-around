import type { FilterSpecification, GeoJSONSource } from 'maplibre-gl'
import { useCallback, useMemo, useRef, useState } from 'react'
import MapView, {
  AttributionControl,
  GeolocateControl,
  Layer,
  type MapLayerMouseEvent,
  type MapRef,
  NavigationControl,
  Source,
  type ViewStateChangeEvent,
} from 'react-map-gl/maplibre'
import Controls from './Controls'
import { boundsOf, STATUS, toFeatureCollection } from './geo'
import LocationPopup from './LocationPopup'
import type { Location } from './types'
import { loadView, saveView } from './view'

const MAP_STYLE = 'https://tiles.openfreemap.org/styles/liberty'

const clusterFilter: FilterSpecification = ['has', 'point_count']
const pointFilter: FilterSpecification = ['!', ['has', 'point_count']]
const jobCountFilter: FilterSpecification = [
  'all',
  ['!', ['has', 'point_count']],
  ['>', ['get', 'jobCount'], 0],
]

export default function JobsMap({ locations }: { locations: Location[] }) {
  const mapRef = useRef<MapRef>(null)
  const [hiringOnly, setHiringOnly] = useState(false)
  const [selectedId, setSelectedId] = useState<number | null>(null)

  const data = useMemo(() => toFeatureCollection(locations, hiringOnly), [locations, hiringOnly])

  const selected = useMemo(
    () =>
      selectedId === null
        ? null
        : (locations.find((location) => location.id === selectedId) ?? null),
    [locations, selectedId],
  )

  const initialViewState = useMemo(() => {
    const saved = loadView(localStorage.getItem('map-view'))
    if (saved) {
      return saved
    }
    return { bounds: boundsOf(locations), fitBoundsOptions: { padding: 40 } }
  }, [locations])

  const handleMoveEnd = useCallback((event: ViewStateChangeEvent) => {
    saveView({
      longitude: event.viewState.longitude,
      latitude: event.viewState.latitude,
      zoom: event.viewState.zoom,
    })
  }, [])

  const handleClick = useCallback((event: MapLayerMouseEvent) => {
    const map = mapRef.current
    const feature = event.features?.[0]
    if (!map || !feature) {
      return
    }

    if (feature.layer?.id === 'clusters') {
      const clusterId = Number(feature.properties?.cluster_id)
      const source = map.getSource('locations') as GeoJSONSource | undefined
      if (!source || Number.isNaN(clusterId)) {
        return
      }
      void source.getClusterExpansionZoom(clusterId).then((zoom) => {
        map.easeTo({ center: event.lngLat, zoom })
      })
      return
    }

    if (feature.layer?.id === 'points') {
      setSelectedId(Number(feature.properties?.id))
    }
  }, [])

  const handleMouseMove = useCallback((event: MapLayerMouseEvent) => {
    const map = mapRef.current
    if (!map) {
      return
    }
    map.getCanvas().style.cursor = event.features && event.features.length > 0 ? 'pointer' : ''
  }, [])

  const handleMouseLeave = useCallback(() => {
    const map = mapRef.current
    if (map) {
      map.getCanvas().style.cursor = ''
    }
  }, [])

  const flyTo = useCallback(
    (locationId: number) => {
      const target = locations.find((location) => location.id === locationId)
      if (!target) {
        return
      }
      setSelectedId(locationId)
      mapRef.current?.flyTo({ center: [target.lon, target.lat], zoom: 14 })
    },
    [locations],
  )

  const isOnMap = useCallback(
    (locationId: number) => locations.some((location) => location.id === locationId),
    [locations],
  )

  return (
    <div className="map-shell">
      <MapView
        ref={mapRef}
        mapStyle={MAP_STYLE}
        initialViewState={initialViewState}
        interactiveLayerIds={['clusters', 'points']}
        onClick={handleClick}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
        onMoveEnd={handleMoveEnd}
        style={{ width: '100%', height: '100%' }}
      >
        <NavigationControl position="bottom-right" />
        <GeolocateControl position="bottom-right" positionOptions={{ enableHighAccuracy: true }} />
        <AttributionControl customAttribution="Geocoding: Digitransit" />
        <Source
          id="locations"
          type="geojson"
          data={data}
          cluster
          clusterRadius={50}
          clusterMaxZoom={13}
        >
          <Layer
            id="clusters"
            type="circle"
            filter={clusterFilter}
            paint={{
              'circle-color': '#0f172a',
              'circle-radius': ['step', ['get', 'point_count'], 16, 10, 20, 50, 26],
              'circle-stroke-color': '#ffffff',
              'circle-stroke-width': 2,
            }}
          />
          <Layer
            id="cluster-count"
            type="symbol"
            filter={clusterFilter}
            layout={{
              'text-field': '{point_count_abbreviated}',
              'text-font': ['Noto Sans Bold'],
              'text-size': 12,
            }}
            paint={{ 'text-color': '#ffffff' }}
          />
          <Layer
            id="points"
            type="circle"
            filter={pointFilter}
            paint={{
              'circle-color': [
                'match',
                ['get', 'status'],
                'hiring',
                STATUS.hiring.color,
                'open_application_only',
                STATUS.open_application_only.color,
                'no_openings',
                STATUS.no_openings.color,
                'unknown',
                STATUS.unknown.color,
                '#6b7280',
              ],
              'circle-radius': 8,
              'circle-stroke-color': [
                'match',
                ['get', 'status'],
                'unknown',
                STATUS.unknown.stroke ?? '#6b7280',
                '#ffffff',
              ],
              'circle-stroke-width': 2,
            }}
          />
          <Layer
            id="job-count"
            type="symbol"
            filter={jobCountFilter}
            minzoom={12}
            layout={{
              'text-field': ['concat', ['to-string', ['get', 'jobCount']], ' jobs'],
              'text-font': ['Noto Sans Bold'],
              'text-size': 12,
              'text-anchor': 'top',
              'text-offset': [0, 1.2],
            }}
            paint={{
              'text-color': '#111827',
              'text-halo-color': '#ffffff',
              'text-halo-width': 1.5,
            }}
          />
        </Source>
        {selected ? (
          <LocationPopup
            location={selected}
            onClose={() => setSelectedId(null)}
            onFlyTo={flyTo}
            isOnMap={isOnMap}
          />
        ) : null}
      </MapView>
      <Controls hiringOnly={hiringOnly} onHiringOnlyChange={setHiringOnly} />
    </div>
  )
}
