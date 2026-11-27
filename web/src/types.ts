export type HiringStatus = 'hiring' | 'open_application_only' | 'no_openings' | 'unknown'

export type OtherPlace = {
  city: string
  locationId: number | null
}

export type Job = {
  id: number
  title: string
  url: string
  isNew: boolean
  openApplication: boolean
  remote: string
  otherPlaces: OtherPlace[]
}

export type Location = {
  id: number
  companyId: number
  companyName: string
  kind: 'head_office' | 'branch'
  street: string
  postalCode: string
  city: string
  lat: number
  lon: number
  approximate: boolean
  careersUrl: string | null
  status: HiringStatus
  jobs: Job[]
}

export type LocationsResponse = {
  locations: Location[]
}
