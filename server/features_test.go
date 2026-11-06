package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emoserver/backend"
)

func cachedRig(t *testing.T, say string) *convRig {
	t.Helper()
	c := newConv(t, say)
	cache, err := newTTSCache(filepath.Join(t.TempDir(), "c"), 1)
	if err != nil {
		t.Fatal(err)
	}
	c.s.ttsCache = cache
	c.s.ttsFP = "fp1"
	return c
}

func TestTTSCacheServesFixedLinesWithoutResynthesis(t *testing.T) {
	c := cachedRig(t, "")
	for i := 0; i < 3; i++ { // the robot's scripted line, asked three times
		m := decode(t, get(c.h, "/emo/speech/tts?q=hello%20there&l=en"))
		if m["errmessage"] != "ok" {
			t.Fatal(m)
		}
	}
	if len(c.tts.texts) != 1 {
		t.Fatalf("synthesised %d times, want 1", len(c.tts.texts))
	}
	// refusal prompts too
	for i := 0; i < 2; i++ {
		rr := c.post("/emo/voice/voiceprompt?languagecode=en&type=not_execute&index=1", strings.NewReader(""), "")
		if decode(t, rr)["errmessage"] != "ok" {
			t.Fatal("voiceprompt")
		}
	}
	if len(c.tts.texts) != 2 {
		t.Fatalf("texts %v", c.tts.texts)
	}
	// a different TTS configuration must not reuse the old audio
	c.s.ttsFP = "fp2"
	get(c.h, "/emo/speech/tts?q=hello%20there&l=en")
	if len(c.tts.texts) != 3 {
		t.Fatal("cache shared across TTS configurations")
	}
}

func TestConversationRepliesAreNeverCached(t *testing.T) {
	c := cachedRig(t, "tell me a joke")
	for i := 0; i < 2; i++ {
		c.post("/emo/voice/detectintent?languagecode=en&index=1&role=chatgpt", voiceBody(60), c.token(t, "aabbccddeeff"))
	}
	if len(c.tts.texts) != 2 {
		t.Fatalf("expected 2 syntheses, got %d", len(c.tts.texts))
	}
	ents, _ := os.ReadDir(c.s.ttsCache.dir)
	if len(ents) != 0 {
		t.Fatalf("conversation audio was written to the cache: %d files", len(ents))
	}
	// and the cached directory never contains the text of any line
	get(c.h, "/emo/speech/tts?q=secret%20phrase&l=en")
	ents, _ = os.ReadDir(c.s.ttsCache.dir)
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(c.s.ttsCache.dir, e.Name()))
		if strings.Contains(string(b), "secret phrase") || strings.Contains(e.Name(), "secret") {
			t.Fatal("text stored in the cache")
		}
	}
}

func TestPhotoDescriptionsAreNeverCached(t *testing.T) {
	c := cachedRig(t, "")
	c.s.be.Vision = &recLLM{reply: "I see a friendly cat on a sofa."}
	c.post("/emo/ai/imgrecog?languageCode=en&type=take_photo&index=4", strings.NewReader("\xff\xd8\xff\xe0"), "")
	if len(c.tts.texts) != 1 {
		t.Fatalf("texts %v", c.tts.texts)
	}
	if ents, _ := os.ReadDir(c.s.ttsCache.dir); len(ents) != 0 {
		t.Fatalf("photo description cached: %d files", len(ents))
	}
}

