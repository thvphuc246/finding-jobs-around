package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/thvphuc246/finding-jobs-around/internal/careers"
	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/store"
	syncpkg "github.com/thvphuc246/finding-jobs-around/internal/sync"
	"github.com/thvphuc246/finding-jobs-around/internal/useragent"
)

const (
	memberListURL = "https://teknologiateollisuus.fi/jasenille/wp-admin/admin-ajax.php?action=em_member_feed&association_numbers=394&industries[]=390"
	geocodingURL  = "https://api.digitransit.fi/geocoding/v1/search"
	careersPath   = "careers.csv"
	httpTimeout   = 30 * time.Second
	requestDelay  = time.Second
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("sync failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	apiKey := os.Getenv("DIGITRANSIT_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DIGITRANSIT_API_KEY is not set")
	}

	ctx := context.Background()
	db, err := store.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	httpClient := useragent.Client(httpTimeout)
	geocoder := &geocode.Client{
		BaseURL: geocodingURL,
		Key:     apiKey,
		HTTP:    httpClient,
		Delay:   requestDelay,
	}
	careersClient := &careers.Client{
		HTTP:  httpClient,
		Delay: requestDelay,
	}

	report, err := syncpkg.Run(ctx, syncpkg.Deps{
		Store: db,
		Fetch: func(ctx context.Context) ([]members.Row, error) {
			return members.Fetch(ctx, httpClient, memberListURL)
		},
		Geocoder:    geocoder,
		Careers:     careersClient,
		CareersPath: careersPath,
		Now:         time.Now(),
		Log:         log,
	})
	if err != nil {
		return err
	}

	log.Info("sync finished",
		"companies", report.Companies,
		"locations", report.Locations,
		"newly_inactive", report.NewlyInactive,
		"precise", report.Precise,
		"approximate", report.Approximate,
		"not_found", report.NotFound,
		"csv_overrides", report.CSVOverrides,
		"detected_found", report.DetectedFound,
		"detected_none", report.DetectedNone,
		"detected_blocked", report.DetectedBlocked,
		"unreachable", report.Unreachable,
	)
	return nil
}
