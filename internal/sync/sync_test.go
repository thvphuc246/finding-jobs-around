package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thvphuc246/finding-jobs-around/internal/careers"
	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/store"
	"github.com/thvphuc246/finding-jobs-around/internal/useragent"
)

type fakeGeocoder struct {
	points map[string]geocode.Point
	errs   map[string]error
	calls  []string
}

func (f *fakeGeocoder) Locate(_ context.Context, street, postalCode, city string) (geocode.Point, bool, error) {
	key := street + ", " + postalCode + " " + city
	f.calls = append(f.calls, key)
	if err, ok := f.errs[key]; ok {
		return geocode.Point{}, false, err
	}
	if point, ok := f.points[key]; ok {
		return point, false, nil
	}
	return geocode.Point{Lat: 60.17, Lon: 24.94}, false, nil
}

func TestRunInsertsAndSkipsRepeat(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	deps := testDeps(t, db, geocoder, fixtureRows(t))

	report, err := Run(context.Background(), deps)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if report.Companies != 12 || report.Locations != 19 {
		t.Fatalf("report is %+v, want 12 companies and 19 locations", report)
	}
	if report.Precise != 19 || report.Approximate != 0 || report.NotFound != 0 {
		t.Fatalf("report is %+v, want 19 precise geocodes", report)
	}
	if got := count(t, conn, "SELECT count(*) FROM companies"); got != 12 {
		t.Errorf("companies in database: %d, want 12", got)
	}
	if got := count(t, conn, "SELECT count(*) FROM locations"); got != 19 {
		t.Errorf("locations in database: %d, want 19", got)
	}

	report, err = Run(context.Background(), deps)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.Precise != 0 || report.Approximate != 0 || report.NotFound != 0 {
		t.Fatalf("second run geocoded rows: %+v", report)
	}
	if len(geocoder.calls) != 19 {
		t.Fatalf("geocoder called %d times, want 19", len(geocoder.calls))
	}
	if got := count(t, conn, "SELECT count(*) FROM companies"); got != 12 {
		t.Errorf("companies after second run: %d, want 12", got)
	}
	if got := count(t, conn, "SELECT count(*) FROM locations"); got != 19 {
		t.Errorf("locations after second run: %d, want 19", got)
	}
}

func TestRunDeactivatesMissingMember(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	rows := fixtureRows(t)

	if _, err := Run(context.Background(), testDeps(t, db, geocoder, rows)); err != nil {
		t.Fatalf("first run: %v", err)
	}

	report, err := Run(context.Background(), testDeps(t, db, geocoder, rows[:len(rows)-1]))
	if err != nil {
		t.Fatalf("run without last member: %v", err)
	}
	if report.NewlyInactive != 2 {
		t.Errorf("newly inactive: %d, want 2", report.NewlyInactive)
	}
	if !inactive(t, conn, "companies", 21121) {
		t.Error("company 21121 is still active")
	}
	if !inactive(t, conn, "locations", 21121) {
		t.Error("location 21121 is still active")
	}

	if _, err := Run(context.Background(), testDeps(t, db, geocoder, rows)); err != nil {
		t.Fatalf("run with member restored: %v", err)
	}
	if inactive(t, conn, "companies", 21121) {
		t.Error("company 21121 was not reactivated")
	}
	if inactive(t, conn, "locations", 21121) {
		t.Error("location 21121 was not reactivated")
	}
}

func TestRunRegeocodesChangedStreet(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	rows := []members.Row{{
		ID:             1,
		Title:          "Acme Oy",
		Type:           "päätoimipaikka",
		StreetAddress1: "Vanha 1",
		PostalCode:     "00100",
		City:           "HELSINKI",
	}}

	if _, err := Run(context.Background(), testDeps(t, db, geocoder, rows)); err != nil {
		t.Fatalf("first run: %v", err)
	}

	rows[0].StreetAddress1 = "Uusi 2"
	if _, err := Run(context.Background(), testDeps(t, db, geocoder, rows)); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if len(geocoder.calls) != 2 {
		t.Fatalf("geocoder called %d times, want 2", len(geocoder.calls))
	}
	if geocoder.calls[1] != "Uusi 2, 00100 HELSINKI" {
		t.Errorf("second geocode was %q, want the new address", geocoder.calls[1])
	}
	var lat *float64
	if err := conn.QueryRow(context.Background(), "SELECT lat FROM locations WHERE id = 1").Scan(&lat); err != nil {
		t.Fatalf("read location: %v", err)
	}
	if lat == nil {
		t.Fatal("location has no coordinates")
	}
}

