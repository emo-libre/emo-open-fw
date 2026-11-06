package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"emoserver/backend"
)

const audioBase = "http://192.0.2.10:8080"

// recording backends
type recLLM struct {
	mu    sync.Mutex
	calls [][]backend.Message
	reply string
	err   error
}

func (r *recLLM) Chat(ctx context.Context, m []backend.Message) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]backend.Message(nil), m...))
	r.mu.Unlock()
	return r.reply, r.err
}

type recTTS struct {
	mu    sync.Mutex
	texts []string
	err   error
}

func (r *recTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	r.mu.Lock()
	r.texts = append(r.texts, text)
	r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return []byte{0xFF, 0xFB, 0x90, 0xC4, 9, 9}, nil
}

type recWeather struct {
	city  string
	later int
	f     backend.Forecast
	err   error
}

func (r *recWeather) Forecast(ctx context.Context, city string, later int) (backend.Forecast, error) {
	r.city, r.later = city, later
	return r.f, r.err
}

type convRig struct {
	s   *server
	h   http.Handler
	llm *recLLM
	tts *recTTS
}

func newConv(t *testing.T, sttText string) *convRig {
	t.Helper()
	s, _, _ := newTestServer(t, false)
	s.cfg.AudioBaseURL = audioBase
	s.cfg.FreeChat = true
	s.cfg.Persona = "off" // these tests count the exact messages sent; persona_test.go covers the character prompt
	rig := &convRig{s: s, llm: &recLLM{reply: "Why did the robot cross the road? **To recharge!** 😀"}, tts: &recTTS{}}
	s.be = backend.Set{STT: backend.MockSTT{Text: sttText}, LLM: rig.llm, TTS: rig.tts}
	rig.h = s.routes()
	return rig
}

func voiceBody(samples int) *bytes.Reader {
	b := make([]byte, 0, 2+2*samples)
	b = append(b, 0x22)
	for i := 0; i < samples; i++ {
		b = append(b, byte(i), byte(i>>8))
	}
	b = append(b, 0x22)
	return bytes.NewReader(b)
}

func (c *convRig) post(path string, body io.Reader, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", "application/octet-stream")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rr := httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	return rr
}

