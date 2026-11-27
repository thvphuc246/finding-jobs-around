package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/store"
	"github.com/thvphuc246/finding-jobs-around/internal/teamtailor"
)

var fixedNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

func TestHandlerLocations(t *testing.T) {
	db, _ := testDatabase(t)
	seed(t, db, fixedNow)

	recorder := get(t, Handler(db, func() time.Time { return fixedNow }))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=300, s-maxage=3600" {
		t.Errorf("Cache-Control = %q", got)
	}

	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Locations) != 3 {
		t.Fatalf("locations = %d, want 3", len(response.Locations))
	}

	byID := make(map[int64]Location, len(response.Locations))
	for _, location := range response.Locations {
		byID[location.ID] = location
	}
	if _, ok := byID[30]; ok {
		t.Error("location of an inactive company is present")
	}
	if _, ok := byID[12]; ok {
		t.Error("location without coordinates is present")
	}

	helsinki := byID[10]
	if helsinki.Status != StatusHiring {
		t.Errorf("Helsinki status = %q, want %q", helsinki.Status, StatusHiring)
	}
	if helsinki.CareersURL == nil || *helsinki.CareersURL != "https://alpha.example/careers" {
		t.Errorf("Helsinki careers URL = %v", helsinki.CareersURL)
	}
	if helsinki.CompanyName != "Alpha Oy" || helsinki.Kind != members.KindHeadOffice {
		t.Errorf("Helsinki company = %q, kind = %q", helsinki.CompanyName, helsinki.Kind)
	}

	wantTitles := []string{"New job", "Boundary job", "Old job", "Open application"}
	if len(helsinki.Jobs) != len(wantTitles) {
		t.Fatalf("Helsinki jobs = %d, want %d", len(helsinki.Jobs), len(wantTitles))
	}
	for i, want := range wantTitles {
		if helsinki.Jobs[i].Title != want {
			t.Errorf("Helsinki job %d = %q, want %q", i, helsinki.Jobs[i].Title, want)
		}
	}

	if !helsinki.Jobs[0].IsNew {
		t.Error("job seen two days ago is not new")
	}
	if helsinki.Jobs[1].IsNew {
		t.Error("job seen exactly seven days ago is new")
	}
	if helsinki.Jobs[2].IsNew {
		t.Error("job seen ten days ago is new")
	}
	if !helsinki.Jobs[3].OpenApplication {
		t.Error("open application job is not marked")
	}
	if len(helsinki.Jobs[0].OtherPlaces) != 0 {
		t.Errorf("single-city job other places = %v, want none", helsinki.Jobs[0].OtherPlaces)
	}

	oldPlaces := make(map[string]*int64)
	for _, place := range helsinki.Jobs[2].OtherPlaces {
		oldPlaces[place.City] = place.LocationID
	}
	if len(oldPlaces) != 2 {
		t.Fatalf("old job other places = %v, want Stockholm and Tampere", oldPlaces)
	}
	if id, ok := oldPlaces["Tampere"]; !ok || id == nil || *id != 11 {
		t.Errorf("Tampere other place = %v, want location 11", oldPlaces["Tampere"])
	}
	if id, ok := oldPlaces["Stockholm"]; !ok || id != nil {
		t.Errorf("Stockholm other place = %v, want no location", oldPlaces["Stockholm"])
	}

	tampere := byID[11]
	if tampere.Status != StatusHiring {
		t.Errorf("Tampere status = %q, want %q", tampere.Status, StatusHiring)
	}
	if len(tampere.Jobs) != 1 || tampere.Jobs[0].Title != "Old job" {
		t.Fatalf("Tampere jobs = %+v, want the old job", tampere.Jobs)
	}
	if len(tampere.Jobs[0].OtherPlaces) != 2 {
		t.Fatalf("Tampere other places = %v, want Helsinki and Stockholm", tampere.Jobs[0].OtherPlaces)
	}

	espoo := byID[20]
	if espoo.Status != StatusUnknown {
		t.Errorf("Espoo status = %q, want %q", espoo.Status, StatusUnknown)
	}
	if espoo.CareersURL != nil {
		t.Errorf("Espoo careers URL = %v, want null", espoo.CareersURL)
	}
	if espoo.Jobs == nil {
		t.Error("Espoo jobs is null, want an empty list")
	}
	if len(espoo.Jobs) != 0 {
		t.Errorf("Espoo jobs = %v, want none", espoo.Jobs)
	}
}

func TestHandlerStoreFailure(t *testing.T) {
	db, _ := testDatabase(t)
	db.Close()

	recorder := get(t, Handler(db, time.Now))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] == "" {
		t.Errorf("error body = %v, want an error message", body)
	}
}

