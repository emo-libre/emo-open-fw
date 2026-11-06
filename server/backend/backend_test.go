package backend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var mp3Bytes = []byte{0xFF, 0xFB, 0x90, 0xC4, 1, 2, 3}

func TestBlockedHost(t *testing.T) {
	for _, h := range []string{"api.living.ai", "EU-API.LIVING.AI", "api.living.ai:443", "res.living.ai.", "living.ai", "x.livingai.cn", "a.b.livingai.cn:8080"} {
		if !BlockedHost(h) {
			t.Errorf("%q should be blocked", h)
		}
	}
	for _, h := range []string{"127.0.0.1:8000", "localhost", "api.openai.com", "notliving.ai", "living.ai.example.com", "[::1]:8080"} {
		if BlockedHost(h) {
			t.Errorf("%q should be allowed", h)
		}
	}
}

func TestGuardedClient(t *testing.T) {
	if _, _, err := NewGuardedClient("https://api.living.ai/v1", 0); err == nil {
		t.Fatal("vendor base_url accepted")
	}
	if _, _, err := NewGuardedClient("ftp://x/y", 0); err == nil {
		t.Fatal("bad scheme accepted")
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "other") }))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer redirect.Close()

	c, _, err := NewGuardedClient(redirect.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(other.URL); err == nil { // a different host than configured
		t.Fatal("request to another host was allowed")
	}
	if _, err := c.Get(redirect.URL); err == nil { // redirect to another host
		t.Fatal("cross-host redirect followed")
	}
}