func TestTTSCachePruneAndCorruption(t *testing.T) {
	cache, _ := newTTSCache(t.TempDir(), 1)
	cache.max = 1 << 20 // no pruning while filling
	mp3 := func(n int) []byte { b := make([]byte, n); b[0], b[1] = 0xFF, 0xFB; return b }
	now := time.Now()
	for i := 0; i < 5; i++ {
		k := cacheKey("fp", "en", string(rune('a'+i)))
		cache.put(k, mp3(400))
		os.Chtimes(cache.path(k), now.Add(time.Duration(i)*time.Minute), now.Add(time.Duration(i)*time.Minute))
	}
	cache.max = 1000 // 2000 bytes stored: prune down to 80% of the cap, oldest first
	cache.mu.Lock()
	cache.pruneLocked()
	cache.mu.Unlock()
	if _, ok := cache.get(cacheKey("fp", "en", "a")); ok {
		t.Fatal("oldest entry survived pruning")
	}
	if _, ok := cache.get(cacheKey("fp", "en", "e")); !ok {
		t.Fatal("newest entry was pruned")
	}
	// a damaged entry is ignored, not served
	k := cacheKey("fp", "en", "bad")
	os.WriteFile(cache.path(k), []byte("not an mp3"), 0o600)
	if _, ok := cache.get(k); ok {
		t.Fatal("corrupt entry served")
	}
	if st, _ := os.Stat(cache.path(cacheKey("fp", "en", "e"))); st.Mode().Perm() != 0o600 {
		t.Fatalf("cache file mode %v", st.Mode())
	}
}

func TestPrewarmFillsCache(t *testing.T) {
	c := cachedRig(t, "")
	c.s.prewarm(context.Background())
	want := 0
	for _, m := range c.s.prompts {
		want += len(m)
	}
	if len(c.tts.texts) != want {
		t.Fatalf("synthesised %d lines, want %d", len(c.tts.texts), want)
	}
	// second run finds everything cached
	c.s.prewarm(context.Background())
	if len(c.tts.texts) != want {
		t.Fatal("prewarm re-synthesised cached lines")
	}
	// a refusal line is now instant: no further synthesis
	before := len(c.tts.texts)
	c.post("/emo/voice/voiceprompt?languagecode=en&type=not_execute3&index=1", strings.NewReader(""), "")
	get(c.h, "/emo/chat/start?lang=en")
	if len(c.tts.texts) != before {
		t.Fatal("warmed prompt was synthesised again")
	}
	// no TTS or no cache: prewarm is a no-op, not a crash
	c.s.be.TTS = nil
	c.s.prewarm(context.Background())
}

func TestNoticesAndPermission(t *testing.T) {
	c := newConv(t, "")
	m := decode(t, get(c.h, "/emo/notice/latest"))
	if m["code"].(float64) != 200 || len(m["notice"].(map[string]any)) != 0 {
		t.Fatalf("%v", m)
	}
	f := filepath.Join(t.TempDir(), "n.json")
	var sb strings.Builder
	sb.WriteString(`[{"id":3,"message":"it's good to see you"},{"id":4,"message":""}`)
	for i := 5; i < 12; i++ {
		sb.WriteString(`,{"id":` + string(rune('0'+i%10)) + `,"message":"note ` + string(rune('a'+i)) + `"}`)
	}
	sb.WriteString(`]`)
	os.WriteFile(f, []byte(sb.String()), 0o644)
	c.s.cfg.NoticesFile = f
	n := decode(t, get(c.h, "/emo/notice/latest"))["notice"].(map[string]any)
	if len(n) != 5 { // at most five, empty messages skipped
		t.Fatalf("%d notices", len(n))
	}
	first := n["0"].(map[string]any)
	if first["id"].(float64) != 3 || first["message"] != "it's good to see you" {
		t.Fatalf("%v", first)
	}
	// the file is re-read: edits show up without a restart
	os.WriteFile(f, []byte(`[{"id":9,"message":"`+strings.Repeat("é", 300)+`"}]`), 0o644)
	n = decode(t, get(c.h, "/emo/notice/latest"))["notice"].(map[string]any)
	msg := n["0"].(map[string]any)["message"].(string)
	if len(msg) > 255 || !strings.HasPrefix(msg, "é") {
		t.Fatalf("message not safely truncated: %d bytes", len(msg))
	}
	os.WriteFile(f, []byte(`garbage`), 0o644)
	if m := decode(t, get(c.h, "/emo/notice/latest")); len(m["notice"].(map[string]any)) != 0 {
		t.Fatal("bad notices file should yield none")
	}

	if p := decode(t, get(c.h, "/emo/permission")); p["type"] != "normal" || p["screen_info"] != "" {
		t.Fatalf("%v", p)
	}
	c.s.cfg.Permission = PermissionConfig{Type: "tagged", ScreenInfo: "Owned by the people in this house"}
	if p := decode(t, get(c.h, "/emo/permission")); p["type"] != "tagged" || p["screen_info"] != "Owned by the people in this house" {
		t.Fatalf("%v", p)
	}
}

