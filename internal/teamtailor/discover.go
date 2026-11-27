package teamtailor

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/thvphuc246/finding-jobs-around/internal/useragent"
)

type Client struct {
	HTTP  *http.Client
	Delay time.Duration
}

func Discover(ctx context.Context, client *Client, careersURL string) (string, bool, error) {
	careers, err := parseCareersURL(careersURL)
	if err != nil {
		return "", false, err
	}
	fetcher := client.fetcher()

	allowed, err := fetcher.RobotsAllowed(ctx, careers)
	if err != nil {
		return "", false, err
	}
	if !allowed {
		return "", false, nil
	}

	origins, err := candidateOrigins(ctx, fetcher, careers)
	if err != nil {
		return "", false, err
	}

	for _, origin := range origins {
		feedURL := origin + "/jobs.rss"
		status, _, body, err := fetcher.Get(ctx, feedURL, useragent.MaxPageBytes)
		if err != nil {
			return "", false, fmt.Errorf("fetch feed %s: %w", feedURL, err)
		}
		if status != http.StatusOK {
			continue
		}
		if _, err := Parse(bytes.NewReader(body)); err != nil {
			continue
		}
		return feedURL, true, nil
	}
	return "", false, nil
}

func (c *Client) Fetch(ctx context.Context, feedURL string) ([]Job, error) {
	status, _, body, err := c.fetcher().Get(ctx, feedURL, useragent.MaxPageBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("feed returned HTTP %d", status)
	}
	return Parse(bytes.NewReader(body))
}

func (c *Client) fetcher() *useragent.Fetcher {
	return &useragent.Fetcher{HTTP: c.HTTP, Delay: c.Delay}
}

func parseCareersURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	careers, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse careers URL %q: %w", raw, err)
	}
	if careers.Scheme != "http" && careers.Scheme != "https" {
		return nil, fmt.Errorf("careers URL %q is not http(s)", raw)
	}
	if careers.Host == "" {
		return nil, fmt.Errorf("careers URL %q has no host", raw)
	}
	return careers, nil
}

func candidateOrigins(ctx context.Context, fetcher *useragent.Fetcher, careers *url.URL) ([]string, error) {
	if isTeamtailorHost(careers.Hostname()) {
		return []string{origin(careers)}, nil
	}

	status, final, body, err := fetcher.Get(ctx, careers.String(), useragent.MaxPageBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch careers page: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("careers page returned HTTP %d", status)
	}

	var origins []string
	if bytes.Contains(body, []byte("teamtailor-cdn.com")) && !bytes.Contains(body, []byte("teamtailor-jobs-widget")) {
		origins = append(origins, origin(final))
	}
	for _, linked := range teamtailorLinks(final, body) {
		origins = append(origins, origin(linked))
	}
	return unique(origins), nil
}

func teamtailorLinks(base *url.URL, body []byte) []*url.URL {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	var links []*url.URL
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			if href, ok := attribute(node, "href"); ok {
				reference, err := url.Parse(strings.TrimSpace(href))
				if err == nil {
					resolved := base.ResolveReference(reference)
					if (resolved.Scheme == "http" || resolved.Scheme == "https") && isTeamtailorHost(resolved.Hostname()) {
						links = append(links, resolved)
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return links
}

func attribute(node *html.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}

func isTeamtailorHost(host string) bool {
	return strings.HasSuffix(strings.ToLower(host), ".teamtailor.com")
}

func origin(parsed *url.URL) string {
	return parsed.Scheme + "://" + parsed.Host
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := values[:0]
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