func TestRunDoesNotRetryNotFound(t *testing.T) {
	db, conn := testDatabase(t)
	key := "Vanha 1, 00100 HELSINKI"
	geocoder := &fakeGeocoder{errs: map[string]error{key: geocode.ErrNotFound}}
	rows := []members.Row{{
		ID:             1,
		Title:          "Acme Oy",
		Type:           "päätoimipaikka",
		StreetAddress1: "Vanha 1",
		PostalCode:     "00100",
		City:           "HELSINKI",
	}}

	report, err := Run(context.Background(), testDeps(t, db, geocoder, rows))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if report.NotFound != 1 {
		t.Fatalf("not found: %d, want 1", report.NotFound)
	}
	if len(geocoder.calls) != 1 {
		t.Fatalf("geocoder called %d times, want 1", len(geocoder.calls))
	}

	report, err = Run(context.Background(), testDeps(t, db, geocoder, rows))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.NotFound != 0 || len(geocoder.calls) != 1 {
		t.Fatalf("not-found address was retried: report %+v, %d calls", report, len(geocoder.calls))
	}

	var lat *float64
	var address *string
	if err := conn.QueryRow(context.Background(), "SELECT lat, geocoded_address FROM locations WHERE id = 1").Scan(&lat, &address); err != nil {
		t.Fatalf("read location: %v", err)
	}
	if lat != nil {
		t.Errorf("not-found location has latitude %v", *lat)
	}
	if address == nil || *address != key {
		t.Errorf("geocoded_address is %v, want %q", address, key)
	}
}

func TestRunCareersCSVOverride(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	server, _ := careersSite(t, "https://acme.teamtailor.com/jobs")
	rows := websiteRow(server.URL)

	csv := writeCareers(t, careersRow{id: 1, name: "Acme Oy", url: "https://manual.example/careers"})
	report, err := Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("run with override: %v", err)
	}
	if report.CSVOverrides != 1 || report.DetectedFound != 0 {
		t.Fatalf("report is %+v, want 1 csv override and no detection", report)
	}
	careersURL, source, _ := careersState(t, conn, 1)
	if careersURL == nil || *careersURL != "https://manual.example/careers" {
		t.Errorf("careers_url is %v, want the csv URL", careersURL)
	}
	if source == nil || *source != "csv" {
		t.Errorf("careers_source is %v, want csv", source)
	}

	csv = writeCareers(t)
	report, err = Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("run without override: %v", err)
	}
	if report.CSVOverrides != 0 || report.DetectedFound != 1 {
		t.Fatalf("report is %+v, want no overrides and 1 detection", report)
	}
	careersURL, source, checked := careersState(t, conn, 1)
	if careersURL == nil || *careersURL != "https://acme.teamtailor.com/jobs" {
		t.Errorf("careers_url is %v, want the detected URL", careersURL)
	}
	if source == nil || *source != "detected" {
		t.Errorf("careers_source is %v, want detected", source)
	}
	if checked == nil || *checked != server.URL {
		t.Errorf("careers_checked_website is %v, want %q", checked, server.URL)
	}
}

func TestRunCareersDetectsOnce(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	server, requests := careersSite(t, "https://acme.teamtailor.com/jobs")
	rows := websiteRow(server.URL)
	csv := writeCareers(t)

	report, err := Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if report.DetectedFound != 1 {
		t.Fatalf("first run detected %d, want 1", report.DetectedFound)
	}

	report, err = Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.DetectedFound != 0 || report.DetectedNone != 0 || report.DetectedBlocked != 0 {
		t.Fatalf("second run detected rows again: %+v", report)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("homepage fetched %d times, want 1", got)
	}
	careersURL, source, checked := careersState(t, conn, 1)
	if careersURL == nil || *careersURL != "https://acme.teamtailor.com/jobs" {
		t.Errorf("careers_url is %v, want the detected URL", careersURL)
	}
	if source == nil || *source != "detected" {
		t.Errorf("careers_source is %v, want detected", source)
	}
	if checked == nil || *checked != server.URL {
		t.Errorf("careers_checked_website is %v, want %q", checked, server.URL)
	}
}

func TestRunCareersDetectsChangedWebsite(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	first, firstRequests := careersSite(t, "https://one.teamtailor.com/jobs")
	second, secondRequests := careersSite(t, "https://two.recruitee.com/jobs")
	rows := websiteRow(first.URL)
	csv := writeCareers(t)

	if _, err := Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers())); err != nil {
		t.Fatalf("first run: %v", err)
	}

	website := second.URL
	rows[0].WebsiteURL = &website
	if _, err := Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers())); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if firstRequests.Load() != 1 || secondRequests.Load() != 1 {
		t.Fatalf("homepages fetched %d and %d times, want 1 each", firstRequests.Load(), secondRequests.Load())
	}
	careersURL, _, checked := careersState(t, conn, 1)
	if careersURL == nil || *careersURL != "https://two.recruitee.com/jobs" {
		t.Errorf("careers_url is %v, want the second website's URL", careersURL)
	}
	if checked == nil || *checked != second.URL {
		t.Errorf("careers_checked_website is %v, want %q", checked, second.URL)
	}
}

