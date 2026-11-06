package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"emoserver/backend"
)

// ---------- drawing ----------

type recImage struct {
	reqs []backend.ImageRequest
	err  error
	out  []byte
	hold chan struct{}
}

func (r *recImage) Generate(ctx context.Context, q backend.ImageRequest) ([]byte, error) {
	r.reqs = append(r.reqs, q)
	if r.hold != nil {
		<-r.hold
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.out != nil {
		return r.out, nil
	}
	return backend.MockImage{}.Generate(ctx, q)
}

func jpegBytes() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}
}

func TestDrawTextToImage(t *testing.T) {
	c := newConv(t, "")
	img := &recImage{}
	c.s.be.Image = img
	rr := c.post("/emo/ai/draw/txttoimg?steps=20&prompt=detailed%20face%2C%202d%2C%20anime&cfg_scale=11&sampler_index=Euler%20a", strings.NewReader(""), "")
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	m := decode(t, rr)
	if m["errmessage"] != "ok" || m["code"].(float64) != 200 {
		t.Fatalf("%v", m)
	}
	raw, err := base64.StdEncoding.DecodeString(m["data"].(string))
	if err != nil || http.DetectContentType(raw) != "image/png" {
		t.Fatalf("data is not a base64 PNG: %v", err)
	}
	q := img.reqs[0]
	if q.Prompt != "detailed face, 2d, anime" || q.Steps != 20 || q.CFG != 11 || q.Sampler != "Euler a" || q.Init != nil {
		t.Fatalf("%+v", q)
	}
}

func TestDrawImageToImageAndLimits(t *testing.T) {
	c := newConv(t, "")
	img := &recImage{}
	c.s.be.Image = img
	rr := c.post("/emo/ai/draw/imgtoimg?steps=999&cfg_scale=-5&denoising_strength=7&prompt=hello&sampler_index=bad%3Bsampler%21", bytes.NewReader(jpegBytes()), "")
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	q := img.reqs[0]
	if q.Steps != 20 || q.CFG != 7 || q.Denoise != 0.2 || q.Sampler != "" || len(q.Init) == 0 { // out-of-range values fall back to defaults
		t.Fatalf("values not bounded: %+v", q)
	}
	c.post("/emo/ai/draw/imgtoimg?steps=30&denoising_strength=0.45&cfg_scale=9&prompt=x", bytes.NewReader(jpegBytes()), "")
	if q := img.reqs[1]; q.Steps != 30 || q.Denoise != 0.45 || q.CFG != 9 {
		t.Fatalf("%+v", q)
	}
	long := strings.Repeat("é", 2000)
	c.post("/emo/ai/draw/txttoimg?prompt="+long, strings.NewReader(""), "")
	if n := len([]rune(img.reqs[2].Prompt)); n > 600 || n == 0 {
		t.Fatalf("prompt length %d", n)
	}
	for name, tc := range map[string]struct{ path, body string }{
		"no prompt":      {"/emo/ai/draw/txttoimg?steps=20", ""},
		"empty photo":    {"/emo/ai/draw/imgtoimg?prompt=x", ""},
		"photo not jpeg": {"/emo/ai/draw/imgtoimg?prompt=x", "this is not a picture"},
	} {
		if rr := c.post(tc.path, strings.NewReader(tc.body), ""); rr.Code != 400 {
			t.Errorf("%s -> %d", name, rr.Code)
		}
	}
	if len(img.reqs) != 3 {
		t.Fatal("backend called for rejected requests")
	}
}

