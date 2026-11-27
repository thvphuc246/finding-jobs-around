package store

import (
	"context"
	"fmt"
	"time"
)

type MapLocation struct {
	ID          int64
	CompanyID   int64
	CompanyName string
	Kind        string
	Street      string
	PostalCode  string
	City        string
	Lat         float64
	Lon         float64
	Approximate bool
	CareersURL  *string
	HasFeed     bool
}

type MapJob struct {
	ID              int64
	LocationID      int64
	Title           string
	URL             string
	Remote          string
	OpenApplication bool
	FirstSeenAt     time.Time
}

type JobLocation struct {
	JobID      int64
	LocationID int64
	City       string
}

type JobPlace struct {
	JobID   int64
	City    string
	Country string
}

type MapData struct {
	Locations   []MapLocation
	Jobs        []MapJob
	Assignments []JobLocation
	Places      []JobPlace
}

func (s *Store) MapData(ctx context.Context) (MapData, error) {
	var data MapData

	locations, err := s.mapLocations(ctx)
	if err != nil {
		return data, err
	}
	data.Locations = locations

	jobs, assignments, err := s.mapJobs(ctx)
	if err != nil {
		return data, err
	}
	data.Jobs = jobs
	data.Assignments = assignments

	jobIDs := make([]int64, 0, len(jobs))
	seen := make(map[int64]bool, len(jobs))
	for _, job := range jobs {
		if seen[job.ID] {
			continue
		}
		seen[job.ID] = true
		jobIDs = append(jobIDs, job.ID)
	}
	places, err := s.mapPlaces(ctx, jobIDs)
	if err != nil {
		return data, err
	}
	data.Places = places

	return data, nil
}

func (s *Store) mapLocations(ctx context.Context) ([]MapLocation, error) {
	rows, err := s.pool.Query(ctx, `
SELECT l.id, c.id, c.name, l.kind, l.street, l.postal_code, l.city, l.lat, l.lon, l.approximate,
       c.careers_url, c.teamtailor_feed_url IS NOT NULL
FROM locations l
JOIN companies c ON c.id = l.company_id
WHERE l.inactive_at IS NULL
  AND c.inactive_at IS NULL
  AND l.lat IS NOT NULL
  AND l.lon IS NOT NULL
ORDER BY l.id`)
	if err != nil {
		return nil, fmt.Errorf("list map locations: %w", err)
	}
	defer rows.Close()

	var locations []MapLocation
	for rows.Next() {
		var location MapLocation
		if err := rows.Scan(
			&location.ID,
			&location.CompanyID,
			&location.CompanyName,
			&location.Kind,
			&location.Street,
			&location.PostalCode,
			&location.City,
			&location.Lat,
			&location.Lon,
			&location.Approximate,
			&location.CareersURL,
			&location.HasFeed,
		); err != nil {
			return nil, fmt.Errorf("read map location: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list map locations: %w", err)
	}
	return locations, nil
}

func (s *Store) mapJobs(ctx context.Context) ([]MapJob, []JobLocation, error) {
	rows, err := s.pool.Query(ctx, `
SELECT jl.job_id, jl.location_id, j.id, j.title, j.url, j.remote, j.open_application, j.first_seen_at, l.city
FROM job_locations jl
JOIN jobs j ON j.id = jl.job_id
JOIN companies c ON c.id = j.company_id
JOIN locations l ON l.id = jl.location_id
WHERE j.closed_at IS NULL
  AND c.inactive_at IS NULL
ORDER BY j.open_application, j.first_seen_at DESC, j.id, jl.location_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("list map jobs: %w", err)
	}
	defer rows.Close()

	var jobs []MapJob
	var assignments []JobLocation
	for rows.Next() {
		var job MapJob
		var assignment JobLocation
		if err := rows.Scan(
			&assignment.JobID,
			&assignment.LocationID,
			&job.ID,
			&job.Title,
			&job.URL,
			&job.Remote,
			&job.OpenApplication,
			&job.FirstSeenAt,
			&assignment.City,
		); err != nil {
			return nil, nil, fmt.Errorf("read map job: %w", err)
		}
		job.LocationID = assignment.LocationID
		jobs = append(jobs, job)
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("list map jobs: %w", err)
	}
	return jobs, assignments, nil
}

func (s *Store) mapPlaces(ctx context.Context, jobIDs []int64) ([]JobPlace, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
SELECT job_id, city, country
FROM job_places
WHERE job_id = ANY($1)
ORDER BY job_id, city, country`, jobIDs)
	if err != nil {
		return nil, fmt.Errorf("list map job places: %w", err)
	}
	defer rows.Close()

	var places []JobPlace
	for rows.Next() {
		var place JobPlace
		if err := rows.Scan(&place.JobID, &place.City, &place.Country); err != nil {
			return nil, fmt.Errorf("read map job place: %w", err)
		}
		places = append(places, place)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list map job places: %w", err)
	}
	return places, nil
}