func TestOpenAIClients(t *testing.T) {
	var gotAuth, gotModel, gotLang, gotSystem string
	var sawImage bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			gotModel, gotLang = r.FormValue("model"), r.FormValue("language")
			f, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "no file", 400)
				return
			}
			b, _ := io.ReadAll(f)
			io.WriteString(w, `{"text":"  hello there  `+string(b[:4])+`"}`)
		case "/v1/chat/completions":
			var req struct {
				Model    string `json:"model"`
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			gotModel = req.Model
			if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
				json.Unmarshal(req.Messages[0].Content, &gotSystem)
			}
			for _, m := range req.Messages {
				if strings.Contains(string(m.Content), "image_url") && strings.Contains(string(m.Content), "data:image/jpeg;base64,") {
					sawImage = true
				}
			}
			io.WriteString(w, `{"choices":[{"message":{"content":" hi back "}}]}`)
		case "/v1/audio/speech":
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if req["response_format"] != "mp3" || req["input"] != "say this" {
				http.Error(w, "bad", 400)
				return
			}
			if req["voice"] == "broken" {
				io.WriteString(w, "not audio")
				return
			}
			w.Write(mp3Bytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("TEST_KEY_ENV", "sekret")
	cfg := OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m1", APIKeyEnv: "TEST_KEY_ENV", System: "be brief"}

	stt, err := NewOpenAISTT(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text, err := stt.Transcribe(context.Background(), []byte("RIFFxxxx"), "en")
	if err != nil || !strings.HasPrefix(text, "hello there") || !strings.HasSuffix(text, "RIFF") {
		t.Fatalf("stt: %q %v", text, err)
	}
	if gotModel != "m1" || gotLang != "en" || gotAuth != "Bearer sekret" {
		t.Fatalf("model %q lang %q auth %q", gotModel, gotLang, gotAuth)
	}

	llm, _ := NewOpenAILLM(cfg)
	out, err := llm.Chat(context.Background(), []Message{{Role: "user", Content: "yo"}})
	if err != nil || out != "hi back" || gotSystem != "be brief" || gotModel != "m1" {
		t.Fatalf("llm: %q %v sys=%q", out, err, gotSystem)
	}
	if _, err := llm.Chat(context.Background(), []Message{{Role: "user", Content: "what is this", ImageJPEG: []byte{0xFF, 0xD8, 1}}}); err != nil || !sawImage {
		t.Fatalf("vision message not sent as image_url: %v", err)
	}

	tts, _ := NewOpenAITTS(cfg)
	b, err := tts.Synthesize(context.Background(), "say this", "en")
	if err != nil || string(b) != string(mp3Bytes) {
		t.Fatalf("tts: %v %v", b, err)
	}
	cfg.Voice = "broken"
	bad, _ := NewOpenAITTS(cfg)
	if _, err := bad.Synthesize(context.Background(), "say this", "en"); err == nil {
		t.Fatal("non-MP3 reply accepted")
	}
	cfg.APIKeyEnv = ""
	cfg.Voice = ""
	if _, err := NewOpenAITTS(OpenAIConfig{BaseURL: "https://api.living.ai/v1"}); err == nil {
		t.Fatal("vendor URL accepted for tts")
	}
}

func TestCommandBackends(t *testing.T) {
	stt := NewCommandSTT(CommandConfig{Args: []string{"sh", "-c", "head -c 4 \"$1\"; echo \" $2\"", "sh", "{wav}", "{lang}"}})
	got, err := stt.Transcribe(context.Background(), []byte("RIFFabc"), "de")
	if err != nil || got != "RIFF de" {
		t.Fatalf("stt: %q %v", got, err)
	}
	if _, err := NewCommandSTT(CommandConfig{Args: []string{"/nonexistent/prog"}}).Transcribe(context.Background(), []byte("x"), "en"); err == nil {
		t.Fatal("missing program accepted")
	}

	tts, err := NewCommandTTS(CommandConfig{Args: []string{"sh", "-c", "cat >/dev/null; printf '\\377\\373\\220\\304' > \"$1\"", "sh", "{out}"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := tts.Synthesize(context.Background(), "hello", "en")
	if err != nil || len(b) != 4 || b[0] != 0xFF {
		t.Fatalf("tts: %v %v", b, err)
	}
	// stdin really carries the text
	dir := t.TempDir()
	cap := filepath.Join(dir, "captured")
	tts2, _ := NewCommandTTS(CommandConfig{Args: []string{"sh", "-c", "cat > \"$2\"; printf '\\377\\373\\220\\304' > \"$1\"", "sh", "{out}", cap}})
	if _, err := tts2.Synthesize(context.Background(), "spoken words", "en"); err != nil {
		t.Fatal(err)
	}
	if c, _ := os.ReadFile(cap); string(c) != "spoken words" {
		t.Fatalf("stdin was %q", c)
	}
	// wav output without a converter is refused up front; non-MP3 output is refused at run time
	if _, err := NewCommandTTS(CommandConfig{Args: []string{"x"}, OutFormat: "wav"}); err == nil {
		t.Fatal("wav without convert accepted")
	}
	notMP3, _ := NewCommandTTS(CommandConfig{Args: []string{"sh", "-c", "printf 'RIFF....WAVE' > \"$1\"", "sh", "{out}"}})
	if _, err := notMP3.Synthesize(context.Background(), "x", "en"); err == nil {
		t.Fatal("non-MP3 output accepted")
	}
	// conversion step
	conv, _ := NewCommandTTS(CommandConfig{
		Args:        []string{"sh", "-c", "printf 'RIFF....WAVE' > \"$1\"", "sh", "{out}"},
		ConvertArgs: []string{"sh", "-c", "printf '\\377\\373\\220\\304' > \"$2\"", "sh", "{in}", "{out}"}, OutFormat: "wav"})
	if b, err := conv.Synthesize(context.Background(), "x", "en"); err != nil || b[0] != 0xFF {
		t.Fatalf("convert: %v %v", b, err)
	}
	// timeout
	slow := NewCommandSTT(CommandConfig{Args: []string{"sleep", "5"}, TimeoutS: 1})
	start := time.Now()
	if _, err := slow.Transcribe(context.Background(), []byte("x"), "en"); err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("timeout not enforced")
	}
}

func TestMocks(t *testing.T) {
	if r, _ := (MockLLM{}).Chat(context.Background(), []Message{{Role: "user", Content: " hi "}}); r != "You said: hi" {
		t.Fatal(r)
	}
	b, _ := (MockTTS{}).Synthesize(context.Background(), strings.Repeat("x", 40), "en")
	if !IsMP3(b) || len(b) == 0 {
		t.Fatal("mock mp3 invalid")
	}
	if !IsMP3([]byte("ID3\x04")) || IsMP3([]byte("RIFF")) || IsMP3(nil) {
		t.Fatal("IsMP3")
	}
}

func TestWMOMapping(t *testing.T) {
	want := map[int]string{0: "sunny", 1: "sunny", 2: "cloudy", 3: "cloudy", 45: "fog", 48: "fog", 51: "rain", 61: "rain",
		65: "rain", 71: "snow", 75: "snow", 80: "rain", 85: "snow", 95: "rain", 99: "rain", 12: "cloudy"}
	for code, w := range want {
		if got := wmoToMain(code); got != w {
			t.Errorf("code %d -> %s, want %s", code, got, w)
		}
	}
}

func TestOpenMeteo(t *testing.T) {
	var geoHits, fcHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/search":
			atomic.AddInt32(&geoHits, 1)
			if r.URL.Query().Get("name") == "Atlantis" {
				io.WriteString(w, `{}`)
				return
			}
			io.WriteString(w, `{"results":[{"latitude":52.5,"longitude":13.4}]}`)
		case "/v1/forecast":
			atomic.AddInt32(&fcHits, 1)
			io.WriteString(w, `{"hourly":{"temperature_2m":[1.5,2,3,4,5,6,7,8,9,10,11,12,13.5,14],"weather_code":[0,0,0,0,0,0,0,0,0,0,0,0,73,0]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	om, err := NewOpenMeteo(OpenMeteoConfig{GeocodeURL: srv.URL, ForecastURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	f, err := om.Forecast(context.Background(), "Berlin", 0)
	if err != nil || f.Main != "sunny" || f.TempC != 1.5 {
		t.Fatalf("%v %v", f, err)
	}
	f, err = om.Forecast(context.Background(), "Berlin", 12)
	if err != nil || f.Main != "snow" || f.TempC != 13.5 {
		t.Fatalf("later=12: %v %v", f, err)
	}
	before := atomic.LoadInt32(&fcHits)
	om.Forecast(context.Background(), "Berlin", 12) // cached
	if atomic.LoadInt32(&fcHits) != before {
		t.Fatal("result not cached")
	}
	if _, err := om.Forecast(context.Background(), "Atlantis", 0); err == nil {
		t.Fatal("unknown city accepted")
	}
	if _, err := om.Forecast(context.Background(), "  ", 0); err == nil {
		t.Fatal("blank city accepted")
	}
	if _, err := NewOpenMeteo(OpenMeteoConfig{GeocodeURL: "https://geocoding.living.ai"}); err == nil {
		t.Fatal("vendor weather URL accepted")
	}
}

// Only guard.go may construct HTTP clients, and nothing here may use the default client or raw dialling.
func TestBackendSourceDiscipline(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		s := string(b)
		for _, bad := range []string{"http.Get(", "http.Post(", "http.PostForm(", "http.Head(", "http.DefaultClient", "net.Dial", "tls.Dial", "InsecureSkipVerify"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s uses %s", f, bad)
			}
		}
		if f != "guard.go" && (strings.Contains(s, "http.Client{") || strings.Contains(s, "http.Transport{")) {
			t.Errorf("%s builds its own HTTP client; use NewGuardedClient", f)
		}
		if f != "guard.go" && strings.Contains(s, "living") {
			t.Errorf("%s mentions the vendor; only guard.go may (it blocks them)", f)
		}
	}
}
