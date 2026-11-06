package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const secret = "SECRETKEY-do-not-leak-123"

func jsonBody(r *http.Request) map[string]any {
	var m map[string]any
	json.NewDecoder(r.Body).Decode(&m)
	return m
}

func TestAnthropic(t *testing.T) {
	t.Setenv("T_ANTHROPIC_KEY", secret)
	var hdr http.Header
	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr, path, body = r.Header.Clone(), r.URL.Path, jsonBody(r)
		io.WriteString(w, `{"content":[{"type":"text","text":" Hello "},{"type":"text","text":"there. "},{"type":"tool_use","name":"x"}]}`)
	}))
	defer srv.Close()
	a, err := NewAnthropic(AnthropicConfig{BaseURL: srv.URL, Model: "m-small", APIKeyEnv: "T_ANTHROPIC_KEY", System: "be brief"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Chat(context.Background(), []Message{{Role: "system", Content: "also JSON"}, {Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"}, {Role: "user", Content: "what is this", ImageJPEG: []byte{0xFF, 0xD8, 1}}})
	if err != nil || out != "Hello there." {
		t.Fatalf("%q %v", out, err)
	}
	if path != "/v1/messages" || hdr.Get("x-api-key") != secret || hdr.Get("anthropic-version") != "2023-06-01" || hdr.Get("Authorization") != "" {
		t.Fatalf("%s %v", path, hdr)
	}
	if body["model"] != "m-small" || body["max_tokens"].(float64) != 200 || body["system"] != "be brief\n\nalso JSON" {
		t.Fatalf("%v", body)
	}
	msgs := body["messages"].([]any)
	if len(msgs) != 3 || msgs[0].(map[string]any)["role"] != "user" || msgs[1].(map[string]any)["role"] != "assistant" {
		t.Fatalf("system messages must be lifted out and roles kept: %v", msgs)
	}
	img := msgs[2].(map[string]any)["content"].([]any)
	src := img[0].(map[string]any)["source"].(map[string]any)
	if img[0].(map[string]any)["type"] != "image" || src["media_type"] != "image/jpeg" || src["data"] != base64.StdEncoding.EncodeToString([]byte{0xFF, 0xD8, 1}) || img[1].(map[string]any)["type"] != "text" {
		t.Fatalf("image block: %v", img)
	}
	if _, err := NewAnthropic(AnthropicConfig{BaseURL: srv.URL}); err == nil {
		t.Fatal("model is required")
	}
	if _, err := a.Chat(context.Background(), []Message{{Role: "system", Content: "only a system message"}}); err == nil {
		t.Fatal("empty conversation accepted")
	}
}

func TestOnlineErrorsNeverLeakKeys(t *testing.T) {
	t.Setenv("T_KEY", secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid key "+secret+" for "+r.URL.String(), 401)
	}))
	defer srv.Close()
	ctx := context.Background()
	check := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: error expected", name)
		} else if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "key=") || strings.Contains(err.Error(), "appid") {
			t.Errorf("%s leaked a secret: %v", name, err)
		}
	}
	a, _ := NewAnthropic(AnthropicConfig{BaseURL: srv.URL, Model: "m", APIKeyEnv: "T_KEY"})
	_, err := a.Chat(ctx, []Message{{Role: "user", Content: "x"}})
	check("anthropic", err)
	gs, _ := NewGoogleSTT(GoogleConfig{BaseURL: srv.URL, APIKeyEnv: "T_KEY"})
	_, err = gs.Transcribe(ctx, []byte("RIFF"), "en")
	check("google stt", err)
	gt, _ := NewGoogleTTS(GoogleConfig{BaseURL: srv.URL, APIKeyEnv: "T_KEY"})
	_, err = gt.Synthesize(ctx, "x", "en")
	check("google tts", err)
	el, _ := NewElevenLabs(ElevenLabsConfig{BaseURL: srv.URL, APIKeyEnv: "T_KEY", VoiceID: "v"})
	_, err = el.Synthesize(ctx, "x", "en")
	check("elevenlabs", err)
	ow, _ := NewOpenWeatherMap(OpenWeatherMapConfig{BaseURL: srv.URL, APIKeyEnv: "T_KEY"})
	_, err = ow.Forecast(ctx, "Bonn", 0)
	check("openweathermap", err)
	wa, _ := NewWeatherAPI(WeatherAPIConfig{BaseURL: srv.URL, APIKeyEnv: "T_KEY"})
	_, err = wa.Forecast(ctx, "Bonn", 0)
	check("weatherapi", err)
	po, _ := NewPollinations(PollinationsConfig{BaseURL: srv.URL})
	_, err = po.Generate(ctx, ImageRequest{Prompt: "x"})
	check("pollinations", err)
}

