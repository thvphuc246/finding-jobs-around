package teamtailor

import "testing"

func TestAssign(t *testing.T) {
	locations := []Location{
		{ID: 1, Kind: "head_office", City: "HELSINKI"},
		{ID: 2, Kind: "branch", City: "TURKU"},
		{ID: 3, Kind: "branch", City: "ESPOO"},
	}

	tests := []struct {
		name   string
		places []Place
		want   []int64
	}{
		{
			name:   "one city against an upper-case location",
			places: []Place{{City: "Helsinki", Country: "Finland"}},
			want:   []int64{1},
		},
		{
			name:   "several cities match several offices",
			places: []Place{{City: "Helsinki", Country: "Finland"}, {City: "Turku", Country: "Finland"}, {City: "Oulu", Country: "Finland"}},
			want:   []int64{1, 2},
		},
		{
			name:   "city comparison ignores case and spaces",
			places: []Place{{City: "  helsinki ", Country: "Finland"}},
			want:   []int64{1},
		},
		{
			name:   "no city match falls back to the head office",
			places: []Place{{City: "Oulu", Country: "Finland"}},
			want:   []int64{1},
		},
		{
			name:   "no places falls back to the head office",
			places: nil,
			want:   []int64{1},
		},
		{
			name:   "Suomi counts as Finland",
			places: []Place{{City: "Oulu", Country: "Suomi"}},
			want:   []int64{1},
		},
		{
			name:   "all places abroad assign nothing",
			places: []Place{{City: "Stockholm", Country: "Sweden"}},
			want:   nil,
		},
		{
			name:   "one Finnish place keeps the head office fallback",
			places: []Place{{City: "Stockholm", Country: "Sweden"}, {City: "Oulu", Country: "Finland"}},
			want:   []int64{1},
		},
		{
			name:   "unknown country keeps the head office fallback",
			places: []Place{{City: "Stockholm", Country: ""}},
			want:   []int64{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Assign(Job{Places: test.places}, locations)
			if len(got) != len(test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("got %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestAssignWithoutHeadOffice(t *testing.T) {
	locations := []Location{{ID: 2, Kind: "branch", City: "TURKU"}}
	if got := Assign(Job{Places: []Place{{City: "Oulu", Country: "Finland"}}}, locations); len(got) != 0 {
		t.Fatalf("got %v, want no locations", got)
	}
}
