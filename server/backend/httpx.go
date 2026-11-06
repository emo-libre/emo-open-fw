package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// remote is a guarded HTTP client for one API host, with the credentials the API wants.
type remote struct {
	c    *http.Client
	base *url.URL
}

func newRemote(baseURL string, timeoutS int, def time.Duration) (*remote, error) {
	t := def
	if timeoutS > 0 {
		t = time.Duration(timeoutS) * time.Second
	}
	c, u, err := NewGuardedClient(baseURL, t)
	if err != nil {
		return nil, err
	}
	return &remote{c: c, base: u}, nil
}

func (r *remote) url(path string, q url.Values) string {
	u := *r.base
	u.Path = strings.TrimRight(u.Path, "/") + path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// urlSeg builds prefix + one escaped path segment (so "/" inside the segment stays inside it) + query.
func (r *remote) urlSeg(prefix, segment string, q url.Values) string {
	s := strings.TrimRight(r.base.String(), "/") + prefix + url.PathEscape(segment)
	if q != nil {
		s += "?" + q.Encode()
	}
	return s
}

// call sends one request and returns the body. Error messages never include the response body or the URL (which may hold a key).
func (r *remote) call(ctx context.Context, method, fullURL string, hdr map[string]string, body any) ([]byte, string, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, rd)
	if err != nil {
		return nil, "", fmt.Errorf("backend: bad request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := r.c.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("backend %s: request failed", r.base.Host)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, "", fmt.Errorf("backend %s: read failed", r.base.Host)
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("backend %s: HTTP %d", r.base.Host, resp.StatusCode)
	}
	return b, resp.Header.Get("Content-Type"), nil
}

// envKey reads an API key from the named environment variable. Keys are never read from the config file.
func envKey(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

// bcp47 maps a base language code to the regional tag cloud speech services expect.
func bcp47(lang string) string {
	m := map[string]string{"en": "en-US", "de": "de-DE", "fr": "fr-FR", "es": "es-ES", "it": "it-IT", "pt": "pt-PT", "vi": "vi-VN",
		"zh": "zh-CN", "ja": "ja-JP", "ko": "ko-KR", "nl": "nl-NL", "ru": "ru-RU", "pl": "pl-PL", "tr": "tr-TR", "th": "th-TH", "id": "id-ID"}
	if v, ok := m[strings.ToLower(lang)]; ok {
		return v
	}
	if lang == "" {
		return "en-US"
	}
	return lang
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