func TestWeatherIntentUsesLocaleCity(t *testing.T) {
	c := newConv(t, "what's the weather")
	w := &recWeather{f: backend.Forecast{Main: "sunny", TempC: 26.957}}
	c.s.be.Weather = w
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=2&locale=Bonn", voiceBody(40), ""))
	p := r["behavior_paras"].(map[string]any)
	if r["rec_behavior"] != "utilities" || p["utility_type"] != "weather" || w.city != "Bonn" {
		t.Fatalf("%v city=%q", r, w.city)
	}
	wm := p["weather"].(map[string]any)
	if wm["main"] != "sunny" || wm["temp"].(float64) != 26.96 { // two decimals, like the documented shape
		t.Fatalf("%v", wm)
	}
	// no weather source -> a spoken apology (cached fixed line), not silence
	c.s.be.Weather = nil
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=3&locale=Bonn", voiceBody(40), ""))
	if r["rec_behavior"] != "speak" || !strings.HasPrefix(r["behavior_paras"].(map[string]any)["url"].(string), audioBase) {
		t.Fatalf("%v", r)
	}
	if c.tts.texts[len(c.tts.texts)-1] != "I can't check the weather right now." {
		t.Fatalf("%v", c.tts.texts)
	}
	// weather source failing behaves the same; and with no TTS either, it degrades to "didn't understand"
	c.s.be.Weather = &recWeather{err: context.DeadlineExceeded}
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=4&locale=Bonn", voiceBody(40), ""))
	if r["rec_behavior"] != "speak" {
		t.Fatalf("%v", r)
	}
	c.s.be.TTS = nil
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=5&locale=Bonn", voiceBody(40), ""))
	if r["rec_behavior"] != "play_animation" {
		t.Fatalf("%v", r)
	}
}

func TestUtilityIntentsOverHTTP(t *testing.T) {
	c := newConv(t, "set a timer for two minutes")
	top, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en-c&index=8", voiceBody(40), ""))
	p := r["behavior_paras"].(map[string]any)
	tm := p["timer"].(map[string]any)
	if r["rec_behavior"] != "utilities" || tm["duration"].(float64) != 120 || top["index"].(float64) != 8 || len(c.llm.calls) != 0 {
		t.Fatalf("%v", r)
	}
	c.s.be.STT = backend.MockSTT{Text: "remind me to eat at 8 p.m. every Friday"}
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=9", voiceBody(40), ""))
	al := r["behavior_paras"].(map[string]any)["alarm"].(map[string]any)
	if al["time"] != "20:00" || al["recurrence"] != "FRI" || al["tag"] != "eat" {
		t.Fatalf("%v", al)
	}
	// a smalltalk answer has an empty behaviour and no paras, as the robot's own handler expects
	c.s.be.STT = backend.MockSTT{Text: "yes"}
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=10", voiceBody(40), ""))
	if r["rec_behavior"] != "" || r["intent"].(map[string]any)["name"] != "smalltalk_yes" || len(r["behavior_paras"].(map[string]any)) != 0 {
		t.Fatalf("%v", r)
	}
}

func TestExampleConfigHasNewFields(t *testing.T) {
	cfg, err := LoadConfig("config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTSCacheMB == 0 || cfg.Permission.Type == "" {
		t.Fatalf("example config should show the new options: %+v", cfg)
	}
}
