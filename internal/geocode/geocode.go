package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var ErrNotFound = errors.New("geocode: address not found")

type Point struct {
	Lat float64
	Lon float64
}

type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
	Delay   time.Duration
}

type feature struct {
	Geometry struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		Confidence float64 `json:"confidence"`
		PostalCode string  `json:"postalcode"`
		Layer      string  `json:"layer"`
	} `json:"properties"`
}

type searchResult struct {
	Features []feature `json:"features"`
}

func (c *Client) Locate(ctx context.Context, street, postalCode, city string) (Point, bool, error) {
	address, found, err := c.search(ctx, street+", "+postalCode+" "+city, "address")
	if err != nil {
		return Point{}, false, err
	}
	if found && address.Properties.Confidence >= 0.8 && postalMatches(address, postalCode) {
		point, err := coordinates(address)
		return point, false, err
	}

	postal, found, err := c.search(ctx, postalCode+" "+city, "")
	if err != nil {
		return Point{}, false, err
	}
	if found && postal.Properties.PostalCode == postalCode {
		point, err := coordinates(postal)
		return point, true, err
	}

	municipality, found, err := c.search(ctx, city, "localadmin")
	if err != nil {
		return Point{}, false, err
	}
	if found {
		point, err := coordinates(municipality)
		return point, true, err
	}

	return Point{}, false, ErrNotFound
}

func (c *Client) search(ctx context.Context, text, layer string) (feature, bool, error) {
	if c.Delay > 0 {
		select {
		case <-ctx.Done():
			return feature{}, false, ctx.Err()
		case <-time.After(c.Delay):
		}
	}

	endpoint, err := url.Parse(c.BaseURL)
	if err != nil {
		return feature{}, false, fmt.Errorf("parse geocoding URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("text", text)
	query.Set("size", "1")
	if layer != "" {
		query.Set("layers", layer)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return feature{}, false, fmt.Errorf("build geocoding request: %w", err)
	}
	req.Header.Set("digitransit-subscription-key", c.Key)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return feature{}, false, fmt.Errorf("geocode %q: %w", text, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return feature{}, false, fmt.Errorf("geocode %q returned HTTP %d", text, resp.StatusCode)
	}

	var result searchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return feature{}, false, fmt.Errorf("decode geocode %q: %w", text, err)
	}
	if len(result.Features) == 0 {
		return feature{}, false, nil
	}
	return result.Features[0], true, nil
}

func postalMatches(f feature, postalCode string) bool {
	return f.Properties.PostalCode == "" || f.Properties.PostalCode == postalCode
}

func coordinates(f feature) (Point, error) {
	if len(f.Geometry.Coordinates) != 2 {
		return Point{}, fmt.Errorf("geocode: feature has %d coordinates", len(f.Geometry.Coordinates))
	}
	return Point{Lat: f.Geometry.Coordinates[1], Lon: f.Geometry.Coordinates[0]}, nil
}
