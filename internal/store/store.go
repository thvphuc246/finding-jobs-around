package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thvphuc246/finding-jobs-around/internal/careers"
	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
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
