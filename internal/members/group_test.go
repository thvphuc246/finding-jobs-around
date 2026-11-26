package members

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGroupFixture(t *testing.T) {
	companies := Group(readFixtures(t))

	if len(companies) != 12 {
		t.Fatalf("got %d companies, want 12", len(companies))
	}

	locations := 0
	byID := make(map[int64]Company, len(companies))
	for _, company := range companies {
		byID[company.ID] = company
		locations += len(company.Locations)
	}
	if locations != 19 {
		t.Fatalf("got %d locations, want 19", locations)
	}

	if got := len(byID[17509].Locations); got != 4 {
		t.Errorf("Advania Finland Oy has %d locations, want 4", got)
	}
	if got := len(byID[17879].Locations); got != 5 {
		t.Errorf("Evitec Oy has %d locations, want 5", got)
	}
	if got := len(byID[18047].Locations); got != 1 {
		t.Errorf("Silo AI Oy has %d locations, want 1", got)
	}

	orphans := []int64{22751, 22755, 22753, 21223, 21227, 21229, 21121}
	for _, id := range orphans {
		company, ok := byID[id]
		if !ok {
			t.Errorf("orphan row %d did not become a company", id)
			continue
		}
		if len(company.Locations) != 1 {
			t.Errorf("orphan company %d has %d locations, want 1", id, len(company.Locations))
			continue
		}
		if company.Locations[0].Kind != KindHeadOffice {
			t.Errorf("orphan company %d location kind is %q, want %q", id, company.Locations[0].Kind, KindHeadOffice)
		}
	}
}

func TestGroupLinksBranch(t *testing.T) {
	tests := []struct {
		name   string
		rows   []Row
		branch int64
		head   int64
	}{
		{
			name: "exact prefix",
			rows: []Row{
				headRow(1, "Druid Oy"),
				branch(2, "Druid Oy Helsinki"),
			},
			branch: 2,
			head:   1,
		},
		{
			name: "comma title",
			rows: []Row{
				headRow(1, "Evitec Oy"),
				branch(2, "Evitec Oy, Lahti"),
			},
			branch: 2,
			head:   1,
		},
		{
			name: "legal suffix fallback",
			rows: []Row{
				headRow(1, "Advania Finland Oy"),
				branch(2, "Advania Finland Kuopio"),
			},
			branch: 2,
			head:   1,
		},
		{
			name: "longest prefix wins",
			rows: []Row{
				headRow(1, "Acme Oy"),
				headRow(2, "Acme Oy Helsinki"),
				branch(3, "Acme Oy Helsinki Espoo"),
			},
			branch: 3,
			head:   2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			company, ok := companyOf(Group(test.rows), test.branch)
			if !ok {
				t.Fatalf("branch %d is in no company", test.branch)
			}
			if company.ID != test.head {
				t.Fatalf("branch %d belongs to company %d, want %d", test.branch, company.ID, test.head)
			}
			for _, location := range company.Locations {
				if location.ID == test.branch && location.Kind != KindBranch {
					t.Fatalf("branch %d kind is %q, want %q", test.branch, location.Kind, KindBranch)
				}
			}
		})
	}
}

func TestGroupOrphanBecomesCompany(t *testing.T) {
	companies := Group([]Row{branch(3, "CoreHW Oy Helsinki")})
	if len(companies) != 1 {
		t.Fatalf("got %d companies, want 1", len(companies))
	}
	company := companies[0]
	if company.ID != 3 {
		t.Fatalf("company id is %d, want 3", company.ID)
	}
	if len(company.Locations) != 1 || company.Locations[0].Kind != KindHeadOffice {
		t.Fatalf("orphan location is not a head office: %+v", company.Locations)
	}
}

func headRow(id int64, title string) Row {
	return Row{
		ID:             id,
		Title:          title,
		Type:           headOfficeType,
		StreetAddress1: "Testikatu 1",
		PostalCode:     "00100",
		City:           "HELSINKI",
	}
}

func branch(id int64, title string) Row {
	row := headRow(id, title)
	row.Type = "toimipaikka"
	return row
}

func companyOf(companies []Company, locationID int64) (Company, bool) {
	for _, company := range companies {
		for _, location := range company.Locations {
			if location.ID == locationID {
				return company, true
			}
		}
	}
	return Company{}, false
}

func readFixtures(t *testing.T) []Row {
	t.Helper()
	var rows []Row
	for _, name := range []string{"members-page1.json", "members-page2.json"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var page []Row
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		rows = append(rows, page...)
	}
	return rows
}