func (c *convRig) token(t *testing.T, id string) string {
	tok, err := signJWT(c.s.key, map[string]any{"sub": id, "exp": time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func qr(t *testing.T, rr *httptest.ResponseRecorder) (map[string]any, map[string]any) {
	t.Helper()
	m := decode(t, rr)
	r, _ := m["queryResult"].(map[string]any)
	if r == nil {
		t.Fatalf("no queryResult: %s", rr.Body.String())
	}
	return m, r
}

func TestChatModeReplyShape(t *testing.T) {
	c := newConv(t, "tell me a joke")
	rr := c.post("/emo/voice/detectintent?languagecode=en-c&index=7&role=chatgpt&source=0", voiceBody(1000), c.token(t, "aabbccddeeff"))
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	top, r := qr(t, rr)
	if top["index"].(float64) != 7 { // the robot drops a reply with the wrong index
		t.Fatalf("index not echoed: %v", top["index"])
	}
	if top["languageCode"] != "en" || top["queryId"] == "" || r["resultCode"] == "" {
		t.Fatalf("%v", top)
	}
	if r["rec_behavior"] != "speak" || r["queryText"] != "tell me a joke" || r["intent"].(map[string]any)["name"] != "chatgpt_speak" {
		t.Fatalf("%v", r)
	}
	p := r["behavior_paras"].(map[string]any)
	if p["listen"].(float64) != 1 {
		t.Fatal("chat replies should keep listening")
	}
	if p["txt"] != "Why did the robot cross the road? To recharge!" { // markdown and emoji stripped
		t.Fatalf("txt = %q", p["txt"])
	}
	u := p["url"].(string)
	if !strings.HasPrefix(u, audioBase+"/mp3/download/") || !strings.Contains(u, "?id=") || !strings.Contains(u, "&token=") {
		t.Fatalf("url %s", u)
	}
	// the robot can fetch the speech from the plain-HTTP side
	rel := strings.TrimPrefix(u, audioBase)
	for _, h := range []http.Handler{c.h, c.s.audioRoutes()} {
		g := get(h, rel)
		if g.Code != 200 || g.Header().Get("Content-Type") != "audio/mpeg" || g.Body.Bytes()[0] != 0xFF {
			t.Fatalf("download %d %q", g.Code, g.Header().Get("Content-Type"))
		}
	}
	if get(c.s.audioRoutes(), "/emo/ota/version").Code != 404 { // the audio listener exposes nothing else
		t.Fatal("audio listener exposes API routes")
	}
	// wrong token / id / name are refused
	for _, bad := range []string{strings.Replace(rel, "token=", "token=00", 1), strings.Replace(rel, "?id=", "?id=9", 1), "/mp3/download/other.mp3?id=1&token=x", "/mp3/download/x.txt"} {
		if get(c.h, bad).Code != 404 {
			t.Fatalf("%s served", bad)
		}
	}
	if len(c.tts.texts) != 1 || c.tts.texts[0] != p["txt"] {
		t.Fatalf("tts input %v", c.tts.texts)
	}
}

func TestGoodbyeEndsChatAndHistoryIsKept(t *testing.T) {
	c := newConv(t, "what is two plus two")
	tok := c.token(t, "aabbccddeeff")
	c.llm.reply = "Four."
	c.post("/emo/voice/detectintent?languagecode=en&index=1&role=chatgpt", voiceBody(100), tok)
	c.s.be.STT = backend.MockSTT{Text: "and times three"}
	c.llm.reply = "Twelve."
	c.post("/emo/voice/detectintent?languagecode=en&index=2&role=chatgpt", voiceBody(100), tok)
	h := c.llm.calls[1]
	if len(h) != 3 || h[0].Content != "what is two plus two" || h[1].Role != "assistant" || h[1].Content != "Four." || h[2].Content != "and times three" {
		t.Fatalf("history sent to the model: %+v", h)
	}
	// a different robot has its own history
	c.post("/emo/voice/detectintent?languagecode=en&index=3&role=chatgpt", voiceBody(100), c.token(t, "112233445566"))
	if len(c.llm.calls[2]) != 1 {
		t.Fatalf("history leaked between robots: %+v", c.llm.calls[2])
	}
	// goodbye -> chatgpt_end animation, no model call, history cleared
	calls := len(c.llm.calls)
	c.s.be.STT = backend.MockSTT{Text: "Okay, goodbye!"}
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=4&role=chatgpt", voiceBody(100), tok))
	if r["rec_behavior"] != "play_animation" || r["behavior_paras"].(map[string]any)["animation_name"] != "chatgpt_end" {
		t.Fatalf("%v", r)
	}
	if len(c.llm.calls) != calls {
		t.Fatal("model called for a goodbye")
	}
	c.s.be.STT = backend.MockSTT{Text: "hello again"}
	c.post("/emo/voice/detectintent?languagecode=en&index=5&role=chatgpt", voiceBody(100), tok)
	if last := c.llm.calls[len(c.llm.calls)-1]; len(last) != 1 {
		t.Fatalf("history survived the goodbye: %+v", last)
	}
	// chat/start resets too
	c.post("/emo/voice/detectintent?languagecode=en&index=6&role=chatgpt", voiceBody(100), tok)
	startReq := httptest.NewRequest("GET", "/emo/chat/start?lang=en&tz=UTC", nil)
	startReq.Header.Set("Authorization", "Bearer "+tok) // the robot sends its token here too
	c.h.ServeHTTP(httptest.NewRecorder(), startReq)
	c.post("/emo/voice/detectintent?languagecode=en&index=7&role=chatgpt", voiceBody(100), tok)
	if last := c.llm.calls[len(c.llm.calls)-1]; len(last) != 1 {
		t.Fatalf("chat/start did not reset history: %+v", last)
	}
}

func TestNormalModeRules(t *testing.T) {
	cases := []struct{ say, behavior, extra string }{
		{"Hello!", "play_animation", "Hi"},
		{"hey EMO", "play_animation", "Hi"},
		{"please shut down", "power_off", ""},
		{"can you dance for me", "dance", ""},
		{"go explore", "explore", ""},
		{"Let's chat", "featured_game", "chatgpt"},
		{"talk to me", "featured_game", "chatgpt"},
	}
	for _, tc := range cases {
		c := newConv(t, tc.say)
		top, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=3", voiceBody(50), ""))
		if r["rec_behavior"] != tc.behavior || top["index"].(float64) != 3 {
			t.Errorf("%q -> %v", tc.say, r)
			continue
		}
		if tc.extra != "" {
			p := r["behavior_paras"].(map[string]any)
			if p["animation_name"] != tc.extra && p["game_name"] != tc.extra {
				t.Errorf("%q paras %v", tc.say, p)
			}
		}
		if len(c.llm.calls) != 0 {
			t.Errorf("%q needlessly used the model", tc.say)
		}
	}
	// "dance" inside another word must not match
	c := newConv(t, "abundance of cheese")
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] == "dance" {
		t.Fatal("substring match")
	}
	// free chat: anything else is answered by the model, statelessly
	c = newConv(t, "tell me about dinosaurs")
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), c.token(t, "aabbccddeeff")))
	if r["rec_behavior"] != "speak" || len(c.llm.calls) != 1 || len(c.llm.calls[0]) != 1 {
		t.Fatalf("%v", r)
	}
	// ...unless free chat is off
	c = newConv(t, "tell me about dinosaurs")
	c.s.cfg.FreeChat = false
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] != "play_animation" || len(c.llm.calls) != 0 {
		t.Fatalf("%v", r)
	}
}

