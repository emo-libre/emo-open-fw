package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"emoserver/backend"
)

type failSTT struct{}

func (failSTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	return "", errors.New("connection refused")
}

func adminRig(t *testing.T) *convRig {
	c := newConv(t, "")
	t.Setenv("TEST_ADMIN_TOKEN", "letmein-123")
	c.s.cfg.AdminTokenEnv = "TEST_ADMIN_TOKEN"
	c.s.adminTok = adminToken("TEST_ADMIN_TOKEN")
	c.h = c.s.routes()
	return c
}

func TestActivityLogIsOptInBoundedAndPrivate(t *testing.T) {
	c := adminRig(t)
	c.s.be.STT = backend.MockSTT{Text: "what time is it"}
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), "")
	if got := decode(t, adminReq(c.h, "/admin/activity", "letmein-123")); got["enabled"] != false || len(got["entries"].([]any)) != 0 {
		t.Fatalf("recorded while off: %v", got)
	}
	c.s.act.on = true
	tok := c.token(t, "aabbccddeeff")
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), tok)
	c.s.be.STT = backend.MockSTT{Text: strings.Repeat("blah ", 100)}
	c.post("/emo/voice/detectintent?languagecode=en&index=2", voiceBody(50), tok)
	c.s.be.STT = failSTT{}
	c.post("/emo/voice/detectintent?languagecode=en&index=3", voiceBody(50), tok)
	e := decode(t, adminReq(c.h, "/admin/activity", "letmein-123"))["entries"].([]any)
	if len(e) != 3 {
		t.Fatalf("%v", e)
	}
	newest, oldest := e[0].(map[string]any), e[2].(map[string]any)
	if newest["result"] != "speech-to-text failed or heard nothing" || oldest["heard"] != "what time is it" || oldest["result"] != "command: time" || oldest["robot"] != "eeff" {
		t.Fatalf("%v", e)
	}
	if len([]rune(e[1].(map[string]any)["heard"].(string))) > 200 {
		t.Fatal("heard text not cut")
	}
	for i := 0; i < 130; i++ {
		c.s.act.add("x", "en", "hi", "r", 0, nil)
	}
	if n := len(c.s.act.list()); n != activityMax {
		t.Fatalf("kept %d", n)
	}
	if rr := httptest.NewRecorder(); true {
		req := httptest.NewRequest("DELETE", "/admin/activity", nil)
		req.Header.Set("Authorization", "Bearer letmein-123")
		c.h.ServeHTTP(rr, req)
		if rr.Code != 200 || len(c.s.act.list()) != 0 {
			t.Fatalf("clear: %d", rr.Code)
		}
		req = httptest.NewRequest("DELETE", "/admin/activity", nil)
		rr = httptest.NewRecorder()
		c.h.ServeHTTP(rr, req)
		if rr.Code != 401 {
			t.Fatalf("clear without token: %d", rr.Code)
		}
	}
}

func TestSelfTestSayAndSetup(t *testing.T) {
	c := adminRig(t)
	c.s.be = backend.Set{STT: failSTT{}, LLM: c.llm, TTS: c.tts, News: nil}
	m := decode(t, adminReq(c.h, "/admin/selftest", "letmein-123"))
	res := m["results"].(map[string]any)
	if res["speech_to_text"].(map[string]any)["ok"] != false || res["speech_to_text"].(map[string]any)["error"] == "" ||
		res["language_model"].(map[string]any)["ok"] != true || res["text_to_speech"].(map[string]any)["ok"] != true || len(res) != 3 {
		t.Fatalf("%v", m)
	}
	if rr := adminReq(c.h, "/admin/selftest", "nope"); rr.Code != 401 {
		t.Fatalf("selftest without token: %d", rr.Code)
	}
	// speak
	rr := adminReq(c.h, "/admin/say?lang=en&text=hello", "letmein-123")
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "audio/mpeg" || rr.Header().Get("Cache-Control") != "no-store" || rr.Body.Len() == 0 {
		t.Fatalf("say: %d %v", rr.Code, rr.Header())
	}
	if rr := adminReq(c.h, "/admin/say?text="+strings.Repeat("a", 300), "letmein-123"); rr.Code != 400 {
		t.Fatalf("long text: %d", rr.Code)
	}
	c.s.be.TTS = nil
	if rr := adminReq(c.h, "/admin/say?text=hello", "letmein-123"); rr.Code != 503 {
		t.Fatalf("no tts: %d", rr.Code)
	}
	// setup checklist
	checks := decode(t, adminReq(c.h, "/admin/setup", "letmein-123"))["checks"].([]any)
	if len(checks) < 6 {
		t.Fatalf("%v", checks)
	}
}