func get(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/locations", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func testDatabase(t *testing.T) (*store.Store, *pgx.Conn) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is not set; set it to a disposable Postgres database")
	}
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(873421)"); err != nil {
		t.Fatalf("lock test database: %v", err)
	}

	db, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	if _, err := conn.Exec(ctx, "TRUNCATE companies, locations RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		_ = conn.Close(context.Background())
	})
	return db, conn
}

func seed(t *testing.T, db *store.Store, now time.Time) {
	t.Helper()
	ctx := context.Background()

	alphaWebsite := "https://alpha.example"
	betaWebsite := "https://beta.example"
	gammaWebsite := "https://gamma.example"
	alphaCareers := "https://alpha.example/careers"
	alphaFeed := "https://alpha.example/jobs.rss"

	companies := []members.Company{
		{
			ID:         1,
			Name:       "Alpha Oy",
			WebsiteURL: &alphaWebsite,
			Locations: []members.Location{
				{ID: 10, Kind: members.KindHeadOffice, Street: "Alpha Street 1", PostalCode: "00100", City: "Helsinki"},
				{ID: 11, Kind: members.KindBranch, Street: "Alpha Street 2", PostalCode: "33100", City: "Tampere"},
				{ID: 12, Kind: members.KindBranch, Street: "Alpha Street 3", PostalCode: "20100", City: "Turku"},
			},
		},
		{
			ID:         2,
			Name:       "Beta Oy",
			WebsiteURL: &betaWebsite,
			Locations: []members.Location{
				{ID: 20, Kind: members.KindHeadOffice, Street: "Beta Street 1", PostalCode: "02100", City: "Espoo"},
			},
		},
		{
			ID:         3,
			Name:       "Gamma Oy",
			WebsiteURL: &gammaWebsite,
			Locations: []members.Location{
				{ID: 30, Kind: members.KindHeadOffice, Street: "Gamma Street 1", PostalCode: "01300", City: "Vantaa"},
			},
		},
	}

	if _, err := db.SaveMembers(ctx, companies, now); err != nil {
		t.Fatalf("save members: %v", err)
	}
	if _, err := db.SaveMembers(ctx, companies[:2], now); err != nil {
		t.Fatalf("deactivate Gamma: %v", err)
	}

	points := map[int64]geocode.Point{
		10: {Lat: 60.17, Lon: 24.94},
		11: {Lat: 61.50, Lon: 23.76},
		20: {Lat: 60.20, Lon: 24.66},
		30: {Lat: 60.29, Lon: 25.04},
	}
	for id, point := range points {
		if err := db.SaveGeocode(ctx, id, &point, false, "key"); err != nil {
			t.Fatalf("save geocode %d: %v", id, err)
		}
	}

	if err := db.SaveDetectedCareers(ctx, 1, &alphaCareers, alphaWebsite); err != nil {
		t.Fatalf("save careers: %v", err)
	}
	if err := db.SaveFeedDiscovery(ctx, 1, &alphaFeed, alphaCareers); err != nil {
		t.Fatalf("save feed discovery: %v", err)
	}

	oldJob := teamtailor.Job{
		GUID:  "old",
		Title: "Old job",
		URL:   "https://alpha.example/jobs/old",
		Places: []teamtailor.Place{
			{City: "Helsinki", Country: "Finland"},
			{City: "Tampere", Country: "Finland"},
			{City: "Stockholm", Country: "Sweden"},
		},
	}
	boundaryJob := teamtailor.Job{
		GUID:   "boundary",
		Title:  "Boundary job",
		URL:    "https://alpha.example/jobs/boundary",
		Places: []teamtailor.Place{{City: "Helsinki", Country: "Finland"}},
	}
	newJob := teamtailor.Job{
		GUID:   "new",
		Title:  "New job",
		URL:    "https://alpha.example/jobs/new",
		Places: []teamtailor.Place{{City: "Helsinki", Country: "Finland"}},
	}
	openJob := teamtailor.Job{
		GUID:            "open",
		Title:           "Open application",
		URL:             "https://alpha.example/jobs/open",
		OpenApplication: true,
		Places:          []teamtailor.Place{{City: "Helsinki", Country: "Finland"}},
	}

	saves := []struct {
		at   time.Time
		jobs []teamtailor.Job
	}{
		{at: now.Add(-10 * 24 * time.Hour), jobs: []teamtailor.Job{oldJob}},
		{at: now.Add(-7 * 24 * time.Hour), jobs: []teamtailor.Job{oldJob, boundaryJob}},
		{at: now.Add(-2 * 24 * time.Hour), jobs: []teamtailor.Job{oldJob, boundaryJob, newJob}},
		{at: now.Add(-1 * 24 * time.Hour), jobs: []teamtailor.Job{oldJob, boundaryJob, newJob, openJob}},
	}
	for _, save := range saves {
		if _, err := db.SaveJobs(ctx, 1, save.jobs, save.at); err != nil {
			t.Fatalf("save jobs at %s: %v", save.at, err)
		}
	}
}
