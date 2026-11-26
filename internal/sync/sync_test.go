package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thvphuc246/finding-jobs-around/internal/geocode"
	"github.com/thvphuc246/finding-jobs-around/internal/members"
	"github.com/thvphuc246/finding-jobs-around/internal/store"
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
	deps := testDeps(db, geocoder, fixtureRows(t))

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

	if _, err := Run(context.Background(), testDeps(db, geocoder, rows)); err != nil {
		t.Fatalf("first run: %v", err)
	}

	report, err := Run(context.Background(), testDeps(db, geocoder, rows[:len(rows)-1]))
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

	if _, err := Run(context.Background(), testDeps(db, geocoder, rows)); err != nil {
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

	if _, err := Run(context.Background(), testDeps(db, geocoder, rows)); err != nil {
		t.Fatalf("first run: %v", err)
	}

	rows[0].StreetAddress1 = "Uusi 2"
	if _, err := Run(context.Background(), testDeps(db, geocoder, rows)); err != nil {
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

	report, err := Run(context.Background(), testDeps(db, geocoder, rows))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if report.NotFound != 1 {
		t.Fatalf("not found: %d, want 1", report.NotFound)
	}
	if len(geocoder.calls) != 1 {
		t.Fatalf("geocoder called %d times, want 1", len(geocoder.calls))
	}

	report, err = Run(context.Background(), testDeps(db, geocoder, rows))
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

func testDeps(db *store.Store, geocoder Geocoder, rows []members.Row) Deps {
	return Deps{
		Store: db,
		Fetch: func(context.Context) ([]members.Row, error) {
			return rows, nil
		},
		Geocoder: geocoder,
		Now:      time.Now(),
		Log:      slog.New(slog.DiscardHandler),
	}
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