func TestAdminOnItsOwnListener(t *testing.T) {
	c := adminRig(t)
	c.s.cfg.ListenAdmin = "127.0.0.1:0"
	c.h = c.s.routes()
	// the robot-facing server no longer carries the admin pages ...
	if rr := get(c.h, "/admin/ui"); rr.Code != 404 {
		t.Fatalf("admin page on the robot listener: %d", rr.Code)
	}
	if rr := adminReq(c.h, "/admin/status", "letmein-123"); rr.Code != 404 {
		t.Fatalf("admin API on the robot listener: %d", rr.Code)
	}
	// ... the admin listener has them, and nothing a robot uses
	a := c.s.adminRoutes()
	if rr := get(a, "/admin/ui"); rr.Code != 200 {
		t.Fatalf("admin listener: %d", rr.Code)
	}
	if rr := get(a, "/"); rr.Code != 302 || rr.Header().Get("Location") != "/admin/ui" {
		t.Fatalf("root: %d %v", rr.Code, rr.Header())
	}
	for _, p := range []string{"/time", "/token/aabbccddeeff", "/emo/ota/version", "/emo/voice/detectintent"} {
		if rr := get(a, p); rr.Code != 404 {
			t.Errorf("%s reachable on the admin listener: %d", p, rr.Code)
		}
	}
	if rr := adminReq(a, "/admin/status", "letmein-123"); rr.Code != 200 {
		t.Fatalf("status: %d", rr.Code)
	}
	cfg := defaultConfig()
	cfg.ListenAdmin = "127.0.0.1:8090"
	if cfg.validate() == nil {
		t.Fatal("listen_admin without a token variable accepted")
	}
}

func TestWrongTokensAreThrottledPerAddress(t *testing.T) {
	c := adminRig(t)
	try := func(addr, token string) int {
		req := httptest.NewRequest("GET", "/admin/status", nil)
		req.RemoteAddr = addr
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		c.h.ServeHTTP(rr, req)
		return rr.Code
	}
	for i := 0; i < throttleMax; i++ {
		if code := try("192.0.2.9:5000", "wrong"); code != 401 {
			t.Fatalf("guess %d: %d", i, code)
		}
	}
	if code := try("192.0.2.9:5001", "wrong"); code != 429 {
		t.Fatalf("not throttled: %d", code)
	}
	if code := try("192.0.2.9:5002", "letmein-123"); code != 429 {
		t.Fatalf("even the right token waits out the minute for a throttled address: %d", code)
	}
	// someone else is unaffected, and a page load with no token at all is not a guess
	if code := try("192.0.2.77:1", "letmein-123"); code != 200 {
		t.Fatalf("another address: %d", code)
	}
	for i := 0; i < 20; i++ {
		if code := try("192.0.2.50:1", ""); code != 401 {
			t.Fatalf("no header: %d", code)
		}
	}
	if code := try("192.0.2.50:1", "letmein-123"); code != 200 {
		t.Fatalf("missing headers counted as guesses: %d", code)
	}
	// the window slides
	var th authThrottle
	now := time.Now()
	for i := 0; i < throttleMax; i++ {
		th.failed("x", now)
	}
	if !th.blocked("x", now.Add(10*time.Second)) || th.blocked("x", now.Add(throttleWindow+time.Second)) {
		t.Fatal("window")
	}
}

func TestAdminAddressesAreListedForOtherDevices(t *testing.T) {
	c := adminRig(t)
	c.s.cfg.ListenAdmin, c.s.cfg.ListenAdminTLS = ":8090", ":8444"
	urls := c.s.adminURLs()
	var plain, secure bool
	for _, u := range urls {
		plain = plain || strings.HasPrefix(u, "http://127.0.0.1:8090/admin/ui")
		secure = secure || strings.HasPrefix(u, "https://127.0.0.1:8444/admin/ui")
	}
	if !plain || !secure {
		t.Fatalf("%v", urls)
	}
	c.s.cfg.ListenAdmin, c.s.cfg.ListenAdminTLS = "127.0.0.1:8090", ""
	for _, u := range c.s.adminURLs() {
		if !strings.HasPrefix(u, "http://127.0.0.1:") {
			t.Fatalf("a loopback-only page is advertised on %s", u)
		}
	}
	c.s.cfg.ListenAdmin = ""
	cfg := defaultConfig()
	cfg.ListenAdminTLS = ":8444"
	if cfg.validate() == nil {
		t.Fatal("an admin listener without a token accepted")
	}
}
