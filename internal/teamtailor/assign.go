package teamtailor

import "strings"

const headOfficeKind = "head_office"

type Location struct {
	ID   int64
	Kind string
	City string
}

func Assign(job Job, locations []Location) []int64 {
	var matched []int64
	for _, location := range locations {
		if matchesPlace(location.City, job.Places) {
			matched = append(matched, location.ID)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	if allAbroad(job.Places) {
		return nil
	}
	for _, location := range locations {
		if location.Kind == headOfficeKind {
			return []int64{location.ID}
		}
	}
	return nil
}

func matchesPlace(city string, places []Place) bool {
	for _, place := range places {
		if place.City == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(city), strings.TrimSpace(place.City)) {
			return true
		}
	}
	return false
}

func allAbroad(places []Place) bool {
	if len(places) == 0 {
		return false
	}
	for _, place := range places {
		country := strings.TrimSpace(place.Country)
		if country == "" || strings.EqualFold(country, "Finland") || strings.EqualFold(country, "Suomi") {
			return false
		}
	}
	return true
}
