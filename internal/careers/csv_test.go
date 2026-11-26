package careers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	path := writeFile(t, "company_id,company_name,careers_url\n1,Acme Oy,https://acme.fi/careers\n2,Beta Oy,\n")
	overrides, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("got %d overrides, want 2", len(overrides))
	}
	if overrides[1].URL != "https://acme.fi/careers" {
		t.Errorf("override 1 URL is %q", overrides[1].URL)
	}
	if override, ok := overrides[2]; !ok || override.URL != "" {
		t.Errorf("override 2 is %+v, want an empty URL", override)
	}
}

func TestLoadRejectsInvalidFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		line    string
	}{
		{
			name:    "bad header",
			content: "id,name,url\n1,Acme Oy,https://acme.fi\n",
			line:    "line 1",
		},
		{
			name:    "bad id",
			content: "company_id,company_name,careers_url\nabc,Acme Oy,https://acme.fi\n",
			line:    "line 2",
		},
		{
			name:    "duplicate id",
			content: "company_id,company_name,careers_url\n1,Acme Oy,https://acme.fi\n1,Acme Oy,https://acme.fi\n",
			line:    "line 3",
		},
		{
			name:    "relative URL",
			content: "company_id,company_name,careers_url\n1,Acme Oy,/careers\n",
			line:    "line 2",
		},
		{
			name:    "non-http URL",
			content: "company_id,company_name,careers_url\n1,Acme Oy,ftp://acme.fi/careers\n",
			line:    "line 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeFile(t, test.content))
			if err == nil {
				t.Fatal("Load accepted an invalid file")
			}
			if !strings.Contains(err.Error(), test.line) {
				t.Fatalf("error %q does not name %s", err, test.line)
			}
		})
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "careers.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write careers.csv: %v", err)
	}
	return path
}