func TestGoogleSpeech(t *testing.T) {
	t.Setenv("T_GKEY", secret)
	var path, key string
	var body map[string]any
	mp3 := base64.StdEncoding.EncodeToString([]byte{0xFF, 0xFB, 0x90, 0xC4, 1, 2})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, key, body = r.URL.Path, r.URL.Query().Get("key"), jsonBody(r)
		if strings.HasSuffix(r.URL.Path, ":recognize") {
			io.WriteString(w, `{"results":[{"alternatives":[{"transcript":" xin chào "}]},{"alternatives":[{"transcript":"EMO"}]}]}`)
			return
		}
		if strings.Contains(string(mustJSON(body)), `"bad":true`) {
			io.WriteString(w, `{"audioContent":"`+base64.StdEncoding.EncodeToString([]byte("not audio"))+`"}`)
			return
		}
		io.WriteString(w, `{"audioContent":"`+mp3+`"}`)
	}))
	defer srv.Close()
	stt, _ := NewGoogleSTT(GoogleConfig{BaseURL: srv.URL, APIKeyEnv: "T_GKEY"})
	txt, err := stt.Transcribe(context.Background(), []byte("RIFFwav"), "vi")
	if err != nil || txt != "xin chào EMO" {
		t.Fatalf("%q %v", txt, err)
	}
	cfg := body["config"].(map[string]any)
	aud := body["audio"].(map[string]any)["content"].(string)
	if path != "/v1/speech:recognize" || key != secret || cfg["languageCode"] != "vi-VN" || cfg["encoding"] != "LINEAR16" || cfg["sampleRateHertz"].(float64) != 16000 ||
		aud != base64.StdEncoding.EncodeToString([]byte("RIFFwav")) {
		t.Fatalf("%s %v", path, body)
	}
	tts, _ := NewGoogleTTS(GoogleConfig{BaseURL: srv.URL, APIKeyEnv: "T_GKEY", Voice: "voice-x", Rate: 1.1})
	b, err := tts.Synthesize(context.Background(), "hello", "de")
	if err != nil || !IsMP3(b) {
		t.Fatalf("%v %v", b, err)
	}
	v := body["voice"].(map[string]any)
	ac := body["audioConfig"].(map[string]any)
	if path != "/v1/text:synthesize" || v["languageCode"] != "de-DE" || v["name"] != "voice-x" || ac["audioEncoding"] != "MP3" || ac["speakingRate"].(float64) != 1.1 ||
		body["input"].(map[string]any)["text"] != "hello" {
		t.Fatalf("%s %v", path, body)
	}
	if _, err := tts.Synthesize(context.Background(), "bad", "en"); err != nil {
		t.Log("(the mock only flags a body containing bad:true)")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestElevenLabs(t *testing.T) {
	t.Setenv("T_EL", secret)
	var path, rawPath, format string
	var hdr http.Header
	var body map[string]any
	reply := []byte{0xFF, 0xFB, 0x90, 0xC4, 9}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, rawPath, format, hdr, body = r.URL.Path, r.URL.EscapedPath(), r.URL.Query().Get("output_format"), r.Header.Clone(), jsonBody(r)
		w.Write(reply)
	}))
	defer srv.Close()
	e, err := NewElevenLabs(ElevenLabsConfig{BaseURL: srv.URL, APIKeyEnv: "T_EL", VoiceID: "voice/1 id", Model: "fast-model"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Synthesize(context.Background(), "hi there", "en")
	if err != nil || string(b) != string(reply) {
		t.Fatal(err)
	}
	if path != "/v1/text-to-speech/voice/1 id" || rawPath != "/v1/text-to-speech/voice%2F1%20id" { // escaped exactly once
		t.Fatalf("voice id escaping: decoded %q raw %q", path, rawPath)
	}
	if format != "mp3_22050_32" || hdr.Get("xi-api-key") != secret || body["text"] != "hi there" || body["model_id"] != "fast-model" {
		t.Fatalf("%q %v %v", format, hdr, body)
	}
	if _, err := NewElevenLabs(ElevenLabsConfig{BaseURL: srv.URL}); err == nil {
		t.Fatal("voice id is required")
	}
}

func TestOpenWeatherMap(t *testing.T) {
	t.Setenv("T_OWM", secret)
	var hits int32
	var q map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		q = map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		switch r.URL.Path {
		case "/data/2.5/weather":
			io.WriteString(w, `{"weather":[{"main":"Clouds"}],"main":{"temp":12.34}}`)
		case "/data/2.5/forecast":
			io.WriteString(w, `{"list":[{"weather":[{"main":"Clear"}],"main":{"temp":1}},{"weather":[{"main":"Clear"}],"main":{"temp":2}},{"weather":[{"main":"Clear"}],"main":{"temp":3}},
			  {"weather":[{"main":"Clear"}],"main":{"temp":4}},{"weather":[{"main":"Snow"}],"main":{"temp":-1.5}},{"weather":[{"main":"Rain"}],"main":{"temp":9}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	o, _ := NewOpenWeatherMap(OpenWeatherMapConfig{BaseURL: srv.URL, APIKeyEnv: "T_OWM"})
	f, err := o.Forecast(context.Background(), "Hà Nội", 0)
	if err != nil || f.Main != "cloudy" || f.TempC != 12.34 {
		t.Fatalf("%+v %v", f, err)
	}
	if q["q"] != "Hà Nội" || q["units"] != "metric" || q["appid"] != secret {
		t.Fatalf("%v", q)
	}
	f, err = o.Forecast(context.Background(), "Hà Nội", 12)
	if err != nil || f.Main != "snow" || f.TempC != -1.5 { // +12 h is the 5th three-hourly entry
		t.Fatalf("%+v %v", f, err)
	}
	n := atomic.LoadInt32(&hits)
	o.Forecast(context.Background(), "hà nội", 12) // cached, case-insensitive
	if atomic.LoadInt32(&hits) != n {
		t.Fatal("not cached")
	}
	if _, err := o.Forecast(context.Background(), "", 0); err == nil {
		t.Fatal("blank city accepted")
	}
	for in, want := range map[string]string{"Clear": "sunny", "Clouds": "cloudy", "Rain": "rain", "Drizzle": "rain", "Thunderstorm": "rain", "Snow": "snow",
		"Mist": "fog", "Fog": "fog", "Haze": "fog", "Dust": "dust", "Smoke": "dust", "Sand": "dust", "Weird": "cloudy"} {
		if owmMain(in) != want {
			t.Errorf("owmMain(%s) = %s", in, owmMain(in))
		}
	}
}

func TestWeatherAPI(t *testing.T) {
	t.Setenv("T_WA", secret)
	var q map[string]string
	now := int64(1_800_000_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		hours := ""
		for h := 0; h < 48; h++ {
			code := 1000
			if h == 12 {
				code = 1210 // light snow
			}
			if h == 11 || h == 13 {
				code = 1183 // light rain
			}
			hours += `{"time_epoch":` + itoa(now+int64(h)*3600) + `,"temp_c":` + itoa(int64(h)) + `,"condition":{"code":` + itoa(int64(code)) + `}},`
		}
		io.WriteString(w, `{"location":{"localtime_epoch":`+itoa(now)+`},"current":{"temp_c":20.5,"condition":{"code":1003}},"forecast":{"forecastday":[{"hour":[`+strings.TrimSuffix(hours, ",")+`]}]}}`)
	}))
	defer srv.Close()
	w, _ := NewWeatherAPI(WeatherAPIConfig{BaseURL: srv.URL, APIKeyEnv: "T_WA"})
	f, err := w.Forecast(context.Background(), "Berlin", 0)
	if err != nil || f.Main != "cloudy" || f.TempC != 20.5 || q["key"] != secret || q["q"] != "Berlin" {
		t.Fatalf("%+v %v %v", f, err, q)
	}
	f, err = w.Forecast(context.Background(), "Berlin", 12)
	if err != nil || f.Main != "snow" || f.TempC != 12 {
		t.Fatalf("+12h: %+v %v", f, err)
	}
	for code, want := range map[int]string{1000: "sunny", 1003: "cloudy", 1009: "cloudy", 1030: "fog", 1135: "fog", 1063: "rain", 1195: "rain", 1276: "rain",
		1066: "snow", 1225: "snow", 1258: "snow", 1114: "snow", 9999: "cloudy"} {
		if weatherAPIMain(code) != want {
			t.Errorf("code %d -> %s", code, weatherAPIMain(code))
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestPollinations(t *testing.T) {
	png, _ := MockImage{}.Generate(context.Background(), ImageRequest{Prompt: "x"})
	var path, decoded, rawq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, decoded, rawq = r.URL.EscapedPath(), r.URL.Path, r.URL.RawQuery
		w.Write(png)
	}))
	defer srv.Close()
	p, err := NewPollinations(PollinationsConfig{BaseURL: srv.URL, Width: 256, Height: 256, Model: "flux"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Generate(context.Background(), ImageRequest{Prompt: "a robot, 2d/anime"})
	if err != nil || !isImage(b) {
		t.Fatal(err)
	}
	if decoded != "/prompt/a robot, 2d/anime" || path != "/prompt/a%20robot%2C%202d%2Fanime" || // escaped exactly once; the slash stays inside the prompt
		!strings.Contains(rawq, "width=256") || !strings.Contains(rawq, "model=flux") || !strings.Contains(rawq, "nologo=true") {
		t.Fatalf("%s ?%s", path, rawq)
	}
	if _, err := p.Generate(context.Background(), ImageRequest{Prompt: "x", Init: []byte{1}}); err != ErrUnsupported {
		t.Fatalf("img2img: %v", err)
	}
	notImg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>busy</html>") }))
	defer notImg.Close()
	p2, _ := NewPollinations(PollinationsConfig{BaseURL: notImg.URL})
	if _, err := p2.Generate(context.Background(), ImageRequest{Prompt: "x"}); err == nil {
		t.Fatal("html accepted as an image")
	}
}

func TestBCP47(t *testing.T) {
	for in, want := range map[string]string{"en": "en-US", "vi": "vi-VN", "de": "de-DE", "pt": "pt-PT", "": "en-US", "xx": "xx", "VI": "vi-VN"} {
		if bcp47(in) != want {
			t.Errorf("%q -> %q", in, bcp47(in))
		}
	}
}

// Every online provider is bound to its own host: a misconfigured base URL pointing at the robot vendor is refused.
func TestOnlineProvidersRefuseVendorHosts(t *testing.T) {
	v := "https://api.living.ai"
	if _, err := NewAnthropic(AnthropicConfig{BaseURL: v, Model: "m"}); err == nil {
		t.Error("anthropic")
	}
	if _, err := NewGoogleSTT(GoogleConfig{BaseURL: v}); err == nil {
		t.Error("google stt")
	}
	if _, err := NewGoogleTTS(GoogleConfig{BaseURL: v}); err == nil {
		t.Error("google tts")
	}
	if _, err := NewElevenLabs(ElevenLabsConfig{BaseURL: v, VoiceID: "x"}); err == nil {
		t.Error("elevenlabs")
	}
	if _, err := NewOpenWeatherMap(OpenWeatherMapConfig{BaseURL: v}); err == nil {
		t.Error("owm")
	}
	if _, err := NewWeatherAPI(WeatherAPIConfig{BaseURL: v}); err == nil {
		t.Error("weatherapi")
	}
	if _, err := NewPollinations(PollinationsConfig{BaseURL: v}); err == nil {
		t.Error("pollinations")
	}
}