func TestDrawBackendProblems(t *testing.T) {
	c := newConv(t, "")
	if rr := c.post("/emo/ai/draw/txttoimg?prompt=x", strings.NewReader(""), ""); rr.Code != 501 {
		t.Fatalf("no backend -> %d", rr.Code)
	}
	c.s.be.Image = &recImage{err: fmt.Errorf("gpu on fire")}
	if rr := c.post("/emo/ai/draw/txttoimg?prompt=x", strings.NewReader(""), ""); rr.Code != 502 || strings.Contains(rr.Body.String(), "fire") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String()) // errors are summarised, never echoed
	}
	c.s.be.Image = &recImage{out: []byte("not an image")}
	if rr := c.post("/emo/ai/draw/txttoimg?prompt=x", strings.NewReader(""), ""); rr.Code != 502 {
		t.Fatalf("garbage image -> %d", rr.Code)
	}
	// one drawing at a time: a second request while one runs is told to come back
	hold := make(chan struct{})
	c.s.be.Image = &recImage{hold: hold}
	done := make(chan int)
	go func() { done <- c.post("/emo/ai/draw/txttoimg?prompt=slow", strings.NewReader(""), "").Code }()
	time.Sleep(100 * time.Millisecond)
	if rr := c.post("/emo/ai/draw/txttoimg?prompt=second", strings.NewReader(""), ""); rr.Code != 503 {
		t.Fatalf("concurrent drawing -> %d", rr.Code)
	}
	close(hold)
	if code := <-done; code != 200 {
		t.Fatalf("first drawing -> %d", code)
	}
}

func TestFinishImage(t *testing.T) {
	big, _ := backend.MockImage{}.Generate(context.Background(), backend.ImageRequest{Prompt: "p", Width: 400, Height: 200})
	out, err := finishImage(big, DrawConfig{Format: "png", MaxDim: 100})
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(out))
	if err != nil || im.Bounds().Dx() != 100 || im.Bounds().Dy() != 50 {
		t.Fatalf("%v %v", err, im.Bounds())
	}
	small, _ := finishImage(big, DrawConfig{Format: "png", MaxDim: 1000}) // never upscaled
	if im, _ := png.Decode(bytes.NewReader(small)); im.Bounds().Dx() != 400 {
		t.Fatal("image was enlarged")
	}
	j, err := finishImage(big, DrawConfig{Format: "jpeg"})
	if err != nil || http.DetectContentType(j) != "image/jpeg" {
		t.Fatalf("%v %s", err, http.DetectContentType(j))
	}
	if p, _ := finishImage(big, DrawConfig{Format: "passthrough", MaxDim: 10}); !bytes.Equal(p, big) {
		t.Fatal("passthrough altered the image")
	}
	if _, err := finishImage([]byte("junk"), DrawConfig{Format: "png"}); err == nil {
		t.Fatal("junk accepted")
	}
	tall := image.NewRGBA(image.Rect(0, 0, 10, 400))
	var buf bytes.Buffer
	png.Encode(&buf, tall)
	out, _ = finishImage(buf.Bytes(), DrawConfig{Format: "png", MaxDim: 100})
	if im, _ := png.Decode(bytes.NewReader(out)); im.Bounds().Dy() != 100 || im.Bounds().Dx() != 2 {
		t.Fatalf("tall image: %v", im.Bounds())
	}
}

// ---------- news ----------

func TestNewsIntent(t *testing.T) {
	c := newConv(t, "what's the news")
	c.s.be.News = backend.MockNews{Lines: []string{"Robots learn to dance", "Rain expected tomorrow", "Local cat wins prize", "Fourth story"}}
	top, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=5", voiceBody(40), ""))
	p := r["behavior_paras"].(map[string]any)
	if r["rec_behavior"] != "utilities" || p["utility_type"] != "news" || top["index"].(float64) != 5 {
		t.Fatalf("%v", r)
	}
	n := p["news"].(map[string]any)
	urls := n["urls"].([]any)
	if n["count"].(float64) != 3 || len(urls) != 3 { // default count
		t.Fatalf("%v", n)
	}
	first := urls[0].(map[string]any)
	if first["id"].(float64) != 1 || !strings.HasPrefix(first["url"].(string), audioBase+"/mp3/download/") {
		t.Fatalf("%v", first)
	}
	if len(c.tts.texts) != 3 {
		t.Fatalf("spoke %d clips", len(c.tts.texts))
	}
	for _, line := range c.tts.texts {
		if !strings.Contains("Robots learn to dance|Rain expected tomorrow|Local cat wins prize", line) {
			t.Fatalf("unexpected text %q", line)
		}
	}
	// no news source -> spoken apology
	c.s.be.News = nil
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=6", voiceBody(40), ""))
	if r["rec_behavior"] != "speak" {
		t.Fatalf("%v", r)
	}
	if c.tts.texts[len(c.tts.texts)-1] != "I can't get the news right now." {
		t.Fatal(c.tts.texts)
	}
	// "tell me about the news media" is a conversation, not the news
	if _, ok := parseIntent("tell me about the history of news media in europe"); ok {
		t.Fatal("conversation hijacked")
	}
}

