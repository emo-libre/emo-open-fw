package backend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// RSSConfig lists feeds (RSS 2.0 or Atom) to read headlines from. Each feed host is contacted directly by this server and
// only for this purpose; nothing about the robot or its owner is sent.
type RSSConfig struct {
	Feeds    []string `json:"feeds"`
	TimeoutS int      `json:"timeout_s"`
}

type feed struct {
	u *url.URL
	c *http.Client
}

type RSS struct {
	feeds []feed
	mu    sync.Mutex
	cache map[string]cachedFeed
	now   func() time.Time
}

type cachedFeed struct {
	titles []string
	at     time.Time
}

func NewRSS(cfg RSSConfig) (*RSS, error) {
	if len(cfg.Feeds) == 0 {
		return nil, errors.New("news: no feeds configured")
	}
	r := &RSS{cache: map[string]cachedFeed{}, now: time.Now}
	for _, f := range cfg.Feeds {
		c, u, err := NewGuardedClient(f, time.Duration(cfg.TimeoutS)*time.Second)
		if err != nil {
			return nil, fmt.Errorf("news feed %q: %w", f, err)
		}
		r.feeds = append(r.feeds, feed{u: u, c: c})
	}
	return r, nil
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

var cdataRe = regexp.MustCompile(`(?s)<!\[CDATA\[(.*?)\]\]>`)

// cleanTitle strips markup and entities and bounds the length. The input is the title element's raw inner XML, so inline
// tags (which feeds sometimes leave unescaped) and CDATA sections are handled.
func cleanTitle(s string) string {
	s = cdataRe.ReplaceAllString(s, "$1")
	s = html.UnescapeString(tagRe.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		s = strings.TrimSpace(string(r[:160]))
	}
	return s
}

type titleEl struct {
	Inner string `xml:",innerxml"`
}

type feedDoc struct {
	Items []struct {
		Title titleEl `xml:"title"`
	} `xml:"channel>item"`
	Entries []struct {
		Title titleEl `xml:"title"`
	} `xml:"entry"`
}

func parseFeed(b []byte) ([]string, error) {
	var d feedDoc
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	dec.Strict = false
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) { return in, nil } // assume UTF-8-compatible
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	var out []string
	for _, it := range d.Items {
		if t := cleanTitle(it.Title.Inner); t != "" {
			out = append(out, t)
		}
	}
	for _, e := range d.Entries {
		if t := cleanTitle(e.Title.Inner); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *RSS) fetch(ctx context.Context, f feed) ([]string, error) {
	key := f.u.String()
	r.mu.Lock()
	if c, ok := r.cache[key]; ok && r.now().Sub(c.at) < 10*time.Minute {
		r.mu.Unlock()
		return c.titles, nil
	}
	r.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, "GET", key, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("news: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	titles, err := parseFeed(b)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[key] = cachedFeed{titles, r.now()}
	r.mu.Unlock()
	return titles, nil
}

// Headlines interleaves the feeds (one from each in turn), drops duplicates, and returns at most n.
func (r *RSS) Headlines(ctx context.Context, n int) ([]string, error) {
	var lists [][]string
	for _, f := range r.feeds {
		if t, err := r.fetch(ctx, f); err == nil && len(t) > 0 {
			lists = append(lists, t)
		}
	}
	if len(lists) == 0 {
		return nil, errors.New("news: no headlines available")
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; len(out) < n; i++ {
		progressed := false
		for _, l := range lists {
			if i < len(l) {
				progressed = true
				k := strings.ToLower(l[i])
				if !seen[k] && len(out) < n {
					seen[k] = true
					out = append(out, l[i])
				}
			}
		}
		if !progressed {
			break
		}
	}
	return out, nil
}

// MockNews returns fixed headlines.
type MockNews struct{ Lines []string }

func (m MockNews) Headlines(ctx context.Context, n int) ([]string, error) {
	if len(m.Lines) == 0 {
		return nil, errors.New("news: none")
	}
	if n > len(m.Lines) {
		n = len(m.Lines)
	}
	return m.Lines[:n], nil
}
