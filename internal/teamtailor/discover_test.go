package teamtailor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

const validFeed = `<?xml version="1.0"?><rss version="2.0" xmlns:tt="https://teamtailor.com/locations"><channel><item><title>Engineer</title><link>https://acme.teamtailor.com/jobs/1</link><guid>g1</guid><tt:locations><tt:location><tt:name>Helsinki</tt:name><tt:city>Helsinki</tt:city><tt:country>Finland</tt:country></tt:location></tt:locations></item></channel></rss>`

const allowRobots = "User-agent: *\nAllow: /\n"

func TestDiscover(t *testing.T) {
	cdnPage := response{body: `<html><body><script src="https://teamtailor-cdn.com/app.js"></script></body></html>`}
	widgetPage := response{body: `<html><body><script src="https://teamtailor-cdn.com/app.js"></script><div class="teamtailor-jobs-widget" data-teamtailor-api-key="x"></div></body></html>`}
	linkPage := response{body: `<html><body><a href="https://acme.teamtailor.com/openings">Jobs</a></body></html>`}

	tests := []struct {
		name         string
		careersURL   func(*httptest.Server) string
		routes       map[string]response
		wantFound    bool
		wantFeed     func(*httptest.Server) string
		wantError    bool
		notRequested []string
	}{
		{
			name:       "teamtailor host",
			careersURL: func(*httptest.Server) string { return "https://acme.teamtailor.com/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/jobs.rss":   {body: validFeed},
			},
			wantFound:    true,
			wantFeed:     func(*httptest.Server) string { return "https://acme.teamtailor.com/jobs.rss" },
			notRequested: []string{"/careers"},
		},
		{
			name:       "custom domain with the CDN marker",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    cdnPage,
				"/jobs.rss":   {body: validFeed},
			},
			wantFound: true,
			wantFeed:  func(server *httptest.Server) string { return server.URL + "/jobs.rss" },
		},
		{
			name:       "widget page is not a Teamtailor site",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    widgetPage,
				"/jobs.rss":   {body: validFeed},
			},
			notRequested: []string{"/jobs.rss"},
		},
		{
			name:       "link to a Teamtailor site",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    linkPage,
				"/jobs.rss":   {body: validFeed},
			},
			wantFound: true,
			wantFeed:  func(*httptest.Server) string { return "https://acme.teamtailor.com/jobs.rss" },
		},
		{
			name:       "candidate serving HTML is skipped",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    cdnPage,
				"/jobs.rss":   {body: `<html><body>Not a feed</body></html>`},
			},
		},
		{
			name:       "candidate returning 404 is skipped",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    cdnPage,
				"/jobs.rss":   {status: http.StatusNotFound},
			},
		},
		{
			name:       "plain page without candidates",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    {body: `<html><body><a href="/about">About</a></body></html>`},
				"/jobs.rss":   {body: validFeed},
			},
			notRequested: []string{"/jobs.rss"},
		},
		{
			name:       "robots disallow",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: "User-agent: finding-jobs-around/1.0\nDisallow: /\n"},
				"/careers":    cdnPage,
				"/jobs.rss":   {body: validFeed},
			},
			notRequested: []string{"/careers", "/jobs.rss"},
		},
		{
			name:       "careers page error",
			careersURL: func(server *httptest.Server) string { return server.URL + "/careers" },
			routes: map[string]response{
				"/robots.txt": {body: allowRobots},
				"/careers":    {status: http.StatusInternalServerError},
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, hits := serveSite(t, test.routes)
			client := clientFor(t, server)

			feedURL, found, err := Discover(context.Background(), client, test.careersURL(server))
			if test.wantError {
				if err == nil {
					t.Fatal("Discover returned no error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Discover returned error: %v", err)
			}
			if found != test.wantFound {
				t.Fatalf("found is %v, want %v", found, test.wantFound)
			}
			if test.wantFeed != nil {
				if want := test.wantFeed(server); feedURL != want {
					t.Errorf("feed URL is %q, want %q", feedURL, want)
				}
			}
			for _, path := range test.notRequested {
				if got := hits.get(path); got != 0 {
					t.Errorf("%s was requested %d times", path, got)
				}
			}
		})
	}
}

func TestFetch(t *testing.T) {
	server, _ := serveSite(t, map[string]response{
		"/jobs.rss": {body: validFeed},
	})
	jobs, err := clientFor(t, server).Fetch(context.Background(), server.URL+"/jobs.rss")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Engineer" {
		t.Fatalf("Fetch returned %+v", jobs)
	}
}

func TestFetchRejectsErrorStatus(t *testing.T) {
	server, _ := serveSite(t, map[string]response{
		"/jobs.rss": {status: http.StatusInternalServerError},
	})
	if _, err := clientFor(t, server).Fetch(context.Background(), server.URL+"/jobs.rss"); err == nil {
		t.Fatal("Fetch accepted an error status")
	}
}

type response struct {
	status int
	body   string
}

type hits struct {
	mu    sync.Mutex
	count map[string]int
}

func (h *hits) add(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count[path]++
}

func (h *hits) get(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count[path]
}

func serveSite(t *testing.T, routes map[string]response) (*httptest.Server, *hits) {
	t.Helper()
	counted := &hits{count: make(map[string]int)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counted.add(r.URL.Path)
		route, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		status := route.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, route.body)
	}))
	t.Cleanup(server.Close)
	return server, counted
}

func clientFor(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return &Client{HTTP: &http.Client{Transport: rewriteTransport{target: target}}}
}

type rewriteTransport struct {
	target *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
