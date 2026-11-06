package backend

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// vendorSuffixes are the robot vendor's domains. No backend may point at them, whatever the config says.
var vendorSuffixes = []string{"living.ai", "livingai.cn"}

// BlockedHost reports whether host (with or without port) belongs to the robot vendor.
func BlockedHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	for _, s := range vendorSuffixes {
		if h == s || strings.HasSuffix(h, "."+s) {
			return true
		}
	}
	return false
}

// guardedTransport only lets requests through to the single configured host.
type guardedTransport struct {
	allowed string
	base    http.RoundTripper
}

func (g guardedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != g.allowed || BlockedHost(r.URL.Host) {
		return nil, fmt.Errorf("backend: request to %q refused (only %q is allowed)", r.URL.Host, g.allowed)
	}
	return g.base.RoundTrip(r)
}

// NewGuardedClient returns an HTTP client bound to baseURL's host. It follows no redirects to other hosts.
func NewGuardedClient(baseURL string, timeout time.Duration) (*http.Client, *url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, nil, fmt.Errorf("backend: bad base_url %q", baseURL)
	}
	if BlockedHost(u.Host) {
		return nil, nil, errors.New("backend: base_url points at the robot vendor; refused")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	c := &http.Client{
		Timeout:   timeout,
		Transport: guardedTransport{allowed: u.Host, base: &http.Transport{TLSHandshakeTimeout: 10 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second}},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != u.Host {
				return errors.New("backend: redirect to another host refused")
			}
			if len(via) >= 3 {
				return errors.New("backend: too many redirects")
			}
			return nil
		},
	}
	return c, u, nil
}