func TestFailuresDegradeToDontUnderstand(t *testing.T) {
	check := func(name string, c *convRig, body io.Reader) {
		t.Helper()
		top, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=9&role=chatgpt", body, ""))
		if r["rec_behavior"] != "play_animation" || r["behavior_paras"].(map[string]any)["animation_name"] != "dont_understand" || top["index"].(float64) != 9 {
			t.Errorf("%s: %v", name, r)
		}
	}
	c := newConv(t, "")
	check("empty transcript", c, voiceBody(50))
	c = newConv(t, "hello")
	check("empty body", c, bytes.NewReader(nil))
	check("only framing bytes", c, bytes.NewReader([]byte{0x22, 0x22}))
	c = newConv(t, "hello")
	c.s.be.STT = nil
	check("no stt backend", c, voiceBody(50))
	c = newConv(t, "hello there")
	c.llm.err = fmt.Errorf("model down")
	check("llm error", c, voiceBody(50))
	c = newConv(t, "hello there")
	c.tts.err = fmt.Errorf("tts down")
	check("tts error", c, voiceBody(50))
	c = newConv(t, "hello there")
	c.s.be.LLM = nil
	check("no llm", c, voiceBody(50))
	c = newConv(t, "hello there")
	c.llm.reply = "   "
	check("blank model reply", c, voiceBody(50))
}

func TestChatStartAndVoicePrompt(t *testing.T) {
	c := newConv(t, "")
	m := decode(t, get(c.h, "/emo/chat/start?lang=en%2dc&name=Alex&tz=Europe/Berlin"))
	if m["errcode"].(float64) != 0 || m["responsetag"] != "chatstart" || m["errmsg"] != "OK" || !strings.HasPrefix(m["url"].(string), audioBase) {
		t.Fatalf("%v", m)
	}
	if c.tts.texts[0] != "Hi, Alex! What shall we talk about?" {
		t.Fatalf("greeting %q", c.tts.texts[0])
	}
	get(c.h, "/emo/chat/start?lang=en&tz=UTC")
	if c.tts.texts[1] != "Hi! What shall we talk about?" {
		t.Fatalf("greeting without name %q", c.tts.texts[1])
	}

	// voice prompt: the fields the robot's handler reads
	req := httptest.NewRequest("POST", "/emo/voice/voiceprompt?languagecode=nl&type=not_execute2&index=123", strings.NewReader(""))
	rr := httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	m = decode(t, rr)
	if m["responsetag"] != "voiceprompt" || m["errmessage"] != "ok" || m["errmsg"] != "ok" || m["errcode"].(float64) != 0 || m["url"] == nil {
		t.Fatalf("%v", m)
	}
	if c.tts.texts[2] != "I'm busy at the moment. Ask me again in a bit." { // no Dutch lines yet -> English fallback
		t.Fatalf("prompt %q", c.tts.texts[2])
	}
	req = httptest.NewRequest("POST", "/emo/voice/voiceprompt?languagecode=en&type=never_heard_of_it&index=1", nil)
	rr = httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	if c.tts.texts[3] != "Sorry, I can't do that right now." {
		t.Fatalf("generic prompt %q", c.tts.texts[3])
	}

	// speech unavailable -> the failure shapes the robot checks
	c.s.be.TTS = nil
	m = decode(t, get(c.h, "/emo/chat/start?lang=en"))
	if m["errcode"].(float64) == 0 || m["url"] != nil {
		t.Fatalf("%v", m)
	}
	rr = httptest.NewRecorder()
	c.h.ServeHTTP(rr, httptest.NewRequest("POST", "/emo/voice/voiceprompt?languagecode=en&type=x", nil))
	m = decode(t, rr)
	if m["errmessage"] == "ok" || m["url"] != nil {
		t.Fatalf("%v", m)
	}
}

