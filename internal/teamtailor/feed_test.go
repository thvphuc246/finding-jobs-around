package teamtailor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "feed.xml"))
	if err != nil {
		t.Fatalf("open feed: %v", err)
	}
	defer func() { _ = file.Close() }()

	jobs, err := Parse(file)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}

	first := jobs[0]
	if first.GUID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("GUID is %q", first.GUID)
	}
	if first.Title != "Senior Backend Developer" {
		t.Errorf("Title is %q", first.Title)
	}
	if first.URL != "https://acme.teamtailor.com/jobs/11111111-1111-1111-1111-111111111111" {
		t.Errorf("URL is %q", first.URL)
	}
	if first.Remote != "hybrid" {
		t.Errorf("Remote is %q", first.Remote)
	}
	if first.OpenApplication {
		t.Error("a normal title is marked open application")
	}
	if first.PublishedAt == nil {
		t.Fatal("PublishedAt is nil")
	}
	wantPublished := time.Date(2026, 11, 24, 10, 0, 0, 0, time.FixedZone("", 2*60*60))
	if !first.PublishedAt.Equal(wantPublished) {
		t.Errorf("PublishedAt is %v, want %v", first.PublishedAt, wantPublished)
	}
	if len(first.Places) != 3 {
		t.Fatalf("got %d places, want 3", len(first.Places))
	}
	if first.Places[0].City != "Helsinki" || first.Places[0].Country != "Finland" {
		t.Errorf("first place is %+v", first.Places[0])
	}
	if first.Places[2].City != "Stockholm" {
		t.Errorf("empty city did not fall back to name: %+v", first.Places[2])
	}

	wantOpen := []bool{false, true, true, false, true, true}
	for i, want := range wantOpen {
		if jobs[i].OpenApplication != want {
			t.Errorf("job %d (%q) OpenApplication is %v, want %v", i, jobs[i].Title, jobs[i].OpenApplication, want)
		}
	}

	if jobs[1].Places[0].City != "Espoo" {
		t.Errorf("empty tt:city did not fall back to tt:name: %+v", jobs[1].Places[0])
	}
	if jobs[2].Title != "Avoin hakemus — Data" {
		t.Errorf("title was not trimmed: %q", jobs[2].Title)
	}
}

func TestParseRejectsNonFeeds(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "HTML page",
			body: `<!doctype html><html><body><a href="/jobs">Jobs</a></body></html>`,
		},
		{
			name: "RSS without the Teamtailor namespace",
			body: `<?xml version="1.0"?><rss version="2.0"><channel><item><title>Engineer</title></item></channel></rss>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.body)); err == nil {
				t.Fatal("Parse accepted a non-Teamtailor document")
			}
		})
	}
}

func TestParseWithoutPubDate(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0" xmlns:tt="https://teamtailor.com/locations"><channel><item><title>Engineer</title><guid>g1</guid></item></channel></rss>`
	jobs, err := Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].PublishedAt != nil {
		t.Errorf("PublishedAt is %v, want nil", jobs[0].PublishedAt)
	}
	if len(jobs[0].Places) != 0 {
		t.Errorf("got %d places, want none", len(jobs[0].Places))
	}
}