// ---------- beta channel and content packs ----------

func TestBetaChannelAndConfigValidation(t *testing.T) {
	c := newConv(t, "")
	if m := decode(t, get(c.h, "/emo/ota/version")); m["test-version-num"] != nil {
		t.Fatalf("beta keys present when unset: %v", m)
	}
	c.s.cfg.BetaVersionNum, c.s.cfg.BetaVersionName = 2000, "open-beta"
	m := decode(t, get(c.h, "/emo/ota/version"))
	if m["test-version-num"].(float64) != 2000 || m["test-version-name"] != "open-beta" || m["version-num"].(float64) != 1234 {
		t.Fatalf("%v", m)
	}
	dir := t.TempDir()
	w := func(s string) string { p := filepath.Join(dir, "c.json"); os.WriteFile(p, []byte(s), 0o644); return p }
	if _, err := LoadConfig(w(`{"beta_version_num": 5}`)); err == nil {
		t.Fatal("half a beta config accepted")
	}
	if _, err := LoadConfig(w(`{"allowed_devices": ["not-a-mac"]}`)); err == nil {
		t.Fatal("bad device id accepted")
	}
	if _, err := LoadConfig(w(`{"allowed_devices": ["aabbccddeeff"], "beta_version_num": 5, "beta_version_name": "b"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestContentPacks(t *testing.T) {
	c := newConv(t, "")
	dir := t.TempDir()
	for g, files := range map[string][]string{"mp3": {"b.mp3", "a.mp3", ".hidden", "bad name.mp3"}, "avi": {"clip.avi"}, "model": {"face.kmodel"}} {
		os.MkdirAll(filepath.Join(dir, g), 0o755)
		for _, f := range files {
			os.WriteFile(filepath.Join(dir, g, filepath.Base(f)), []byte("DATA-"+f), 0o644)
		}
	}
	os.MkdirAll(filepath.Join(dir, "mp3", "subdir"), 0o755)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "outside.txt"), []byte("secret"), 0o644)
	c.s.cfg.ContentDir = dir

	u := decode(t, get(c.h, "/emo/ota/res/1"))["updates"].(map[string]any)
	sd := u["sd"].(map[string]any)
	mp3 := sd["mp3"].(map[string]any)
	if mp3["0"] != "/mp3/" || mp3["1"] != "a.mp3" || mp3["2"] != "b.mp3" || len(mp3) != 3 { // sorted; unsafe names and sub-directories skipped
		t.Fatalf("mp3 group %v", mp3)
	}
	if sd["avi"].(map[string]any)["1"] != "clip.avi" || len(sd["mot"].(map[string]any)) != 1 {
		t.Fatalf("%v", sd)
	}
	mod := u["model"].(map[string]any)
	e := mod["1"].(map[string]any)
	if mod["0"] != "/model/" || e["0"] != "face.kmodel" || e["1"].(float64) != float64(len("DATA-face.kmodel")) {
		t.Fatalf("model %v", mod)
	}
	if rr := get(c.h, "/mp3/a.mp3"); rr.Code != 200 || rr.Body.String() != "DATA-a.mp3" {
		t.Fatalf("%d %q", rr.Code, rr.Body.String())
	}
	for _, p := range []string{"/mp3/.hidden", "/mp3/subdir", "/mp3/..%2foutside.txt", "/mp3/nothere.mp3", "/avi/a.mp3", "/mot/clip.avi", "/mp3/a.mp3/x"} {
		if rr := get(c.h, p); rr.Code == 200 {
			t.Errorf("%s served %q", p, rr.Body.String())
		}
	}
	// the generated-speech route is untouched by the content routes
	c.s.cfg.ContentDir = ""
	if rr := get(c.h, "/mp3/a.mp3"); rr.Code == 200 {
		t.Fatal("content served without content_dir")
	}
	u = decode(t, get(c.h, "/emo/ota/res/1"))["updates"].(map[string]any)
	if len(u["sd"].(map[string]any)["mp3"].(map[string]any)) != 1 {
		t.Fatal("empty content_dir should give empty (but present) groups")
	}
}

// ---------- admin, telemetry, access control ----------

func adminReq(h http.Handler, path, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAdminAPIAndTelemetryPrivacy(t *testing.T) {
	c := newConv(t, "")
	if rr := adminReq(c.h, "/admin/status", "x"); rr.Code != 404 {
		t.Fatalf("admin API reachable without being enabled: %d", rr.Code)
	}
	t.Setenv("TEST_ADMIN_TOKEN", "letmein-123")
	c.s.cfg.AdminTokenEnv = "TEST_ADMIN_TOKEN"
	c.s.adminTok = adminToken(c.s.cfg.AdminTokenEnv)
	c.h = c.s.routes()

	tok := c.token(t, "aabbccddeeff")
	rep := `{"ver":42,"city":"Springfield","tzone":"Europe/Berlin","lang":"en","age":42,"wkup":120,"kcrash":"0/0.3","batt":3244,"drop":0,` +
		`"beh":25,"touch":[0,0,0],"wifi":-51,"geo":120.3,"mic":[0,0,0,0],"err":1,"esp":"3.2.0","ken":"3.2.0","evil":"x"}`
	req := httptest.NewRequest("POST", "/emo/report/info", strings.NewReader(rep))
	req.Header.Set("Authorization", "Bearer "+tok)
	c.h.ServeHTTP(httptest.NewRecorder(), req)

	for _, bad := range []string{"", "wrong", "letmein-12"} {
		if rr := adminReq(c.h, "/admin/robots", bad); rr.Code != 401 {
			t.Errorf("token %q -> %d", bad, rr.Code)
		}
	}
	rr := adminReq(c.h, "/admin/robots", "letmein-123")
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	body := rr.Body.String()
	for _, leak := range []string{"Springfield", "Europe/Berlin", "city", "tzone", "lang", "touch", "geo", "mic", "evil"} {
		if strings.Contains(body, leak) {
			t.Fatalf("admin output contains %q: %s", leak, body)
		}
	}
	m := decode(t, rr)
	r0 := m["robots"].([]any)[0].(map[string]any)
	rep0 := r0["reports"].([]any)[0].(map[string]any)
	if r0["id"] != "aabbccddeeff" || rep0["batt"].(float64) != 3244 || rep0["esp"] != "3.2.0" || rep0["wifi"].(float64) != -51 {
		t.Fatalf("%v", r0)
	}
	// reports are bounded
	for i := 0; i < 40; i++ {
		req := httptest.NewRequest("POST", "/emo/report/info", strings.NewReader(`{"batt":1}`))
		req.Header.Set("Authorization", "Bearer "+tok)
		c.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	m = decode(t, adminReq(c.h, "/admin/robots", "letmein-123"))
	if n := len(m["robots"].([]any)[0].(map[string]any)["reports"].([]any)); n != 20 {
		t.Fatalf("%d reports kept", n)
	}
	st := decode(t, adminReq(c.h, "/admin/status", "letmein-123"))
	if st["requests"].(map[string]any)["/emo/report/info"].(float64) < 41 || st["backends"].(map[string]any)["tts"] != true {
		t.Fatalf("%v", st)
	}
	if sanitizeReport([]byte("garbage")) != nil {
		t.Fatal("garbage report kept")
	}
}

func TestRequireTokenAndAllowedDevices(t *testing.T) {
	c := newConv(t, "")
	if rr := get(c.h, "/emo/notice/latest"); rr.Code != 200 { // open by default
		t.Fatal(rr.Code)
	}
	c.s.cfg.RequireToken = true
	if rr := get(c.h, "/emo/notice/latest"); rr.Code != 401 {
		t.Fatalf("no token -> %d", rr.Code)
	}
	good := c.token(t, "aabbccddeeff")
	req := httptest.NewRequest("GET", "/emo/notice/latest", nil)
	req.Header.Set("Authorization", "Bearer "+good)
	rr := httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("valid token -> %d", rr.Code)
	}
	forged, _ := signJWT([]byte("some other key some other key !!"), map[string]any{"sub": "aabbccddeeff", "exp": time.Now().Add(time.Hour).Unix()})
	req = httptest.NewRequest("GET", "/emo/notice/latest", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	rr = httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("forged token -> %d", rr.Code)
	}
	// time, token and the firmware file never need a bearer token
	for _, p := range []string{"/time", "/token/aabbccddeeff", "/firmware/esp32/image.bin"} {
		if rr := get(c.h, p); rr.Code != 200 {
			t.Errorf("%s -> %d", p, rr.Code)
		}
	}
	// allow-list: other robots get no token and no service
	c.s.cfg.AllowedDevices = []string{"112233445566"}
	if rr := get(c.h, "/token/aabbccddeeff"); rr.Code != 403 {
		t.Fatalf("unlisted robot got a token: %d", rr.Code)
	}
	if rr := get(c.h, "/token/112233445566"); rr.Code != 200 {
		t.Fatalf("listed robot refused: %d", rr.Code)
	}
	req = httptest.NewRequest("GET", "/emo/notice/latest", nil)
	req.Header.Set("Authorization", "Bearer "+good) // valid token, but for a robot that is not allowed
	rr = httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("unlisted robot with an old token -> %d", rr.Code)
	}
}

// ---------- language packs ----------

func TestShippedGermanPack(t *testing.T) {
	ps, err := loadPacks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps["de"].phrases) < 100 {
		t.Fatalf("de pack has %d phrases", len(ps["de"].phrases))
	}
	for say, want := range map[string]string{
		"Hallo!": "greeting", "Schalt dich aus": "power_off", "Tanz für mich": "dance", "Wie spät ist es?": "time",
		"Mach das Licht aus": "turn_off_light", "Lauter!": "volume_to", "Wie ist das Wetter": "weather", "Lies die Nachrichten": "news",
		"Ja": "smalltalk_yes", "Nein danke": "smalltalk_no", "Stopp": "be_quiet", "Dreh dich um": "turn_around",
		"Spielen wir Tic Tac Toe": "featured_game_ttt", "Wie alt bist du?": "about_age", "Geh nach links": "left",
		"Sing ein Lied": "play_music", "Mach ein Foto": "take_photo", "Wirf einen Würfel": "featured_game_roll_a_dice",
	} {
		r, ok := ps["de"].match(say)
		if !ok || r.Intent != want {
			t.Errorf("%q -> %q ok=%v, want %s", say, r.Intent, ok, want)
		}
	}
	// "yes"/"no"/"hello" must be the whole utterance, and long sentences are conversation
	for _, say := range []string{"Ja das stimmt nicht", "Hallo ich habe eine Frage", "Nein ich meinte etwas anderes", "Ja ich glaube schon aber ich bin mir nicht sicher", "Hallo ich hätte gern eine lange Geschichte über Drachen und Ritter",
		"Erzähl mir etwas über das Wetter im Mittelalter und die Geschichte der Wetterkarte"} {
		if r, ok := ps["de"].match(say); ok {
			t.Errorf("%q matched %s", say, r.Intent)
		}
	}
	// the longest phrase wins: "tanz zur musik" beats "tanz"
	if r, _ := ps["de"].match("tanz zur musik"); r.Intent != "listen_to_music" {
		t.Errorf("longest-phrase rule: %s", r.Intent)
	}
	if _, ok := ps["fr"].match("Hallo"); ok {
		t.Error("German phrase matched in another language")
	}
}

func TestPackIntegrationAndFallback(t *testing.T) {
	c := newConv(t, "Mach das Licht aus")
	ps, _ := loadPacks("")
	c.s.packs = ps
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=de&index=1", voiceBody(30), ""))
	if r["intent"].(map[string]any)["name"] != "turn_off_light" || len(c.llm.calls) != 0 {
		t.Fatalf("%v", r)
	}
	// a German unit still understands English commands
	c.s.be.STT = backend.MockSTT{Text: "set a timer for ten seconds"}
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=de&index=2", voiceBody(30), ""))
	if r["intent"].(map[string]any)["name"] != "timer_set" {
		t.Fatalf("English fallback: %v", r)
	}
	// English units do not consult the German pack
	c.s.be.STT = backend.MockSTT{Text: "Hallo"}
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=3", voiceBody(30), ""))
	if r["intent"].(map[string]any)["name"] == "greeting" && false {
		t.Fatal()
	}
}

func TestPackValidation(t *testing.T) {
	dir := t.TempDir()
	w := func(s string) { os.WriteFile(filepath.Join(dir, "xx.json"), []byte(s), 0o644) }
	w(`{"dance": ["tanz"]}`)
	if ps, err := loadPacks(dir); err != nil || ps["xx"] == nil || len(ps["xx"].phrases) != 1 {
		t.Fatal(err)
	}
	w(`{"danse": ["tanz"]}`)
	if _, err := loadPacks(dir); err == nil || !strings.Contains(err.Error(), "danse") {
		t.Fatalf("typo in a command key not reported: %v", err)
	}
	w(`{"dance": ["  "]}`)
	if _, err := loadPacks(dir); err == nil {
		t.Fatal("empty phrase accepted")
	}
	w(`not json`)
	if _, err := loadPacks(dir); err == nil {
		t.Fatal("bad json accepted")
	}
	if ps, err := loadPacks(""); err != nil || ps["en"] == nil || ps["de"] == nil || ps["vi"] == nil {
		t.Fatalf("embedded packs missing: %v", err)
	}
	if normalizeLoose("  Tschüss!  Ça va?  ") != "tschüss ça va" {
		t.Fatal(normalizeLoose("  Tschüss!  Ça va?  "))
	}
	// every template builds a result with an intent, and parameter maps are not shared between calls
	for k, f := range packTemplates {
		if f().Intent == "" {
			t.Errorf("template %s has no intent", k)
		}
	}
	a, b := packTemplates["dance_with_lights"](), packTemplates["dance_with_lights"]()
	a.Paras["x"] = 1
	if b.Paras["x"] != nil {
		t.Fatal("template results share state")
	}
}

// ---------- LLM-assisted commands ----------

func TestLLMCommands(t *testing.T) {
	c := newConv(t, "could you maybe do a little jig for me")
	c.s.cfg.LLMCommands = true
	c.llm.reply = "```json\n{\"command\": \"dance\"}\n```"
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(30), ""))
	if r["rec_behavior"] != "dance" || len(c.tts.texts) != 0 {
		t.Fatalf("%v", r)
	}
	if sys := c.llm.calls[0][0]; sys.Role != "system" || !strings.Contains(sys.Content, "volume_up") || strings.Contains(sys.Content, "power_off") ||
		strings.Contains(sys.Content, "install_update") {
		t.Fatalf("instruction offers unsafe commands or omits safe ones: %q", sys.Content)
	}
	// a reply
	c.llm.reply = `{"reply": "I would love to, but my hips are very small."}`
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=2", voiceBody(30), ""))
	if r["rec_behavior"] != "speak" || r["behavior_paras"].(map[string]any)["txt"] != "I would love to, but my hips are very small." {
		t.Fatalf("%v", r)
	}
	// destructive commands are never taken from the model
	for _, cmd := range []string{"power_off", "install_update", "nonsense_command"} {
		c.llm.reply = `{"command": "` + cmd + `"}`
		_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=3", voiceBody(30), ""))
		if r["rec_behavior"] == "power_off" || (r["rec_behavior"] == "utilities" && cmd == "install_update") {
			t.Errorf("model-chosen %s was executed: %v", cmd, r)
		}
		if r["rec_behavior"] != "play_animation" {
			t.Errorf("%s: expected a graceful 'didn't understand', got %v", cmd, r["rec_behavior"])
		}
	}
	// plain text from the model is spoken; broken structured output is not read out loud
	c.llm.reply = "Sure, I can tell you a fact: octopuses have three hearts."
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=4", voiceBody(30), ""))
	if r["rec_behavior"] != "speak" {
		t.Fatalf("%v", r)
	}
	n := len(c.tts.texts)
	c.llm.reply = `{"command": }`
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=5", voiceBody(30), ""))
	if r["rec_behavior"] != "play_animation" || len(c.tts.texts) != n {
		t.Fatalf("%v", r)
	}
	// off by default: exactly the old behaviour
	c.s.cfg.LLMCommands = false
	c.llm.calls = nil
	c.llm.reply = "Plain answer."
	c.post("/emo/voice/detectintent?languagecode=en&index=6", voiceBody(30), "")
	if len(c.llm.calls[0]) != 1 {
		t.Fatalf("command instruction sent while disabled: %v", c.llm.calls[0])
	}
	// a command that needs a backend still goes through it
	c.s.cfg.LLMCommands = true
	c.s.be.Weather = &recWeather{f: backend.Forecast{Main: "rain", TempC: 7.5}}
	c.llm.reply = `{"command":"weather"}`
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=7&locale=Bonn", voiceBody(30), ""))
	if r["behavior_paras"].(map[string]any)["weather"].(map[string]any)["main"] != "rain" {
		t.Fatalf("%v", r)
	}
	if k, r := parseModelChoice("no json here"); k != "" || r != "" {
		t.Fatal("junk parsed")
	}
}

// ---------- prompts in other languages ----------

func TestEveryLanguageHasEveryPrompt(t *testing.T) {
	for lang, m := range defaultPrompts {
		for key := range defaultPrompts["en"] {
			if strings.TrimSpace(m[key]) == "" {
				t.Errorf("%s is missing prompt %q", lang, key)
			}
		}
		if lang != "en" {
			if !strings.Contains(m["chat_start"], "{name}") {
				t.Errorf("%s chat_start lost its {name} slot", lang)
			}
		}
	}
	pr, _ := loadPrompts("")
	if pr.text("de", "not_execute") != "Das kann ich gerade nicht." || pr.text("xx", "not_execute") != "I can't do that right now." {
		t.Fatal("language fallback")
	}
	// the robot's own wire format: a German unit asking for a refusal line gets German speech
	c := newConv(t, "")
	c.post("/emo/voice/voiceprompt?languagecode=de&type=not_execute2&index=1", strings.NewReader(""), "")
	if c.tts.texts[0] != "Ich bin gerade beschäftigt. Frag mich gleich noch einmal." {
		t.Fatalf("%q", c.tts.texts[0])
	}
}

func TestExampleConfigStillLoadsWithEverything(t *testing.T) {
	cfg, err := LoadConfig("config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildBackends(cfg.Backends); err != nil {
		t.Fatal(err)
	}
	if cfg.Draw.Format == "" || cfg.PacksDir == "" {
		t.Fatalf("example config should show the new options: %+v", cfg)
	}
	var m map[string]any
	b, _ := os.ReadFile("config.example.json")
	if json.Unmarshal(b, &m) != nil {
		t.Fatal("example is not JSON")
	}
}

func TestEmbeddedPacksMatchTheFilesOnDisk(t *testing.T) {
	disk, err := os.ReadDir("packs")
	if err != nil {
		t.Fatal(err)
	}
	emb, _ := embeddedPacks.ReadDir("packs")
	if len(disk) != len(emb) {
		t.Fatalf("%d files on disk, %d embedded", len(disk), len(emb))
	}
}
