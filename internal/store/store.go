package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thvphuc246/finding-jobs-around/internal/careers"
	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/teamtailor"
)

//go:embed schema.sql
var schema string

type Store struct {
	pool *pgxpool.Pool
}

type PendingLocation struct {
	ID         int64
	Street     string
	PostalCode string
	City       string
}

type PendingCareers struct {
	ID         int64
	WebsiteURL string
}

type PendingFeed struct {
	ID         int64
	CareersURL string
}

type FeedCompany struct {
	ID      int64
	FeedURL string
}

type JobStats struct {
	Open   int
	Opened int
	Closed int
}

func (p PendingLocation) AddressKey() string {
	return p.Street + ", " + p.PostalCode + " " + p.City
}

func Connect(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) InitSchema(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("initialise schema: %w", err)
	}
	return nil
}

func (s *Store) SaveMembers(ctx context.Context, companies []members.Company, now time.Time) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin member save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	companyIDs := make([]int64, 0, len(companies))
	locationIDs := make([]int64, 0, len(companies))

	for _, company := range companies {
		companyIDs = append(companyIDs, company.ID)
		if _, err := tx.Exec(ctx, `
INSERT INTO companies (id, name, website_url, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    website_url = EXCLUDED.website_url,
    updated_at = EXCLUDED.updated_at,
    inactive_at = NULL`,
			company.ID, company.Name, company.WebsiteURL, now); err != nil {
			return 0, fmt.Errorf("save company %d: %w", company.ID, err)
		}

		for _, location := range company.Locations {
			locationIDs = append(locationIDs, location.ID)
			if _, err := tx.Exec(ctx, `
INSERT INTO locations (id, company_id, kind, street, postal_code, city, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE
SET company_id = EXCLUDED.company_id,
    kind = EXCLUDED.kind,
    street = EXCLUDED.street,
    postal_code = EXCLUDED.postal_code,
    city = EXCLUDED.city,
    updated_at = EXCLUDED.updated_at,
    inactive_at = NULL`,
				location.ID, company.ID, location.Kind, location.Street, location.PostalCode, location.City, now); err != nil {
				return 0, fmt.Errorf("save location %d: %w", location.ID, err)
			}
		}
	}

	companyResult, err := tx.Exec(ctx, `
UPDATE companies SET inactive_at = $1
WHERE inactive_at IS NULL AND id <> ALL($2)`, now, companyIDs)
	if err != nil {
		return 0, fmt.Errorf("deactivate companies: %w", err)
	}
	locationResult, err := tx.Exec(ctx, `
UPDATE locations SET inactive_at = $1
WHERE inactive_at IS NULL AND id <> ALL($2)`, now, locationIDs)
	if err != nil {
		return 0, fmt.Errorf("deactivate locations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit member save: %w", err)
	}
	return int(companyResult.RowsAffected() + locationResult.RowsAffected()), nil
}

func (s *Store) PendingGeocode(ctx context.Context) ([]PendingLocation, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, street, postal_code, city
FROM locations
WHERE inactive_at IS NULL
  AND geocoded_address IS DISTINCT FROM street || ', ' || postal_code || ' ' || city
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list pending locations: %w", err)
	}
	defer rows.Close()

	var pending []PendingLocation
	for rows.Next() {
		var location PendingLocation
		if err := rows.Scan(&location.ID, &location.Street, &location.PostalCode, &location.City); err != nil {
			return nil, fmt.Errorf("read pending location: %w", err)
		}
		pending = append(pending, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending locations: %w", err)
	}
	return pending, nil
}

