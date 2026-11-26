package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/thvphuc246/finding-jobs-around/internal/careers"
	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/store"
)

type Report struct {
	Companies       int
	Locations       int
	NewlyInactive   int
	Precise         int
	Approximate     int
	NotFound        int
	CSVOverrides    int
	DetectedFound   int
	DetectedNone    int
	DetectedBlocked int
	Unreachable     int
}

type Geocoder interface {
	Locate(ctx context.Context, street, postalCode, city string) (geocode.Point, bool, error)
}

type Deps struct {
	Store       *store.Store
	Fetch       func(ctx context.Context) ([]members.Row, error)
	Geocoder    Geocoder
	Careers     *careers.Client
	CareersPath string
	Now         time.Time
	Log         *slog.Logger
}

func Run(ctx context.Context, deps Deps) (Report, error) {
	var report Report

	careersPath := deps.CareersPath
	if careersPath == "" {
		careersPath = "careers.csv"
	}
	overrides, err := careers.Load(careersPath)
	if err != nil {
		return report, err
	}

	if err := deps.Store.InitSchema(ctx); err != nil {
		return report, err
	}

	rows, err := deps.Fetch(ctx)
	if err != nil {
		return report, fmt.Errorf("fetch member list: %w", err)
	}

	companies := members.Group(rows)
	report.Companies = len(companies)
	for _, company := range companies {
		report.Locations += len(company.Locations)
	}

	inactive, err := deps.Store.SaveMembers(ctx, companies, deps.Now)
	if err != nil {
		return report, err
	}
	report.NewlyInactive = inactive

	pending, err := deps.Store.PendingGeocode(ctx)
	if err != nil {
		return report, err
	}

	for _, location := range pending {
		point, approximate, err := deps.Geocoder.Locate(ctx, location.Street, location.PostalCode, location.City)
		switch {
		case errors.Is(err, geocode.ErrNotFound):
			if err := deps.Store.SaveGeocode(ctx, location.ID, nil, false, location.AddressKey()); err != nil {
				return report, err
			}
			report.NotFound++
			deps.Log.Warn("location not found", "id", location.ID, "address", location.AddressKey())
		case err != nil:
			return report, fmt.Errorf("geocode location %d: %w", location.ID, err)
		case approximate:
			if err := deps.Store.SaveGeocode(ctx, location.ID, &point, true, location.AddressKey()); err != nil {
				return report, err
			}
			report.Approximate++
		default:
			if err := deps.Store.SaveGeocode(ctx, location.ID, &point, false, location.AddressKey()); err != nil {
				return report, err
			}
			report.Precise++
		}
	}

	applied := make(map[int64]careers.Override, len(overrides))
	known := make(map[int64]bool, len(companies))
	for _, company := range companies {
		known[company.ID] = true
	}
	for id, override := range overrides {
		if !known[id] {
			deps.Log.Warn("careers override for unknown company", "company_id", id)
			continue
		}
		applied[id] = override
	}
	report.CSVOverrides = len(applied)
	if err := deps.Store.ApplyCareersOverrides(ctx, applied); err != nil {
		return report, err
	}

	pendingCareers, err := deps.Store.PendingCareers(ctx)
	if err != nil {
		return report, err
	}
	for _, company := range pendingCareers {
		result, err := careers.Detect(ctx, deps.Careers, company.WebsiteURL)
		if err != nil {
			report.Unreachable++
			deps.Log.Warn("careers detection failed", "company_id", company.ID, "website", company.WebsiteURL, "error", err)
			continue
		}

		var careersURL *string
		switch result.Status {
		case careers.Found:
			careersURL = &result.URL
			report.DetectedFound++
		case careers.None:
			report.DetectedNone++
		case careers.Blocked:
			report.DetectedBlocked++
		}
		if err := deps.Store.SaveDetectedCareers(ctx, company.ID, careersURL, company.WebsiteURL); err != nil {
			return report, err
		}
	}

	return report, nil
}