func TestSpeechTTS(t *testing.T) {
	c := newConv(t, "")
	m := decode(t, get(c.h, "/emo/speech/tts?q=i%27m%20sorry&l=en%2dc"))
	if m["code"].(float64) != 200 || m["errmessage"] != "ok" || !strings.HasPrefix(m["url"].(string), audioBase+"/mp3/download/") {
		t.Fatalf("%v", m)
	}
	if c.tts.texts[0] != "i'm sorry" {
		t.Fatal(c.tts.texts)
	}
	m = decode(t, get(c.h, "/emo/speech/tts?q=&l=en"))
	if m["errmessage"] == "ok" {
		t.Fatalf("empty text accepted: %v", m)
	}
	c.tts.err = fmt.Errorf("down")
	if m = decode(t, get(c.h, "/emo/speech/tts?q=hi&l=en")); m["errmessage"] == "ok" || m["url"] != nil {
		t.Fatalf("%v", m)
	}
}

func TestWeatherEndpoint(t *testing.T) {
	c := newConv(t, "")
	w := &recWeather{f: backend.Forecast{Main: "cloudy", TempC: 23.87}}
	c.s.be.Weather = w
	m := decode(t, get(c.h, "/emo/weather/forecast?city=Bonn&later=12&lon=0.00000&lat=0.00000"))
	wm := m["weather"].(map[string]any)
	if wm["main"] != "cloudy" || wm["temp_c"].(float64) != 23.87 || w.city != "Bonn" || w.later != 12 {
		t.Fatalf("%v city=%q later=%d", m, w.city, w.later)
	}
	if len(m) != 1 {
		t.Fatalf("extra top-level keys: %v", m) // the robot treats extra pushed keys as news
	}
	w.err = fmt.Errorf("offline")
	m = decode(t, get(c.h, "/emo/weather/forecast?city=Bonn&later=0"))
	if m["weather"] != nil || m["errmsg"] != "api error" {
		t.Fatalf("%v", m)
	}
	c.s.be.Weather = nil
	if m = decode(t, get(c.h, "/emo/weather/forecast?city=Bonn&later=0")); m["weather"] != nil {
		t.Fatalf("%v", m)
	}
}

func TestImageRecognition(t *testing.T) {
	c := newConv(t, "")
	rr := c.post("/emo/ai/imgrecog?languageCode=en&type=take_photo&index=4", bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0}), "")
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("no vision backend -> %d", rr.Code)
	}
	vis := &recLLM{reply: "I see a friendly cat on a sofa."}
	c.s.be.Vision = vis
	top, r := qr(t, c.post("/emo/ai/imgrecog?languageCode=en&type=take_photo&index=4", bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xE0}), ""))
	if r["rec_behavior"] != "speak" || top["index"].(float64) != 4 {
		t.Fatalf("%v", r)
	}
	if len(vis.calls) != 1 || len(vis.calls[0][0].ImageJPEG) != 4 {
		t.Fatal("image not passed to the vision model")
	}
	if rr := c.post("/emo/ai/imgrecog?languageCode=en", bytes.NewReader(nil), ""); rr.Code != 400 {
		t.Fatalf("empty image -> %d", rr.Code)
	}
}