func (s *Store) SaveGeocode(ctx context.Context, id int64, point *geocode.Point, approximate bool, addressKey string) error {
	var lat, lon *float64
	if point != nil {
		lat = &point.Lat
		lon = &point.Lon
	}
	result, err := s.pool.Exec(ctx, `
UPDATE locations
SET lat = $2, lon = $3, approximate = $4, geocoded_address = $5, updated_at = now()
WHERE id = $1`, id, lat, lon, approximate, addressKey)
	if err != nil {
		return fmt.Errorf("save geocode for location %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("save geocode for location %d: no such location", id)
	}
	return nil
}

func (s *Store) ApplyCareersOverrides(ctx context.Context, overrides map[int64]careers.Override) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin careers override: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids := make([]int64, 0, len(overrides))
	for id, override := range overrides {
		ids = append(ids, id)
		var careersURL *string
		if override.URL != "" {
			careersURL = &override.URL
		}
		if _, err := tx.Exec(ctx, `
UPDATE companies
SET careers_url = $2, careers_source = 'csv'
WHERE id = $1`, id, careersURL); err != nil {
			return fmt.Errorf("apply careers override for company %d: %w", id, err)
		}
	}

	if _, err := tx.Exec(ctx, `
UPDATE companies
SET careers_url = NULL, careers_source = NULL, careers_checked_website = NULL
WHERE careers_source = 'csv' AND id <> ALL($1)`, ids); err != nil {
		return fmt.Errorf("clear removed careers overrides: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit careers override: %w", err)
	}
	return nil
}

func (s *Store) PendingCareers(ctx context.Context) ([]PendingCareers, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, website_url
FROM companies
WHERE inactive_at IS NULL
  AND careers_source IS DISTINCT FROM 'csv'
  AND website_url IS NOT NULL
  AND careers_checked_website IS DISTINCT FROM website_url
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list pending careers: %w", err)
	}
	defer rows.Close()

	var pending []PendingCareers
	for rows.Next() {
		var company PendingCareers
		if err := rows.Scan(&company.ID, &company.WebsiteURL); err != nil {
			return nil, fmt.Errorf("read pending careers: %w", err)
		}
		pending = append(pending, company)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending careers: %w", err)
	}
	return pending, nil
}

func (s *Store) SaveDetectedCareers(ctx context.Context, id int64, careersURL *string, checkedWebsite string) error {
	result, err := s.pool.Exec(ctx, `
UPDATE companies
SET careers_url = $2, careers_source = 'detected', careers_checked_website = $3
WHERE id = $1`, id, careersURL, checkedWebsite)
	if err != nil {
		return fmt.Errorf("save careers for company %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("save careers for company %d: no such company", id)
	}
	return nil
}

func (s *Store) PendingFeedDiscovery(ctx context.Context) ([]PendingFeed, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, careers_url
FROM companies
WHERE inactive_at IS NULL
  AND careers_url IS NOT NULL
  AND teamtailor_checked_careers_url IS DISTINCT FROM careers_url
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list pending feed discovery: %w", err)
	}
	defer rows.Close()

	var pending []PendingFeed
	for rows.Next() {
		var company PendingFeed
		if err := rows.Scan(&company.ID, &company.CareersURL); err != nil {
			return nil, fmt.Errorf("read pending feed discovery: %w", err)
		}
		pending = append(pending, company)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending feed discovery: %w", err)
	}
	return pending, nil
}

func (s *Store) SaveFeedDiscovery(ctx context.Context, id int64, feedURL *string, careersURL string) error {
	result, err := s.pool.Exec(ctx, `
UPDATE companies
SET teamtailor_feed_url = $2, teamtailor_checked_careers_url = $3
WHERE id = $1`, id, feedURL, careersURL)
	if err != nil {
		return fmt.Errorf("save feed discovery for company %d: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("save feed discovery for company %d: no such company", id)
	}
	return nil
}

func (s *Store) FeedCompanies(ctx context.Context) ([]FeedCompany, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, teamtailor_feed_url
FROM companies
WHERE inactive_at IS NULL
  AND teamtailor_feed_url IS NOT NULL
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list feed companies: %w", err)
	}
	defer rows.Close()

	var companies []FeedCompany
	for rows.Next() {
		var company FeedCompany
		if err := rows.Scan(&company.ID, &company.FeedURL); err != nil {
			return nil, fmt.Errorf("read feed company: %w", err)
		}
		companies = append(companies, company)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list feed companies: %w", err)
	}
	return companies, nil
}

