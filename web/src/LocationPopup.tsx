import { Popup } from 'react-map-gl/maplibre'
import { STATUS } from './geo'
import type { Location } from './types'

type Props = {
  location: Location
  onClose: () => void
  onFlyTo: (locationId: number) => void
  isOnMap: (locationId: number) => boolean
}

export default function LocationPopup({ location, onClose, onFlyTo, isOnMap }: Props) {
  return (
    <Popup
      longitude={location.lon}
      latitude={location.lat}
      onClose={onClose}
      closeOnClick={false}
      maxWidth="320px"
    >
      <div className="popup">
        <h2>{location.companyName}</h2>
        <p className="address">
          {location.street}, {location.postalCode} {location.city}
        </p>
        <p>{location.kind === 'head_office' ? 'Head office' : 'Branch'}</p>
        {location.approximate ? <p>Approximate location</p> : null}
        {location.careersUrl ? (
          <p>
            <a href={location.careersUrl} target="_blank" rel="noopener noreferrer">
              Careers page
            </a>
          </p>
        ) : (
          <p>No careers page known</p>
        )}
        <p className={`status status-${location.status}`}>{STATUS[location.status].label}</p>
        {location.jobs.length === 0 ? (
          <p>No open jobs</p>
        ) : (
          <ul className="jobs">
            {location.jobs.map((job) => (
              <li key={job.id}>
                <a href={job.url} target="_blank" rel="noopener noreferrer">
                  {job.title}
                </a>
                {job.isNew ? <span className="badge">New</span> : null}
                {job.openApplication ? <span className="tag">Open application</span> : null}
                {job.otherPlaces.length > 0 ? (
                  <p className="places">
                    Also in:{' '}
                    {job.otherPlaces.map((place, index) => {
                      const locationId = place.locationId
                      return (
                        <span key={place.city}>
                          {index > 0 ? ', ' : ''}
                          {locationId !== null && isOnMap(locationId) ? (
                            <button type="button" onClick={() => onFlyTo(locationId)}>
                              {place.city}
                            </button>
                          ) : (
                            place.city
                          )}
                        </span>
                      )
                    })}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </Popup>
  )
}