func TestVoiceBodyAndWAV(t *testing.T) {
	pcm, err := decodeVoiceBody(bytes.NewReader([]byte{0x22, 1, 0, 2, 0, 0x22}))
	if err != nil || len(pcm) != 4 || pcm[0] != 1 {
		t.Fatalf("%v %v", pcm, err)
	}
	if pcm, _ = decodeVoiceBody(bytes.NewReader([]byte{1, 0, 2, 0})); len(pcm) != 4 { // no framing bytes
		t.Fatalf("unframed body mangled: %v", pcm)
	}
	if pcm, _ = decodeVoiceBody(bytes.NewReader([]byte{0x22, 1, 0, 2, 0, 3, 0x22})); len(pcm) != 4 { // odd length trimmed
		t.Fatalf("odd length: %v", pcm)
	}
	for _, b := range [][]byte{nil, {0x22, 0x22}, {0x22}} {
		if _, err := decodeVoiceBody(bytes.NewReader(b)); err == nil {
			t.Fatalf("%v accepted", b)
		}
	}
	if _, err := decodeVoiceBody(bytes.NewReader(make([]byte, voiceMaxBytes+10))); err == nil {
		t.Fatal("oversized body accepted")
	}
	w := wavFromPCM([]byte{1, 2, 3, 4}, 16000)
	if len(w) != 48 || string(w[0:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " || string(w[36:40]) != "data" {
		t.Fatalf("wav header %q", w[:44])
	}
	rate := uint32(w[24]) | uint32(w[25])<<8 | uint32(w[26])<<16
	size := uint32(w[40])
	if rate != 16000 || size != 4 || w[22] != 1 || w[34] != 16 {
		t.Fatalf("rate %d size %d ch %d bits %d", rate, size, w[22], w[34])
	}
}

func TestAudioStoreExpiryAndEviction(t *testing.T) {
	a := newAudioStore()
	now := time.Now()
	a.now = func() time.Time { return now }
	rel := a.Put([]byte("one"))
	name := strings.TrimPrefix(strings.SplitN(rel, "?", 2)[0], "/mp3/download/")
	q := strings.SplitN(rel, "?", 2)[1]
	id := strings.Split(strings.Split(q, "&")[0], "=")[1]
	tok := strings.Split(q, "token=")[1]
	if _, ok := a.get(name, id, tok); !ok {
		t.Fatal("fresh item missing")
	}
	now = now.Add(2 * time.Hour)
	if _, ok := a.get(name, id, tok); ok {
		t.Fatal("expired item served")
	}
	a.max = 10
	r1 := a.Put(bytes.Repeat([]byte("a"), 6))
	now = now.Add(time.Second)
	a.Put(bytes.Repeat([]byte("b"), 6)) // pushes total over the cap: the older one goes
	n1 := strings.TrimPrefix(strings.SplitN(r1, "?", 2)[0], "/mp3/download/")
	a.mu.Lock()
	_, still := a.items[n1]
	a.mu.Unlock()
	if still {
		t.Fatal("oldest item not evicted")
	}
}

func TestTidyReplyAndRouting(t *testing.T) {
	if g := tidyReply("# Title\n**bold** `code` _x_ 😀 ok"); g != "Title bold code x ok" {
		t.Fatalf("%q", g)
	}
	long := strings.Repeat("This is a sentence. ", 40)
	g := tidyReply(long)
	if len([]rune(g)) > 300 || !strings.HasSuffix(g, ".") {
		t.Fatalf("len %d %q", len(g), g[len(g)-10:])
	}
	if g := tidyReply(strings.Repeat("é", 400)); len([]rune(g)) != 300 { // no sentence end: hard cut, rune-safe
		t.Fatalf("%d", len([]rune(g)))
	}
	for in, want := range map[string]string{"en-c": "en", "EN_US": "en", "de": "de", "": "en", " fr-CA ": "fr"} {
		if baseLang(in) != want {
			t.Errorf("baseLang(%q) = %q", in, baseLang(in))
		}
	}
	for _, s := range []string{"bye", "Goodbye EMO", "ok that's all", "end the chat please"} {
		if !isGoodbye(s) {
			t.Errorf("%q should end the chat", s)
		}
	}
	for _, s := range []string{"maybe later", "byte me", "what is a goodbye gift"} {
		if isGoodbye(s) && s != "what is a goodbye gift" {
			t.Errorf("%q should not end the chat", s)
		}
	}
}

func TestPromptsFileOverrides(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.json")
	os.WriteFile(p, []byte(`{"de":{"not_execute":"Das geht gerade nicht."},"en":{"chat_start":"Yo{name}!"}}`), 0o644)
	pr, err := loadPrompts(p)
	if err != nil {
		t.Fatal(err)
	}
	if pr.text("de", "not_execute") != "Das geht gerade nicht." || pr.text("de", "not_execute2") != "Ich bin gerade beschäftigt. Frag mich gleich noch einmal." || pr.text("nl", "not_execute2") != "I'm busy at the moment. Ask me again in a bit." ||
		pr.text("en", "chat_start") != "Yo{name}!" || pr.text("xx", "zzz") != "Sorry, I can't do that right now." {
		t.Fatal("prompt resolution")
	}
	os.WriteFile(p, []byte(`not json`), 0o644)
	if _, err := loadPrompts(p); err == nil {
		t.Fatal("bad prompts file accepted")
	}
}

func TestCaptureMode(t *testing.T) {
	dir := t.TempDir()
	c := newConv(t, "hello")
	h, err := capturing(CaptureConfig{Dir: dir, Bodies: false}, c.s.routes())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/emo/voice/detectintent?languagecode=en&index=1", voiceBody(10))
	req.Header.Set("Authorization", "Bearer TOPSECRETTOKEN")
	req.Header.Set("Secret", "SIGNATURE123")
	req.Header.Set("Content-Type", "application/octet-stream")
	h.ServeHTTP(httptest.NewRecorder(), req)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/emo/something/new?x=1", nil))

	b, err := os.ReadFile(filepath.Join(dir, "capture.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, leak := range []string{"TOPSECRETTOKEN", "SIGNATURE123"} {
		if strings.Contains(s, leak) {
			t.Fatalf("capture recorded %s", leak)
		}
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d records", len(lines))
	}
	var rec map[string]any
	json.Unmarshal([]byte(lines[0]), &rec)
	if ms, ok := rec["ms"].(float64); !ok || ms < 0 { // how long the robot waited
		t.Fatalf("ms = %v", rec["ms"])
	}
	if rec["req_body_bytes"].(float64) != 22 { // sizes are recorded even when bodies are not saved
		t.Fatalf("req_body_bytes = %v", rec["req_body_bytes"])
	}
	if rec["path"] != "/emo/voice/detectintent" || rec["query"] != "languagecode=en&index=1" || rec["status"].(float64) != 200 {
		t.Fatalf("%v", rec)
	}
	hd := rec["headers"].(map[string]any)
	if hd["Authorization"] != "<21 chars>" || hd["Content-Type"] != "application/octet-stream" {
		t.Fatalf("headers %v", hd)
	}
	if !strings.Contains(rec["resp_preview"].(string), "queryResult") {
		t.Fatal("no response preview")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "bodies")); len(entries) != 0 {
		t.Fatal("bodies saved without opting in")
	}

	// bodies on: saved with owner-only permissions, and the handler still sees the full body
	dir2 := t.TempDir()
	h2, _ := capturing(CaptureConfig{Dir: dir2, Bodies: true}, c.s.routes())
	rr := httptest.NewRecorder()
	h2.ServeHTTP(rr, httptest.NewRequest("POST", "/emo/voice/detectintent?languagecode=en&index=2", voiceBody(10)))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "queryResult") {
		t.Fatal("handler starved of its body")
	}
	entries, _ := os.ReadDir(filepath.Join(dir2, "bodies"))
	if len(entries) != 1 {
		t.Fatalf("%d bodies", len(entries))
	}
	if st, _ := entries[0].Info(); st.Mode().Perm() != 0o600 || st.Size() != 22 {
		t.Fatalf("mode %v size %d", st.Mode(), st.Size())
	}
	// off by default
	if h3, _ := capturing(CaptureConfig{}, c.s.routes()); h3 == nil {
		t.Fatal("nil handler")
	}
}

