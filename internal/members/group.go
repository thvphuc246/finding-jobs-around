package members

import (
	"strings"
)

const (
	KindHeadOffice = "head_office"
	KindBranch     = "branch"

	headOfficeType = "päätoimipaikka"
)

var legalSuffixes = map[string]bool{
	"oy":  true,
	"oyj": true,
	"ab":  true,
	"ltd": true,
}

type Company struct {
	ID         int64
	Name       string
	WebsiteURL *string
	Locations  []Location
}

type Location struct {
	ID         int64
	Kind       string
	Street     string
	PostalCode string
	City       string
}

type headOffice struct {
	row        Row
	normalised string
	stripped   string
}

func Group(rows []Row) []Company {
	var companies []Company
	index := make(map[int64]int)
	var heads []headOffice

	for _, row := range rows {
		if row.Type != headOfficeType {
			continue
		}
		normalised := normalise(row.Title)
		heads = append(heads, headOffice{
			row:        row,
			normalised: normalised,
			stripped:   stripSuffix(normalised),
		})
		index[row.ID] = len(companies)
		companies = append(companies, Company{
			ID:         row.ID,
			Name:       row.Title,
			WebsiteURL: row.WebsiteURL,
			Locations:  []Location{location(row, KindHeadOffice)},
		})
	}

	for _, row := range rows {
		if row.Type == headOfficeType {
			continue
		}
		if head, ok := match(heads, row.Title); ok {
			i := index[head.ID]
			companies[i].Locations = append(companies[i].Locations, location(row, KindBranch))
			continue
		}
		index[row.ID] = len(companies)
		companies = append(companies, Company{
			ID:         row.ID,
			Name:       row.Title,
			WebsiteURL: row.WebsiteURL,
			Locations:  []Location{location(row, KindHeadOffice)},
		})
	}

	return companies
}

func match(heads []headOffice, title string) (Row, bool) {
	normalised := normalise(title)
	if head, ok := longestPrefix(heads, normalised, false); ok {
		return head, true
	}
	return longestPrefix(heads, normalised, true)
}

func longestPrefix(heads []headOffice, title string, stripped bool) (Row, bool) {
	best := -1
	bestLen := 0
	for i, head := range heads {
		prefix := head.normalised
		if stripped {
			prefix = head.stripped
		}
		if prefix == "" || !strings.HasPrefix(title, prefix+" ") {
			continue
		}
		if best < 0 || len(prefix) > bestLen {
			best = i
			bestLen = len(prefix)
		}
	}
	if best < 0 {
		return Row{}, false
	}
	return heads[best].row, true
}

func normalise(title string) string {
	replaced := strings.NewReplacer(",", " ", ".", " ").Replace(title)
	return strings.Join(strings.Fields(strings.ToLower(replaced)), " ")
}

func stripSuffix(title string) string {
	fields := strings.Fields(title)
	if len(fields) == 0 {
		return title
	}
	if legalSuffixes[fields[len(fields)-1]] {
		return strings.Join(fields[:len(fields)-1], " ")
	}
	return title
}

func location(row Row, kind string) Location {
	return Location{
		ID:         row.ID,
		Kind:       kind,
		Street:     strings.TrimSpace(row.StreetAddress1 + " " + row.StreetAddress2),
		PostalCode: row.PostalCode,
		City:       row.City,
	}
}
