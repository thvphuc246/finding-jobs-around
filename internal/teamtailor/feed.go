package teamtailor

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const namespace = "https://teamtailor.com/locations"

var openApplicationPattern = regexp.MustCompile(`(?i)^(?:open application|avoin hakemus|öppen ansökan)\b`)

type Job struct {
	GUID            string
	Title           string
	URL             string
	PublishedAt     *time.Time
	Remote          string
	OpenApplication bool
	Places          []Place
}

type Place struct {
	City    string
	Country string
}

type feed struct {
	XMLName xml.Name   `xml:"rss"`
	Attrs   []xml.Attr `xml:",any,attr"`
	Channel channel    `xml:"channel"`
}

type channel struct {
	Items []item `xml:"item"`
}

type item struct {
	Title        string    `xml:"title"`
	Link         string    `xml:"link"`
	GUID         string    `xml:"guid"`
	PubDate      string    `xml:"pubDate"`
	RemoteStatus string    `xml:"remoteStatus"`
	Locations    locations `xml:"locations"`
}

type locations struct {
	Location []location `xml:"location"`
}

type location struct {
	Name    string `xml:"name"`
	City    string `xml:"city"`
	Country string `xml:"country"`
}

func Parse(reader io.Reader) ([]Job, error) {
	var document feed
	if err := xml.NewDecoder(reader).Decode(&document); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	if !hasNamespace(document.Attrs) {
		return nil, errors.New("parse feed: missing the Teamtailor locations namespace")
	}

	jobs := make([]Job, 0, len(document.Channel.Items))
	for _, entry := range document.Channel.Items {
		job := Job{
			GUID:            strings.TrimSpace(entry.GUID),
			Title:           strings.TrimSpace(entry.Title),
			URL:             strings.TrimSpace(entry.Link),
			Remote:          strings.TrimSpace(entry.RemoteStatus),
			OpenApplication: openApplicationPattern.MatchString(strings.TrimSpace(entry.Title)),
		}
		job.PublishedAt = parsePublished(entry.PubDate)
		for _, place := range entry.Locations.Location {
			city := strings.TrimSpace(place.City)
			if city == "" {
				city = strings.TrimSpace(place.Name)
			}
			job.Places = append(job.Places, Place{
				City:    city,
				Country: strings.TrimSpace(place.Country),
			})
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func hasNamespace(attrs []xml.Attr) bool {
	for _, attr := range attrs {
		if attr.Name.Space == "xmlns" && attr.Name.Local == "tt" && attr.Value == namespace {
			return true
		}
	}
	return false
}

func parsePublished(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123} {
		if published, err := time.Parse(layout, raw); err == nil {
			return &published
		}
	}
	return nil
}
