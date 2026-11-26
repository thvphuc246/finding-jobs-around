package members

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFetchReturnsEveryPage(t *testing.T) {
	files := map[string]string{
		"1": "testdata/members-page1.json",
		"2": "testdata/members-page2.json",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := os.ReadFile(files[r.URL.Query().Get("page")])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("x-wp-totalpages", "2")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	rows, err := Fetch(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(rows) != 19 {
		t.Fatalf("got %d rows, want 19", len(rows))
	}
	if rows[0].ID != 20979 || rows[len(rows)-1].ID != 21121 {
		t.Fatalf("unexpected row order: first %d, last %d", rows[0].ID, rows[len(rows)-1].ID)
	}
}

func TestFetchRejectsHTMLBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-wp-totalpages", "1")
		_, _ = w.Write([]byte("<html>fatal error</html>"))
	}))
	defer server.Close()

	_, err := Fetch(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("Fetch accepted an HTML body")
	}
	if !strings.Contains(err.Error(), "page 1") {
		t.Fatalf("error %q does not name page 1", err)
	}
}
