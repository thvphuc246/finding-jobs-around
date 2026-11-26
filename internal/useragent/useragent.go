package useragent

import (
	"net/http"
	"time"
)

const Agent = "finding-jobs-around/1.0"

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
