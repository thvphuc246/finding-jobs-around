import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchLocations } from './api'
import ErrorBoundary from './ErrorBoundary'
import JobsMap from './JobsMap'
import type { Location } from './types'

type State = { kind: 'loading' } | { kind: 'error' } | { kind: 'ready'; locations: Location[] }

export default function App() {
  const [state, setState] = useState<State>({ kind: 'loading' })
  const controllerRef = useRef<AbortController | null>(null)

  const load = useCallback(() => {
    controllerRef.current?.abort()
    const controller = new AbortController()
    controllerRef.current = controller
    setState({ kind: 'loading' })
    fetchLocations(controller.signal)
      .then((response) => setState({ kind: 'ready', locations: response.locations }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) {
          return
        }
        console.error(error)
        setState({ kind: 'error' })
      })
  }, [])

  useEffect(() => {
    load()
    return () => controllerRef.current?.abort()
  }, [load])

  if (state.kind === 'loading') {
    return <p className="status">Loading locations…</p>
  }

  if (state.kind === 'error') {
    return (
      <div className="status" role="alert">
        <p>The locations could not be loaded.</p>
        <button type="button" onClick={load}>
          Retry
        </button>
      </div>
    )
  }

  return (
    <ErrorBoundary>
      <JobsMap locations={state.locations} />
    </ErrorBoundary>
  )
}
