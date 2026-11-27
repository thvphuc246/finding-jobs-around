package api

import "testing"

func TestStatus(t *testing.T) {
	tests := []struct {
		name    string
		hasFeed bool
		jobs    []Job
		want    string
	}{
		{name: "no feed", hasFeed: false, jobs: nil, want: StatusUnknown},
		{name: "no feed with jobs", hasFeed: false, jobs: []Job{{}}, want: StatusUnknown},
		{name: "feed but no jobs here", hasFeed: true, jobs: nil, want: StatusNoOpenings},
		{name: "hiring", hasFeed: true, jobs: []Job{{OpenApplication: false}}, want: StatusHiring},
		{name: "only open application", hasFeed: true, jobs: []Job{{OpenApplication: true}}, want: StatusOpenApplicationOnly},
		{name: "hiring among open applications", hasFeed: true, jobs: []Job{{OpenApplication: true}, {OpenApplication: false}}, want: StatusHiring},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Status(test.hasFeed, test.jobs); got != test.want {
				t.Errorf("Status(%v, %d jobs) = %q, want %q", test.hasFeed, len(test.jobs), got, test.want)
			}
		})
	}
}
