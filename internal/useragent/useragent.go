package useragent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/temoto/robotstxt"
)

const (
	Agent        = "finding-jobs-around/1.0"
	MaxPageBytes = 1 << 20
)

type transport struct {
	base http.RoundTripper
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("User-Agent", Agent)
	return t.base.RoundTrip(clone)
}

func Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: transport{base: http.DefaultTransport},
	}
}

type Fetcher struct {
	HTTP  *http.Client
	Delay time.Duration
}

func (f *Fetcher) Get(ctx context.Context, rawURL string, limit int64) (int, *url.URL, []byte, error) {
	if f.Delay > 0 {
		select {
		case <-ctx.Done():
			return 0, nil, nil, ctx.Err()
		case <-time.After(f.Delay):
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Request.URL, body, nil
}

func (f *Fetcher) RobotsAllowed(ctx context.Context, site *url.URL) (bool, error) {
	robotsURL := &url.URL{Scheme: site.Scheme, Host: site.Host, Path: "/robots.txt"}
	status, _, body, err := f.Get(ctx, robotsURL.String(), MaxPageBytes)
	if err != nil {
		return false, fmt.Errorf("fetch robots.txt: %w", err)
	}
	robots, err := robotstxt.FromStatusAndBytes(status, body)
	if err != nil {
		return false, fmt.Errorf("parse robots.txt: %w", err)
	}
	return robots.FindGroup(Agent).Test(site.RequestURI()), nil
}
