package careers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/thvphuc246/finding-jobs-around/internal/useragent"
)

var hiringPlatforms = []string{
	"teamtailor.com",
	"recruitee.com",
	"greenhouse.io",
	"lever.co",
	"workable.com",
	"smartrecruiters.com",
	"personio.",
	"jobylon.com",
	"talentadore.com",
	"myworkdayjobs.com",
	"successfactors",
	"ashbyhq.com",
	"breezy.hr",
	"laura.fi",
	"rekrytointi.com",
	"eilakaisla.fi",
	"varbi.com",
	"reachmee.com",
}

var careerSubdomains = map[string]bool{
	"careers":   true,
	"career":    true,
	"jobs":      true,
	"job":       true,
	"rekry":     true,
	"ura":       true,
	"tyopaikat": true,
	"join":      true,
	"work":      true,
}

var careerSegments = map[string]bool{
	"careers":           true,
	"career":            true,
	"jobs":              true,
	"job":               true,
	"rekry":             true,
	"rekrytointi":       true,
	"ura":               true,
	"uralle":            true,
	"tyopaikat":         true,
	"työpaikat":         true,
	"avoimet-tyopaikat": true,
	"avoimet-työpaikat": true,
	"join-us":           true,
	"joinus":            true,
	"tule-meille":       true,
	"tule-toihin":       true,
	"work-with-us":      true,
	"open-positions":    true,
	"toihin":            true,
	"töihin":            true,
	"vacancies":         true,
}

var careerTextPattern = regexp.MustCompile(`(?i)^(?:careers?|jobs|join us|join our team|work with us|open positions|vacancies|rekry|rekrytointi|ura|uralla|työpaikat|avoimet työpaikat|tule meille(?: töihin)?|tule töihin|töihin(?: meille)?)$`)

type Status int

const (
	None Status = iota
	Found
	Blocked
)

type Result struct {
	Status Status
	URL    string
}

type Client struct {
	HTTP  *http.Client
	Delay time.Duration
}

type link struct {
	url  string
	text string
}

func Detect(ctx context.Context, client *Client, websiteURL string) (Result, error) {
	site, err := siteURL(websiteURL)
	if err != nil {
		return Result{}, err
	}

	fetcher := &useragent.Fetcher{HTTP: client.HTTP, Delay: client.Delay}

	allowed, err := fetcher.RobotsAllowed(ctx, site)
	if err != nil {
		return Result{}, err
	}
	if !allowed {
		return Result{Status: Blocked}, nil
	}

	status, final, body, err := fetcher.Get(ctx, site.String(), useragent.MaxPageBytes)
	if err != nil {
		return Result{}, fmt.Errorf("fetch homepage: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return Result{}, fmt.Errorf("homepage returned HTTP %d", status)
	}

	links, err := parseLinks(final, body)
	if err != nil {
		return Result{}, err
	}

	best := 0
	var bestURL string
	for _, candidate := range links {
		if score := candidate.score(); score > best {
			best = score
			bestURL = candidate.url
		}
	}
	if best == 0 {
		return Result{Status: None}, nil
	}
	return Result{Status: Found, URL: bestURL}, nil
}

func siteURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	site, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse website URL %q: %w", raw, err)
	}
	if site.Scheme != "http" && site.Scheme != "https" {
		return nil, fmt.Errorf("website URL %q is not http(s)", raw)
	}
	if site.Host == "" {
		return nil, fmt.Errorf("website URL %q has no host", raw)
	}
	return site, nil
}

func parseLinks(base *url.URL, body []byte) ([]link, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse homepage: %w", err)
	}

	var links []link
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			if href, ok := attribute(node, "href"); ok {
				if resolved, ok := resolve(base, href); ok {
					links = append(links, link{url: resolved, text: linkText(node)})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return links, nil
}

func attribute(node *html.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}

func resolve(base *url.URL, href string) (string, bool) {
	reference, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	if resolved.Host == "" {
		return "", false
	}
	resolved.Fragment = ""
	return resolved.String(), true
}

func linkText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func (l link) score() int {
	parsed, err := url.Parse(l.url)
	if err != nil {
		return 0
	}
	host := strings.ToLower(parsed.Hostname())
	for _, platform := range hiringPlatforms {
		if strings.Contains(host, platform) {
			return 4
		}
	}
	label, _, _ := strings.Cut(host, ".")
	if careerSubdomains[label] {
		return 3
	}
	if careerPath(parsed) {
		return 2
	}
	if careerTextPattern.MatchString(l.text) {
		return 1
	}
	return 0
}

func careerPath(parsed *url.URL) bool {
	trimmed := strings.Trim(parsed.EscapedPath(), "/")
	if trimmed == "" {
		return false
	}
	segments := strings.Split(trimmed, "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			continue
		}
		if !careerSegments[strings.ToLower(decoded)] {
			continue
		}
		if i == len(segments)-1 || len(segments) <= 2 {
			return true
		}
	}
	return false
}
