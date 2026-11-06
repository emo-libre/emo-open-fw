package main

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---- usage counters and robot status (in memory only) ----

type robotInfo struct {
	LastSeen time.Time        `json:"last_seen"`
	Reports  []map[string]any `json:"reports"`
}

type stats struct {
	mu     sync.Mutex
	hits   map[string]int
	robots map[string]*robotInfo
}

func newStats() *stats { return &stats{hits: map[string]int{}, robots: map[string]*robotInfo{}} }

func routeKey(path string) string {
	p := strings.Split(strings.Trim(path, "/"), "/")
	n := 1
	if len(p) > 0 && p[0] == "emo" {
		n = 3
	}
	if len(p) < n {
		n = len(p)
	}
	return "/" + strings.Join(p[:n], "/")
}

func (st *stats) hit(path string) {
	st.mu.Lock()
	st.hits[routeKey(path)]++
	st.mu.Unlock()
}

func (st *stats) seen(dev string) {
	if dev == "" || dev == "anon" {
		return
	}
	st.mu.Lock()
	r := st.robots[dev]
	if r == nil {
		r = &robotInfo{}
		st.robots[dev] = r
	}
	r.LastSeen = time.Now()
	st.mu.Unlock()
}

// The only telemetry fields kept. City, time zone, language, age and anything else the robot reports are dropped on arrival.
var reportNumeric = []string{"ver", "batt", "wifi", "wkup", "drop", "beh", "err"}
var reportStrings = map[string]int{"esp": 16, "ken": 16, "kcrash": 24}

func sanitizeReport(body []byte) map[string]any {
	var in map[string]any
	if json.Unmarshal(body, &in) != nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range reportNumeric {
		if v, ok := in[k].(float64); ok {
			out[k] = v
		}
	}
	for k, max := range reportStrings {
		if v, ok := in[k].(string); ok && len(v) <= max {
			out[k] = v
		}
	}
	return out
}

func (st *stats) report(dev string, body []byte) {
	rep := sanitizeReport(body)
	if rep == nil || dev == "" || dev == "anon" {
		return
	}
	rep["at"] = time.Now().UTC().Format(time.RFC3339)
	st.mu.Lock()
	defer st.mu.Unlock()
	r := st.robots[dev]
	if r == nil {
		r = &robotInfo{}
		st.robots[dev] = r
	}
	r.LastSeen = time.Now()
	r.Reports = append(r.Reports, rep)
	if len(r.Reports) > 20 {
		r.Reports = r.Reports[len(r.Reports)-20:]
	}
}

// ---- middleware: counting, optional token requirement, device allow-list ----

func (s *server) allowedDevice(id string) bool {
	if len(s.cfg.AllowedDevices) == 0 {
		return true
	}
	for _, d := range s.cfg.AllowedDevices {
		if d == id {
			return true
		}
	}
	return false
}

func (s *server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.stats.hit(r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/emo/") {
			dev := s.deviceID(r)
			if dev == "anon" && s.cfg.RequireToken {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "errmessage": "unauthorized"})
				return
			}
			if dev != "anon" && !s.allowedDevice(dev) {
				writeJSON(w, http.StatusForbidden, map[string]any{"code": 403, "errmessage": "device not allowed"})
				return
			}
			s.stats.seen(dev)
		}
		next.ServeHTTP(w, r)
	})
}

// ---- admin API (disabled unless admin_token_env names a non-empty environment variable) ----

func adminToken(envName string) []byte {
	if envName == "" {
		return nil
	}
	return []byte(os.Getenv(envName))
}

// authThrottle slows guessing: after 8 wrong tokens from one address within a minute, that address is told to wait for a minute.
type authThrottle struct {
	mu   sync.Mutex
	fail map[string][]time.Time
}

const (
	throttleMax    = 8
	throttleWindow = time.Minute
)

func (t *authThrottle) blocked(ip string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.recent(ip, now)) >= throttleMax
}

func (t *authThrottle) recent(ip string, now time.Time) []time.Time {
	var keep []time.Time
	for _, x := range t.fail[ip] {
		if now.Sub(x) < throttleWindow {
			keep = append(keep, x)
		}
	}
	if len(keep) == 0 {
		delete(t.fail, ip)
	} else {
		t.fail[ip] = keep
	}
	return keep
}

func (t *authThrottle) failed(ip string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fail == nil {
		t.fail = map[string][]time.Time{}
	}
	if len(t.fail) > 1000 { // never grow without bound
		t.fail = map[string][]time.Time{}
	}
	t.fail[ip] = append(t.recent(ip, now), now)
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func (s *server) adminAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, now := clientIP(r), time.Now()
		if s.throttle.blocked(ip, now) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"code": 429, "errmessage": "too many wrong tokens; wait a minute"})
			return
		}
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(tok), s.adminTok) != 1 {
			if ok { // a missing header is not a guess
				s.throttle.failed(ip, now)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "errmessage": "unauthorized"})
			return
		}
		h(w, r)
	}
}

// adminURLs lists the addresses the admin page can be opened at, for the first-run message and the Help tab.
func (s *server) adminURLs() []string {
	var out []string
	hosts := lanIPs()
	hosts = append([]string{"127.0.0.1"}, hosts...)
	add := func(scheme, listen string) {
		if listen == "" {
			return
		}
		_, port, err := net.SplitHostPort(listen)
		if err != nil {
			return
		}
		h, _, _ := net.SplitHostPort(listen)
		for _, host := range hosts {
			if h == "" || h == host || h == "0.0.0.0" || h == "::" {
				out = append(out, scheme+"://"+host+":"+port+"/admin/ui")
			}
		}
	}
	add("http", s.cfg.ListenAdmin)
	add("https", s.cfg.ListenAdminTLS)
	if s.cfg.ListenAdmin == "" && s.cfg.ListenAdminTLS == "" && s.cfg.Listen != "" {
		add("https", s.cfg.Listen)
	}
	return out
}

func (s *server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	s.stats.mu.Lock()
	hits := map[string]int{}
	for k, v := range s.stats.hits {
		hits[k] = v
	}
	s.stats.mu.Unlock()
	cacheFiles, cacheBytes := 0, int64(0)
	if s.ttsCache != nil {
		for _, f := range listCache(s.ttsCache.dir) {
			cacheFiles++
			cacheBytes += f
		}
	}
	s.store.mu.Lock()
	items, storeBytes := len(s.store.items), s.store.total
	s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"uptime_s": int(time.Since(s.started).Seconds()), "version_num": s.cfg.VersionNum, "requests": hits,
		"backends": map[string]bool{"stt": s.be.STT != nil, "llm": s.be.LLM != nil, "tts": s.be.TTS != nil, "vision": s.be.Vision != nil,
			"weather": s.be.Weather != nil, "image": s.be.Image != nil, "news": s.be.News != nil},
		"tts_cache":  map[string]any{"files": cacheFiles, "bytes": cacheBytes},
		"audio_held": map[string]any{"items": items, "bytes": storeBytes},
	})
}

func (s *server) handleAdminRobots(w http.ResponseWriter, r *http.Request) {
	s.stats.mu.Lock()
	ids := make([]string, 0, len(s.stats.robots))
	for id := range s.stats.robots {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		ri := s.stats.robots[id]
		list = append(list, map[string]any{"id": id, "last_seen": ri.LastSeen.UTC().Format(time.RFC3339), "reports": ri.Reports})
	}
	s.stats.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"robots": list})
}