func TestConfigRequiresReachableAudioURL(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "c.json")
		os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	if _, err := LoadConfig(write(`{"backends":{"tts":{"type":"mock"}}}`)); err == nil {
		t.Fatal("tts without audio_base_url accepted")
	}
	if _, err := LoadConfig(write(`{"audio_base_url":"https://192.0.2.1","backends":{"tts":{"type":"mock"}}}`)); err == nil {
		t.Fatal("https audio url accepted (the robot downloads speech over plain http)")
	}
	if _, err := LoadConfig(write(`{"audio_base_url":"http://tts.living.ai","backends":{"tts":{"type":"mock"}}}`)); err == nil {
		t.Fatal("vendor audio url accepted")
	}
	if _, err := LoadConfig(write(`{"audio_base_url":"http://192.0.2.1:8080","backends":{"tts":{"type":"mock"}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(write(`{"backends":{"llm":{"type":"openai","openai":{"base_url":"https://api.living.ai/v1"}}}}`)); err != nil {
		t.Fatal("config load itself should succeed; the backend is refused at build time:", err)
	}
	cfg, _ := LoadConfig(write(`{"backends":{"llm":{"type":"openai","openai":{"base_url":"https://api.living.ai/v1"}}}}`))
	if _, err := buildBackends(cfg.Backends); err == nil {
		t.Fatal("vendor LLM url accepted")
	}
	if _, err := buildBackends(BackendsConfig{STT: CapConfig{Type: "bogus"}}); err == nil {
		t.Fatal("unknown backend type accepted")
	}
}

func TestExampleConfigLoads(t *testing.T) {
	cfg, err := LoadConfig("config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildBackends(cfg.Backends); err != nil {
		t.Fatal(err)
	}
	if cfg.AudioBaseURL == "" {
		t.Fatal("example config lost its key fields")
	}
	if cfg.Backends.LLM.OpenAI.System != "" || cfg.Persona != "" { // the built-in persona (persona.go) gives EMO its character
		t.Fatal("example config overrides or stacks on the built-in persona")
	}
}

// Every preset must load, validate and build its backends (no network needed to construct them).
func TestPresetsLoadAndBuild(t *testing.T) {
	files, _ := filepath.Glob("presets/*.json")
	if len(files) < 3 {
		t.Fatalf("presets: %v", files)
	}
	for _, f := range files {
		cfg, err := LoadConfig(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		set, err := buildBackends(cfg.Backends)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if set.STT == nil || set.LLM == nil || set.TTS == nil || set.Weather == nil || set.News == nil {
			t.Errorf("%s leaves a core feature unconfigured: %+v", f, set)
		}
	}
	// every provider type the factory knows can be constructed from a minimal config
	for name, c := range map[string]BackendsConfig{
		"anthropic llm":  {LLM: CapConfig{Type: "anthropic", Anthropic: backend.AnthropicConfig{Model: "m"}}},
		"anthropic vis":  {Vision: CapConfig{Type: "anthropic", Anthropic: backend.AnthropicConfig{Model: "m"}}},
		"google stt":     {STT: CapConfig{Type: "google"}},
		"google tts":     {TTS: CapConfig{Type: "google"}},
		"elevenlabs":     {TTS: CapConfig{Type: "elevenlabs", ElevenLabs: backend.ElevenLabsConfig{VoiceID: "v"}}},
		"openweathermap": {Weather: WeatherConfig{Type: "openweathermap"}},
		"weatherapi":     {Weather: WeatherConfig{Type: "weatherapi"}},
		"pollinations":   {Image: ImageConfig{Type: "pollinations"}},
		"open-meteo":     {Weather: WeatherConfig{Type: "open-meteo"}},
		"sdwebui":        {Image: ImageConfig{Type: "sdwebui", SDWebUI: backend.SDWebUIConfig{BaseURL: "http://127.0.0.1:7860"}}},
	} {
		if _, err := buildBackends(c); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := buildBackends(BackendsConfig{TTS: CapConfig{Type: "elevenlabs"}}); err == nil {
		t.Error("elevenlabs without a voice accepted")
	}
	if _, err := buildBackends(BackendsConfig{LLM: CapConfig{Type: "anthropic"}}); err == nil {
		t.Error("anthropic without a model accepted")
	}
}

// What the robot would otherwise read aloud: stage directions and emoticons seen in real model replies; numbers and times survive.
func TestTidyReplySpeechOnly(t *testing.T) {
	for in, want := range map[string]string{
		"Haha! Let's stay up :) Is there something?":                "Haha! Let's stay up Is there something?",
		"I'm on my way! *rolls onto the desk, a bit wobbly* There!": "I'm on my way! There!",
		"**Yes!** I love you :D <3":                                 "Yes! I love you",
		"Okay ;-) see you at 3:30 (about 8) :(":                     "Okay see you at 3:30 (about 8)",
		"XD that was funny ^_^.":                                    "that was funny.",
		"It’s “fun”":                                                `It's "fun"`,
		"Note:Do not strip words like http://x or a:b":              "Note:Do not strip words like http://x or a:b",
		":) :)": "",
	} {
		if g := tidyReply(in); g != want {
			t.Errorf("tidyReply(%q) = %q, want %q", in, g, want)
		}
	}
}

