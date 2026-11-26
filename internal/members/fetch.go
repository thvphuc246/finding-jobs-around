package members

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const perPage = 500

type Row struct {
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	Type           string  `json:"type"`
	StreetAddress1 string  `json:"street_address_1"`
	StreetAddress2 string  `json:"street_address_2"`
	PostalCode     string  `json:"postal_code"`
	City           string  `json:"city"`
	WebsiteURL     *string `json:"website_url"`
}

func Fetch(ctx context.Context, client *http.Client, baseURL string) ([]Row, error) {
	var rows []Row
	pages := 1
	for page := 1; page <= pages; page++ {
		pageRows, total, err := fetchPage(ctx, client, baseURL, page)
		if err != nil {
			return nil, err
		}
		pages = total
		rows = append(rows, pageRows...)
	}
	return rows, nil
}

func fetchPage(ctx context.Context, client *http.Client, baseURL string, page int) ([]Row, int, error) {
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, 0, fmt.Errorf("parse member list URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build member list page %d request: %w", page, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch member list page %d: %w", page, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("member list page %d returned HTTP %d", page, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read member list page %d: %w", page, err)
	}
	var rows []Row
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, 0, fmt.Errorf("decode member list page %d: %w", page, err)
	}
	return rows, pageCount(resp.Header), nil
}

func pageCount(header http.Header) int {
	pages, err := strconv.Atoi(header.Get("x-wp-totalpages"))
	if err != nil || pages < 1 {
		return 1
	}
	return pages
}