func TestRunCareersRetriesUnreachable(t *testing.T) {
	db, conn := testDatabase(t)
	geocoder := &fakeGeocoder{}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
			return
		}
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	rows := websiteRow(server.URL)
	csv := writeCareers(t)

	report, err := Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if report.Unreachable != 1 {
		t.Fatalf("first run unreachable: %d, want 1", report.Unreachable)
	}

	report, err = Run(context.Background(), testDepsWithCareers(t, db, geocoder, rows, csv, localCareers()))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.Unreachable != 1 {
		t.Fatalf("second run unreachable: %d, want 1", report.Unreachable)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("homepage fetched %d times, want 2", got)
	}
	careersURL, source, checked := careersState(t, conn, 1)
	if careersURL != nil || source != nil || checked != nil {
		t.Errorf("unreachable site saved careers state: %v %v %v", careersURL, source, checked)
	}
}

func testDeps(t *testing.T, db *store.Store, geocoder Geocoder, rows []members.Row) Deps {
	t.Helper()
	return testDepsWithCareers(t, db, geocoder, rows, writeCareers(t), offlineCareers())
}

func testDepsWithCareers(t *testing.T, db *store.Store, geocoder Geocoder, rows []members.Row, careersPath string, careersClient *careers.Client) Deps {
	t.Helper()
	return Deps{
		Store: db,
		Fetch: func(context.Context) ([]members.Row, error) {
			return rows, nil
		},
		Geocoder:    geocoder,
		Careers:     careersClient,
		CareersPath: careersPath,
		Now:         time.Now(),
		Log:         slog.New(slog.DiscardHandler),
	}
}

func offlineCareers() *careers.Client {
	return &careers.Client{HTTP: &http.Client{Transport: offlineTransport{}}}
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("tests do not call real websites")
}

func localCareers() *careers.Client {
	return &careers.Client{HTTP: useragent.Client(0)}
}

type careersRow struct {
	id   int64
	name string
	url  string
}

func writeCareers(t *testing.T, rows ...careersRow) string {
	t.Helper()
	var builder strings.Builder
	builder.WriteString("company_id,company_name,careers_url\n")
	for _, row := range rows {
		_, _ = fmt.Fprintf(&builder, "%d,%s,%s\n", row.id, row.name, row.url)
	}
	path := filepath.Join(t.TempDir(), "careers.csv")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write careers.csv: %v", err)
	}
	return path
}

func websiteRow(website string) []members.Row {
	return []members.Row{{
		ID:             1,
		Title:          "Acme Oy",
		Type:           "päätoimipaikka",
		StreetAddress1: "Testikatu 1",
		PostalCode:     "00100",
		City:           "HELSINKI",
		WebsiteURL:     &website,
	}}
}

func careersSite(t *testing.T, careersURL string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
		case "/":
			requests.Add(1)
			_, _ = fmt.Fprintf(w, `<a href="%s">Careers</a>`, careersURL)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func careersState(t *testing.T, conn *pgx.Conn, id int64) (url, source, checked *string) {
	t.Helper()
	if err := conn.QueryRow(context.Background(), `
SELECT careers_url, careers_source, careers_checked_website
FROM companies
WHERE id = $1`, id).Scan(&url, &source, &checked); err != nil {
		t.Fatalf("read careers for company %d: %v", id, err)
	}
	return url, source, checked
}

func testDatabase(t *testing.T) (*store.Store, *pgx.Conn) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is not set; set it to a disposable Postgres database")
	}
	ctx := context.Background()

	db, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	if _, err := conn.Exec(ctx, "TRUNCATE companies, locations RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close(context.Background())
		db.Close()
	})
	return db, conn
}

func fixtureRows(t *testing.T) []members.Row {
	t.Helper()
	var rows []members.Row
	for _, name := range []string{"members-page1.json", "members-page2.json"} {
		data, err := os.ReadFile(filepath.Join("..", "members", "testdata", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var page []members.Row
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		rows = append(rows, page...)
	}
	return rows
}

func count(t *testing.T, conn *pgx.Conn, query string) int {
	t.Helper()
	var value int
	if err := conn.QueryRow(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}

func inactive(t *testing.T, conn *pgx.Conn, table string, id int64) bool {
	t.Helper()
	var inactiveAt *time.Time
	if err := conn.QueryRow(context.Background(), "SELECT inactive_at FROM "+table+" WHERE id = $1", id).Scan(&inactiveAt); err != nil {
		t.Fatalf("read %s %d: %v", table, id, err)
	}
	return inactiveAt != nil
}