func (s *Store) SaveJobs(ctx context.Context, companyID int64, jobs []teamtailor.Job, now time.Time) (JobStats, error) {
	stats := JobStats{Open: len(jobs)}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return stats, fmt.Errorf("begin job save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locations, err := activeLocations(ctx, tx, companyID)
	if err != nil {
		return stats, err
	}

	jobIDs := make([]int64, 0, len(jobs))
	guids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		guids = append(guids, job.GUID)
		var publishedAt any
		if job.PublishedAt != nil {
			publishedAt = *job.PublishedAt
		}

		var jobID int64
		var opened bool
		if err := tx.QueryRow(ctx, `
INSERT INTO jobs (company_id, guid, title, url, remote, open_application, published_at, first_seen_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
ON CONFLICT (company_id, guid) DO UPDATE
SET title = EXCLUDED.title,
    url = EXCLUDED.url,
    remote = EXCLUDED.remote,
    open_application = EXCLUDED.open_application,
    published_at = EXCLUDED.published_at,
    last_seen_at = EXCLUDED.last_seen_at,
    closed_at = NULL
RETURNING id, (first_seen_at = last_seen_at)`,
			companyID, job.GUID, job.Title, job.URL, job.Remote, job.OpenApplication, publishedAt, now).Scan(&jobID, &opened); err != nil {
			return stats, fmt.Errorf("save job %s: %w", job.GUID, err)
		}
		if opened {
			stats.Opened++
		}
		jobIDs = append(jobIDs, jobID)
	}

	closed, err := tx.Exec(ctx, `
UPDATE jobs
SET closed_at = $2
WHERE company_id = $1 AND closed_at IS NULL AND guid <> ALL($3)`, companyID, now, guids)
	if err != nil {
		return stats, fmt.Errorf("close vanished jobs: %w", err)
	}
	stats.Closed = int(closed.RowsAffected())

	if _, err := tx.Exec(ctx, `DELETE FROM job_places WHERE job_id = ANY($1)`, jobIDs); err != nil {
		return stats, fmt.Errorf("clear job places: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM job_locations WHERE job_id = ANY($1)`, jobIDs); err != nil {
		return stats, fmt.Errorf("clear job locations: %w", err)
	}

	for i, job := range jobs {
		jobID := jobIDs[i]
		for _, place := range job.Places {
			if _, err := tx.Exec(ctx, `
INSERT INTO job_places (job_id, city, country)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`, jobID, place.City, place.Country); err != nil {
				return stats, fmt.Errorf("save place for job %s: %w", job.GUID, err)
			}
		}
		for _, locationID := range teamtailor.Assign(job, locations) {
			if _, err := tx.Exec(ctx, `
INSERT INTO job_locations (job_id, location_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING`, jobID, locationID); err != nil {
				return stats, fmt.Errorf("assign job %s to location %d: %w", job.GUID, locationID, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return stats, fmt.Errorf("commit job save: %w", err)
	}
	return stats, nil
}

func activeLocations(ctx context.Context, tx pgx.Tx, companyID int64) ([]teamtailor.Location, error) {
	rows, err := tx.Query(ctx, `
SELECT id, kind, city
FROM locations
WHERE company_id = $1 AND inactive_at IS NULL
ORDER BY id`, companyID)
	if err != nil {
		return nil, fmt.Errorf("list locations for company %d: %w", companyID, err)
	}
	defer rows.Close()

	var locations []teamtailor.Location
	for rows.Next() {
		var location teamtailor.Location
		if err := rows.Scan(&location.ID, &location.Kind, &location.City); err != nil {
			return nil, fmt.Errorf("read location for company %d: %w", companyID, err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list locations for company %d: %w", companyID, err)
	}
	return locations, nil
}