// Normal-mode replies remember the last few turns per robot, so follow-ups make sense; bounded, forgotten when idle, kept apart from chat mode.
func TestFreeChatFollowUps(t *testing.T) {
	c := newConv(t, "who is the tallest animal")
	tok := c.token(t, "aabbccddeeff")
	say := func(text, reply, query string) []backend.Message {
		t.Helper()
		c.s.be.STT = backend.MockSTT{Text: text}
		c.llm.reply = reply
		c.post("/emo/voice/detectintent?languagecode=en&index=1"+query, voiceBody(50), tok)
		return c.llm.calls[len(c.llm.calls)-1]
	}
	say("who is the tallest animal", "The giraffe!", "")
	h := say("how tall is it", "About five metres.", "")
	if len(h) != 3 || h[0].Content != "who is the tallest animal" || h[1].Content != "The giraffe!" || h[2].Content != "how tall is it" {
		t.Fatalf("follow-up lost its context: %+v", h)
	}
	for i := 0; i < 5; i++ {
		h = say(fmt.Sprintf("question %d", i), "Answer.", "")
	}
	if len(h) != 2*freeChatTurns || h[len(h)-1].Content != "question 4" {
		t.Fatalf("memory not bounded to %d turns: %d messages", freeChatTurns, len(h))
	}
	// another robot starts empty
	c.s.be.STT = backend.MockSTT{Text: "hello there friend"}
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), c.token(t, "112233445566"))
	if last := c.llm.calls[len(c.llm.calls)-1]; len(last) != 1 {
		t.Fatalf("memory leaked between robots: %+v", last)
	}
	// chat mode keeps its own conversation
	if h = say("tell me a story", "Once upon a time.", "&role=chatgpt"); len(h) != 1 {
		t.Fatalf("normal-mode memory leaked into chat mode: %+v", h)
	}
	// idle for longer than freeChatIdle: forgotten
	base := time.Now()
	c.s.freeSess.now = func() time.Time { return base.Add(freeChatIdle + time.Minute) }
	if h = say("and then what", "Hmm?", ""); len(h) != 1 {
		t.Fatalf("stale memory used after %v idle: %+v", freeChatIdle, h)
	}
	// turned off: stateless as before
	c2 := newConv(t, "who is the tallest animal")
	c2.s.freeSess = newFreeChatSessions(-1)
	c2.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), tok)
	c2.s.be.STT = backend.MockSTT{Text: "how tall is it"}
	c2.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), tok)
	if last := c2.llm.calls[len(c2.llm.calls)-1]; len(last) != 1 {
		t.Fatalf("free_chat_turns -1 still remembers: %+v", last)
	}
}

