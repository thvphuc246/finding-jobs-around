package careers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var header = []string{"company_id", "company_name", "careers_url"}

type Override struct {
	URL string
}

func Load(path string) (map[int64]Override, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open careers file: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	first, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s line 1: read header: %w", path, err)
	}
	if !sameFields(first, header) {
		return nil, fmt.Errorf("%s line 1: header must be %s", path, strings.Join(header, ","))
	}

	overrides := make(map[int64]Override)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		line, _ := reader.FieldPos(0)
		if len(record) != len(header) {
			return nil, fmt.Errorf("%s line %d: got %d fields, want %d", path, line, len(record), len(header))
		}

		id, err := strconv.ParseInt(record[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: company_id %q is not an integer", path, line, record[0])
		}
		if _, exists := overrides[id]; exists {
			return nil, fmt.Errorf("%s line %d: duplicate company_id %d", path, line, id)
		}

		careersURL := strings.TrimSpace(record[2])
		if careersURL != "" && !absoluteHTTP(careersURL) {
			return nil, fmt.Errorf("%s line %d: careers_url %q is not an absolute http(s) URL", path, line, careersURL)
		}
		overrides[id] = Override{URL: careersURL}
	}

	return overrides, nil
}

func sameFields(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func absoluteHTTP(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
