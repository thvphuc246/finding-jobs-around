package careers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thvphuc246/finding-jobs-around/internal/useragent"
)

func TestDetectScores(t *testing.T) {
	tests := []struct {
		name string
		page string
		want string
	}{
		{name: "hiring platform", page: "score-four.html", want: "https://acme.teamtailor.com/jobs"},
		{name: "careers subdomain", page: "score-three.html", want: "https://careers.acme.com/"},
		{name: "career path", page: "score-two.html", want: "/en/careers"},
		{name: "link text", page: "score-one.html", want: "/join"},
		{name: "feature configuration", page: "feature-configuration.html"},
		{name: "uratarinat", page: "uratarinat.html"},
		{name: "mailto skipped", page: "mailto.html", want: "/careers"},
		{name: "higher score later", page: "higher-later.html", want: "https://acme.teamtailor.com/jobs"},
		{name: "earlier wins tie", page: "tie.html", want: "/first"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := serveSite(t, test.page, "", http.StatusOK, http.StatusOK)
			result, err := detect(server)
			if err != nil {
				t.Fatalf("Detect returned error: %v", err)
			}
			want := test.want
			if strings.HasPrefix(want, "/") {
				want = server.URL + want
			}
			if want == "" {
				if result.Status != None {
					t.Fatalf("status is %v, want None", result.Status)
				}
				return
			}
			if result.Status != Found {
				t.Fatalf("status is %v, want Found", result.Status)
			}
			if result.URL != want {
				t.Errorf("URL is %q, want %q", result.URL, want)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name         string
		page         string
		robots       string
		robotsStatus int
		pageStatus   int
		want         Status
		wantError    bool
	}{
		{
			name:         "robots disallow",
			page:         "score-four.html",
			robots:       "User-agent: finding-jobs-around/1.0\nDisallow: /\n",
			robotsStatus: http.StatusOK,
			pageStatus:   http.StatusOK,
			want:         Blocked,
		},
		{
			name:         "robots missing allows",
			page:         "score-four.html",
			robotsStatus: http.StatusNotFound,
			pageStatus:   http.StatusOK,
			want:         Found,
		},
		{
			name:         "homepage error",
			page:         "score-four.html",
			robotsStatus: http.StatusOK,
			pageStatus:   http.StatusInternalServerError,
			wantError:    true,
		},
		{
			name:         "no match",
			page:         "no-match.html",
			robotsStatus: http.StatusOK,
			pageStatus:   http.StatusOK,
			want:         None,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := serveSite(t, test.page, test.robots, test.robotsStatus, test.pageStatus)
			result, err := detect(server)
			if test.wantError {
				if err == nil {
					t.Fatal("Detect returned no error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Detect returned error: %v", err)
			}
			if result.Status != test.want {
				t.Fatalf("status is %v, want %v", result.Status, test.want)
			}
			if test.name == "robots missing allows" && result.URL != "https://acme.teamtailor.com/jobs" {
				t.Errorf("URL is %q, want the hiring platform link", result.URL)
			}
		})
	}
}

func TestDetectSendsUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != useragent.Agent {
			t.Errorf("User-Agent is %q, want %q", got, useragent.Agent)
		}
		if r.URL.Path == "/robots.txt" {
			_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
			return
		}
		_, _ = io.WriteString(w, `<a href="/careers">Careers</a>`)
	}))
	defer server.Close()

	if _, err := detect(server); err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
}

func serveSite(t *testing.T, page, robots string, robotsStatus, pageStatus int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(robotsStatus)
			_, _ = io.WriteString(w, robots)
		case "/":
			if pageStatus != http.StatusOK {
				w.WriteHeader(pageStatus)
				return
			}
			body, err := os.ReadFile(filepath.Join("testdata", page))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func detect(server *httptest.Server) (Result, error) {
	return Detect(context.Background(), &Client{HTTP: useragent.Client(0)}, server.URL)
}