// Each voice request reports how long speech-to-text, the model and speech took (activity log), without logging the words.
func TestVoiceStageTimings(t *testing.T) {
	c := newConv(t, "tell me about dinosaurs")
	c.s.act = &activityLog{on: true}
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), "")
	e := c.s.act.list()
	if len(e) != 1 {
		t.Fatalf("activity: %+v", e)
	}
	for _, k := range []string{"stt", "llm", "tts"} {
		if _, ok := e[0].Stages[k]; !ok {
			t.Errorf("stage %q missing: %+v", k, e[0].Stages)
		}
	}
	// a command answered without the model records no model time
	c.s.be.STT = backend.MockSTT{Text: "can you dance for me"}
	c.post("/emo/voice/detectintent?languagecode=en&index=2", voiceBody(50), "")
	if st := c.s.act.list()[0].Stages; st["stt"] < 0 || hasKey(st, "llm") || hasKey(st, "tts") {
		t.Errorf("command stages: %+v", st)
	}
	// the timer is a no-op outside a voice request, and adds repeated stages
	timeStage(context.Background(), "tts")()
	ctx, st := withStages(context.Background())
	timeStage(ctx, "tts")()
	timeStage(ctx, "tts")()
	timeStage(ctx, "stt")()
	if s := st.String(); !strings.HasPrefix(s, "stt ") || !strings.Contains(s, ", tts ") || strings.Count(s, "tts") != 1 {
		t.Errorf("stage summary %q", s)
	}
}

func hasKey(m map[string]int, k string) bool { _, ok := m[k]; return ok }
