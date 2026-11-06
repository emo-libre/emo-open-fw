package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"emoserver/backend"
)

// ---- activity log: what the robot heard and what was done, for the admin page ----
// Off unless activity_log is set. Memory only (never written to disk), the last 100 entries, the robot shown by the last four
// characters of its id, the heard text cut at 200 characters.

type activityEntry struct {
	At     string         `json:"at"`
	Robot  string         `json:"robot"`
	Lang   string         `json:"lang"`
	Heard  string         `json:"heard"`
	Result string         `json:"result"`
	Ms     int            `json:"ms"`
	Stages map[string]int `json:"stages,omitempty"` // milliseconds per backend: stt, llm, vision, tts
}

type activityLog struct {
	mu  sync.Mutex
	on  bool
	buf []activityEntry
}

const activityMax = 100

func (a *activityLog) add(dev, lang, heard, result string, took time.Duration, stages map[string]int) {
	if a == nil || !a.on {
		return
	}
	if utf8.RuneCountInString(heard) > 200 {
		heard = string([]rune(heard)[:200])
	}
	if len(dev) > 4 {
		dev = dev[len(dev)-4:]
	}
	a.mu.Lock()
	a.buf = append(a.buf, activityEntry{At: time.Now().UTC().Format(time.RFC3339), Robot: dev, Lang: lang, Heard: heard, Result: result, Ms: int(took.Milliseconds()), Stages: stages})
	if len(a.buf) > activityMax {
		a.buf = a.buf[len(a.buf)-activityMax:]
	}
	a.mu.Unlock()
}

func (a *activityLog) list() []activityEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]activityEntry, len(a.buf))
	for i, e := range a.buf { // newest first
		out[len(a.buf)-1-i] = e
	}
	return out
}

func (a *activityLog) clear() {
	a.mu.Lock()
	a.buf = nil
	a.mu.Unlock()
}

func (s *server) handleAdminActivity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.act.on, "entries": s.act.list()})
}

func (s *server) handleAdminActivityClear(w http.ResponseWriter, r *http.Request) {
	s.act.clear()
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

// ---- self-test: one tiny request to each configured service ----

type testResult struct {
	OK    bool   `json:"ok"`
	Ms    int    `json:"ms"`
	Error string `json:"error,omitempty"`
}

// silentWAV is half a second of silence.
func silentWAV() []byte { return wavFromPCM(make([]byte, pcmRate), pcmRate) }

func (s *server) handleAdminSelfTest(w http.ResponseWriter, r *http.Request) {
	if !s.testMu.TryLock() { // each run may cost money or time at an online service: one at a time
		writeJSON(w, http.StatusConflict, map[string]any{"code": 409, "errmessage": "a test is already running"})
		return
	}
	defer s.testMu.Unlock()
	type job struct {
		name string
		run  func(ctx context.Context) error
	}
	var jobs []job
	if s.be.STT != nil {
		jobs = append(jobs, job{"speech_to_text", func(ctx context.Context) error { _, err := s.be.STT.Transcribe(ctx, silentWAV(), "en"); return err }})
	}
	if s.be.LLM != nil {
		jobs = append(jobs, job{"language_model", func(ctx context.Context) error {
			out, err := s.be.LLM.Chat(ctx, []backend.Message{{Role: "user", Content: "Reply with the single word: ok"}})
			if err == nil && strings.TrimSpace(out) == "" {
				return errEmptyAnswer
			}
			return err
		}})
	}
	if s.be.TTS != nil {
		jobs = append(jobs, job{"text_to_speech", func(ctx context.Context) error {
			b, err := s.be.TTS.Synthesize(ctx, "ok", "en")
			if err == nil && len(b) == 0 {
				return errEmptyAnswer
			}
			return err
		}})
	}
	if s.be.Weather != nil {
		jobs = append(jobs, job{"weather", func(ctx context.Context) error { _, err := s.be.Weather.Forecast(ctx, "", 0); return err }})
	}
	if s.be.News != nil {
		jobs = append(jobs, job{"news", func(ctx context.Context) error { _, err := s.be.News.Headlines(ctx, 1); return err }})
	}
	results := map[string]testResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, j := range jobs {
		j := j
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			t0 := time.Now()
			err := j.run(ctx)
			res := testResult{OK: err == nil, Ms: int(time.Since(t0).Milliseconds())}
			if err != nil {
				res.Error = err.Error()
			}
			mu.Lock()
			results[j.name] = res
			mu.Unlock()
		}()
	}
	wg.Wait()
	not := []string{}
	for name, ok := range map[string]bool{"vision (photo description)": s.be.Vision != nil, "drawing": s.be.Image != nil} {
		if ok {
			not = append(not, name)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "configured_but_not_tested": not})
}

