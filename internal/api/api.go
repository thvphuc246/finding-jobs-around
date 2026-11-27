package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/thvphuc246/finding-jobs-around/internal/store"
)

const (
	StatusHiring              = "hiring"
	StatusOpenApplicationOnly = "open_application_only"
	StatusNoOpenings          = "no_openings"
	StatusUnknown             = "unknown"

	newWindow = 7 * 24 * time.Hour
)

type Place struct {
	City       string `json:"city"`
	LocationID *int64 `json:"locationId"`
}

type Job struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	URL             string  `json:"url"`
	IsNew           bool    `json:"isNew"`
	OpenApplication bool    `json:"openApplication"`
	Remote          string  `json:"remote"`
	OtherPlaces     []Place `json:"otherPlaces"`
}

type Location struct {
	ID          int64   `json:"id"`
	CompanyID   int64   `json:"companyId"`
	CompanyName string  `json:"companyName"`
	Kind        string  `json:"kind"`
	Street      string  `json:"street"`
	PostalCode  string  `json:"postalCode"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Approximate bool    `json:"approximate"`
	CareersURL  *string `json:"careersUrl"`
	Status      string  `json:"status"`
	Jobs        []Job   `json:"jobs"`
}

type Response struct {
	Locations []Location `json:"locations"`
}

func Status(hasFeed bool, jobs []Job) string {
	if !hasFeed {
		return StatusUnknown
	}
	if len(jobs) == 0 {
		return StatusNoOpenings
	}
	for _, job := range jobs {
		if !job.OpenApplication {
			return StatusHiring
		}
	}
	return StatusOpenApplicationOnly
}

func Handler(db *store.Store, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/locations", func(w http.ResponseWriter, r *http.Request) {
		data, err := db.MapData(r.Context())
		if err != nil {
			slog.Error("load map locations", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load locations"})
			return
		}
		writeJSON(w, http.StatusOK, Response{Locations: build(data, now())})
	})
	return mux
}

func build(data store.MapData, now time.Time) []Location {
	cities := make(map[int64]string, len(data.Locations))
	for _, location := range data.Locations {
		cities[location.ID] = location.City
	}

	assignments := make(map[int64][]store.JobLocation)
	for _, assignment := range data.Assignments {
		assignments[assignment.JobID] = append(assignments[assignment.JobID], assignment)
	}

	places := make(map[int64][]store.JobPlace)
	for _, place := range data.Places {
		places[place.JobID] = append(places[place.JobID], place)
	}

	jobsByLocation := make(map[int64][]Job)
	for _, job := range data.Jobs {
		city, ok := cities[job.LocationID]
		if !ok {
			continue
		}
		other := make([]Place, 0, len(places[job.ID]))
		seen := make(map[string]bool, len(places[job.ID]))
		for _, place := range places[job.ID] {
			placeCity := strings.TrimSpace(place.City)
			key := strings.ToLower(placeCity)
			if strings.EqualFold(placeCity, strings.TrimSpace(city)) || seen[key] {
				continue
			}
			seen[key] = true
			other = append(other, Place{
				City:       place.City,
				LocationID: locationInCity(assignments[job.ID], place.City),
			})
		}
		jobsByLocation[job.LocationID] = append(jobsByLocation[job.LocationID], Job{
			ID:              job.ID,
			Title:           job.Title,
			URL:             job.URL,
			IsNew:           job.FirstSeenAt.After(now.Add(-newWindow)),
			OpenApplication: job.OpenApplication,
			Remote:          job.Remote,
			OtherPlaces:     other,
		})
	}

	locations := make([]Location, 0, len(data.Locations))
	for _, location := range data.Locations {
		jobs := jobsByLocation[location.ID]
		if jobs == nil {
			jobs = []Job{}
		}
		locations = append(locations, Location{
			ID:          location.ID,
			CompanyID:   location.CompanyID,
			CompanyName: location.CompanyName,
			Kind:        location.Kind,
			Street:      location.Street,
			PostalCode:  location.PostalCode,
			City:        location.City,
			Lat:         location.Lat,
			Lon:         location.Lon,
			Approximate: location.Approximate,
			CareersURL:  location.CareersURL,
			Status:      Status(location.HasFeed, jobs),
			Jobs:        jobs,
		})
	}
	return locations
}

func locationInCity(assignments []store.JobLocation, city string) *int64 {
	for _, assignment := range assignments {
		if strings.EqualFold(strings.TrimSpace(assignment.City), strings.TrimSpace(city)) {
			id := assignment.LocationID
			return &id
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusOK {
		w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600")
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write json response", "error", err)
	}
}
