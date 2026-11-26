package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testStreet = "Testikatu 1"
	testPostal = "00100"
	testCity   = "HELSINKI"
	testKey    = "secret-key"
)

func TestLocate(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		want      Point
		approxi   bool
		wantError bool
	}{
		{
			name: "precise hit",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeFeatures(w, geoFeature(24.94, 60.17, 1, testPostal))
			},
			want: Point{Lat: 60.17, Lon: 24.94},
		},
		{
			name: "low confidence falls back to postal",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Query().Get("text"), testStreet) {
					writeFeatures(w, geoFeature(0, 0, 0.4, testPostal))
					return
				}
				writeFeatures(w, geoFeature(24.95, 60.18, 0.9, testPostal))
			},
			want:    Point{Lat: 60.18, Lon: 24.95},
			approxi: true,
		},
		{
			name: "postal mismatch falls back to city",
			handler: func(w http.ResponseWriter, r *http.Request) {
				text := r.URL.Query().Get("text")
				switch {
				case strings.Contains(text, testStreet):
					writeFeatures(w, geoFeature(0, 0, 0.4, testPostal))
				case strings.Contains(text, testPostal):
					writeFeatures(w, geoFeature(0, 0, 1, "00200"))
				default:
					writeFeatures(w, geoFeature(24.96, 60.19, 0, ""))
				}
			},
			want:    Point{Lat: 60.19, Lon: 24.96},
			approxi: true,
		},
		{
			name: "nothing found",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeFeatures(w)
			},
			wantError: true,
		},
		{
			name: "forbidden",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()

			client := &Client{
				BaseURL: server.URL,
				Key:     testKey,
				HTTP:    server.Client(),
			}
			point, approximate, err := client.Locate(context.Background(), testStreet, testPostal, testCity)

			if test.wantError {
				if err == nil {
					t.Fatal("Locate returned no error")
				}
				if test.name == "nothing found" && !errors.Is(err, ErrNotFound) {
					t.Fatalf("error %q is not ErrNotFound", err)
				}
				if test.name == "forbidden" && errors.Is(err, ErrNotFound) {
					t.Fatal("HTTP 403 was reported as not found")
				}
				return
			}
			if err != nil {
				t.Fatalf("Locate returned error: %v", err)
			}
			if point != test.want {
				t.Errorf("point is %+v, want %+v", point, test.want)
			}
			if approximate != test.approxi {
				t.Errorf("approximate is %v, want %v", approximate, test.approxi)
			}
		})
	}
}

func TestLocateSendsKeyAndShape(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("digitransit-subscription-key"); got != testKey {
			t.Errorf("key header is %q, want %q", got, testKey)
		}
		query := r.URL.Query()
		if got := query.Get("size"); got != "1" {
			t.Errorf("size is %q, want 1", got)
		}
		if got := query.Get("layers"); got != "address" {
			t.Errorf("layers is %q, want address", got)
		}
		writeFeatures(w, geoFeature(24.94, 60.17, 1, testPostal))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Key: testKey, HTTP: server.Client()}
	if _, _, err := client.Locate(context.Background(), testStreet, testPostal, testCity); err != nil {
		t.Fatalf("Locate returned error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("sent %d requests, want 1", requests)
	}
}

func writeFeatures(w http.ResponseWriter, features ...map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"features": features})
}

func geoFeature(lon, lat, confidence float64, postalCode string) map[string]any {
	return map[string]any{
		"geometry": map[string]any{"coordinates": []float64{lon, lat}},
		"properties": map[string]any{
			"confidence": confidence,
			"postalcode": postalCode,
		},
	}
}