var errEmptyAnswer = &emptyAnswer{}

type emptyAnswer struct{}

func (*emptyAnswer) Error() string { return "the service answered with nothing" }

// handleAdminSay speaks a short text through the configured voice and returns the MP3, so the page can play it. Never cached.
func (s *server) handleAdminSay(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(r.URL.Query().Get("text"))
	if s.be.TTS == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"code": 503, "errmessage": "no text-to-speech is configured"})
		return
	}
	if text == "" || utf8.RuneCountInString(text) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": "text is required (up to 200 characters)"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	b, err := s.be.TTS.Synthesize(ctx, text, baseLang(r.URL.Query().Get("lang")))
	if err != nil || len(b) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]any{"code": 502, "errmessage": "the voice did not answer"})
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

// ---- setup checklist ----

type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func (s *server) setupChecks() []check {
	var out []check
	add := func(name string, ok bool, detail string) { out = append(out, check{name, ok, detail}) }
	_, ferr := statFile(s.cfg.FirmwareDir, s.cfg.ESP32File)
	add("Firmware image to offer the robot", ferr == nil, map[bool]string{true: s.cfg.ESP32File, false: "put " + s.cfg.ESP32File + " in " + s.cfg.FirmwareDir + " (only needed for updating the robot)"}[ferr == nil])
	add("Public host set", s.cfg.PublicHost != "" && !strings.HasPrefix(s.cfg.PublicHost, "192.0.2."), "public_host is "+s.cfg.PublicHost+" (the robot's downloads use it)")
	add("Speech has a voice", s.be.TTS != nil, map[bool]string{true: "text-to-speech is configured", false: "no tts backend: the robot cannot speak replies"}[s.be.TTS != nil])
	add("Speech has ears", s.be.STT != nil, map[bool]string{true: "speech-to-text is configured", false: "no stt backend: the robot cannot understand speech (touch/command-free features still work)"}[s.be.STT != nil])
	add("Conversation", s.be.LLM != nil, map[bool]string{true: "a language model is configured", false: "no llm backend: only built-in commands work"}[s.be.LLM != nil])
	add("Robot finds this server", s.dns != nil, map[bool]string{true: "the built-in DNS helper is on", false: "dns_listen is off: point the robot's API host name here with your router, or turn the helper on"}[s.dns != nil])
	s.stats.mu.Lock()
	seen := len(s.stats.robots)
	s.stats.mu.Unlock()
	add("Your robot has connected", seen > 0, map[bool]string{true: "a robot has contacted this server", false: "no robot yet: see Help > Making the robot find this server"}[seen > 0])
	add("Only your robot gets in", s.cfg.RequireToken || len(s.cfg.AllowedDevices) > 0, "require_token / allowed_devices (turn on after a capture shows every call carries its token)")
	add("Recent activity visible", s.act.on, map[bool]string{true: "activity_log is on", false: "activity_log is off (set it to see what the robot heard)"}[s.act.on])
	return out
}

func (s *server) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"checks": s.setupChecks()})
}

func statFile(dir, name string) (os.FileInfo, error) { return os.Stat(filepath.Join(dir, name)) }
