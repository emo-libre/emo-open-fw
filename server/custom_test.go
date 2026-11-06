package main

import (
	"context"
	"encoding/binary"
	"math"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCustom(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCustomCommandFileValidation(t *testing.T) {
	bad := map[string]string{
		`{"commands":[{"name":"Bad Name","phrases":["a b"],"say":"x"}]}`:                                               "name",
		`{"commands":[{"name":"a","phrases":[],"say":"x"}]}`:                                                           "phrases",
		`{"commands":[{"name":"a","phrases":["hello there"]}]}`:                                                        "does nothing",
		`{"commands":[{"name":"a","phrases":["x"],"say":"x"}]}`:                                                        "one-letter",
		`{"commands":[{"name":"a","phrases":["hello there"],"say":"x"},{"name":"a","phrases":["bye bye"],"say":"y"}]}`: "twice",
		`{"commands":[{"name":"a","phrases":["hello there"],"intent":"i"}]}`:                                           "go together",
		`{"commands":[{"name":"a","phrases":["hello there"],"exec":["/bin/true"]}]}`:                                   "allow_exec",
		`{"commands":[{"name":"a","phrases":["hello there"],"say":"x","oops":1}]}`:                                     "unknown field",
	}
	for body, want := range bad {
		if _, err := parseCustom([]byte(body), false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s -> %v, want an error mentioning %q", body, err, want)
		}
	}
	if _, err := parseCustom([]byte(`{"commands":[{"name":"a","phrases":["hello there"],"exec":["true"]}]}`), true); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("relative program accepted: %v", err)
	}
	if _, err := parseCustom([]byte(`{"commands":[{"name":"a","phrases":["hello there"],"exec":["/bin/true"],"timeout_s":99}]}`), true); err == nil {
		t.Error("99 s timeout accepted")
	}
	if cmds, err := parseCustom([]byte(`{"commands":[{"name":"ok-1","phrases":["Hello There!"],"say":"hi"}]}`), false); err != nil || len(cmds) != 1 {
		t.Errorf("good file rejected: %v", err)
	}
}

func TestCustomCommandMatching(t *testing.T) {
	p := writeCustom(t, `{"commands":[
	  {"name":"party","phrases":["party mode","let's party"],"say":"Party time!"},
	  {"name":"party-long","phrases":["party mode on the roof"],"say":"Roof party!"},
	  {"name":"hallo","lang":"de","phrases":["Guten Abend zusammen"],"say":"Hallo!"},
	  {"name":"exact-only","exact":true,"phrases":["open sesame"],"say":"Opened"},
	  {"name":"anywhere","contains":true,"phrases":["disco ball"],"say":"Ball"},
	  {"name":"cjk","phrases":["开派对"],"say":"好的"},
	  {"name":"accent","phrases":["café time"],"say":"Coffee"}]}`)
	cs, err := loadCustom(p, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ say, lang, want string }{
		{"Party mode!", "en", "party"}, {"please let's party now", "en", "party"}, {"party mode on the roof", "en", "party-long"},
		{"Guten Abend zusammen", "de", "hallo"}, {"Guten Abend zusammen", "en", ""}, {"open sesame", "en", "exact-only"},
		{"please open sesame for me now", "en", ""}, {"I would really like to see you spin the disco ball tonight okay", "en", "anywhere"},
		{"请开派对", "zh", "cjk"}, {"CAFE time", "en", "accent"}, {"nothing here", "en", ""},
		{"I went to a party mode of life that lasted all of last summer long", "en", ""}, // a sentence that merely contains the phrase stays conversation
	} {
		got := ""
		if m := cs.match(c.say, c.lang); m != nil {
			got = m.Name
		}
		if got != c.want {
			t.Errorf("%q (%s) -> %q, want %q", c.say, c.lang, got, c.want)
		}
	}
}

func TestCustomCommandsOverrideAndReload(t *testing.T) {
	c := newConv(t, "what time is it")
	p := writeCustom(t, `{"commands":[{"name":"clock","phrases":["what time is it"],"say":"Time to build robots."}]}`)
	var err error
	if c.s.custom, err = loadCustom(p, false); err != nil {
		t.Fatal(err)
	}
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] != "speak" || len(c.tts.texts) != 1 || c.tts.texts[0] != "Time to build robots." {
		t.Fatalf("custom command did not override the built-in: %v / %v", r, c.tts.texts)
	}
	if r["intent"] != nil {
		_ = r
	}
	// edit the file: picked up without a restart; a broken edit keeps the old commands
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(p, []byte(`{"commands":[{"name":"clock","phrases":["what time is it"],"say":"Second version."}]}`), 0o644)
	os.Chtimes(p, time.Now().Add(time.Second), time.Now().Add(time.Second))
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), "")
	if got := c.tts.texts[len(c.tts.texts)-1]; got != "Second version." {
		t.Fatalf("reload: %q", got)
	}
	os.WriteFile(p, []byte(`{not json`), 0o644)
	os.Chtimes(p, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second))
	c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), "")
	if got := c.tts.texts[len(c.tts.texts)-1]; got != "Second version." {
		t.Fatalf("a broken edit replaced the commands: %q", got)
	}
}

func TestCustomCommandBehaviours(t *testing.T) {
	c := newConv(t, "do the wave")
	p := writeCustom(t, `{"commands":[
	  {"name":"wave","phrases":["do the wave"],"animation":"Hi"},
	  {"name":"raw","phrases":["go dancing"],"intent":"my_dance","behavior":"dance","paras":{"dance_type":"to_music"}}]}`)
	c.s.custom, _ = loadCustom(p, false)
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] != "play_animation" || r["behavior_paras"].(map[string]any)["animation_name"] != "Hi" {
		t.Fatalf("animation: %v", r)
	}
	c = newConv(t, "go dancing")
	c.s.custom, _ = loadCustom(p, false)
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] != "dance" || r["behavior_paras"].(map[string]any)["dance_type"] != "to_music" {
		t.Fatalf("explicit command: %v", r)
	}
}

func TestCustomCommandRunsAProgramSafely(t *testing.T) {
	echo, err := exec.LookPath("echo")
	if err != nil {
		t.Skip("no echo")
	}
	sh, _ := exec.LookPath("sleep")
	c := newConv(t, "say it back")
	p := writeCustom(t, `{"commands":[
	  {"name":"echo","phrases":["say it back"],"exec":["`+echo+`","you said: {text}"],"speak_output":true},
	  {"name":"slow","phrases":["be slow"],"exec":["`+sh+`","20"],"timeout_s":1,"speak_output":true}]}`)
	var lerr error
	if c.s.custom, lerr = loadCustom(p, true); lerr != nil {
		t.Fatal(lerr)
	}
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if r["rec_behavior"] != "speak" || c.tts.texts[0] != "you said: say it back" {
		t.Fatalf("%v %v", r, c.tts.texts)
	}
	// a program's output is never cached
	if c.s.ttsCache != nil {
		for range listCache(c.s.ttsCache.dir) {
			t.Fatal("program output was cached")
		}
	}
	// a program that overruns its timeout is stopped and the robot gets a polite refusal
	c = newConv(t, "be slow")
	c.s.custom, _ = loadCustom(p, true)
	start := time.Now()
	_, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if time.Since(start) > 5*time.Second || r["rec_behavior"] != "speak" || c.tts.texts[0] != "I can't do that right now." {
		t.Fatalf("timeout handling: %v %v after %v", r, c.tts.texts, time.Since(start))
	}
	// without the allow switch the file is refused outright
	if _, err := loadCustom(p, false); err == nil {
		t.Fatal("a program command loaded without custom_commands_allow_exec")
	}
	// the command line has no shell: metacharacters stay plain text
	out, err := (&customCommand{Exec: []string{echo, "{text}"}, TimeoutS: 2}).runExec(context.Background(), "a; touch /tmp/x_should_not_exist $(id)", "en")
	if err != nil || out != "a; touch /tmp/x_should_not_exist $(id)" {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := os.Stat("/tmp/x_should_not_exist"); err == nil {
		t.Fatal("a shell ran")
	}
}

func TestAdminUIAndPhraseTester(t *testing.T) {
	c := newConv(t, "")
	if rr := get(c.h, "/admin/ui"); rr.Code != 404 {
		t.Fatalf("admin page reachable without being enabled: %d", rr.Code)
	}
	t.Setenv("TEST_ADMIN_TOKEN", "letmein-123")
	c.s.cfg.AdminTokenEnv = "TEST_ADMIN_TOKEN"
	c.s.adminTok = adminToken(c.s.cfg.AdminTokenEnv)
	p := writeCustom(t, `{"commands":[{"name":"party","phrases":["party mode"],"say":"Party time!"}]}`)
	c.s.custom, _ = loadCustom(p, false)
	c.h = c.s.routes()

	rr := get(c.h, "/admin/ui")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "EMO open server") || !strings.Contains(rr.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		strings.Contains(rr.Body.String(), "letmein") {
		t.Fatalf("admin page: %d %v", rr.Code, rr.Header())
	}
	if rr := adminReq(c.h, "/admin/parse?text=hello&lang=en", "wrong"); rr.Code != 401 {
		t.Fatalf("parse API without a token: %d", rr.Code)
	}
	for _, tc := range []struct{ url, key, want string }{
		{"/admin/parse?lang=en&text=" + "set+a+timer+for+ten+minutes", "intent", "timer_set"},
		{"/admin/parse?lang=zh&text=" + "%E5%81%9C%E6%AD%A2%E6%8E%A2%E7%B4%A2", "intent", "be_quiet"},
		{"/admin/parse?lang=de&text=" + "Wie+ist+das+Wetter+in+Berlin", "city", "Berlin"},
		{"/admin/parse?lang=en&text=party+mode", "custom", "party"},
	} {
		m := decode(t, adminReq(c.h, tc.url, "letmein-123"))
		if m[tc.key] != tc.want {
			t.Errorf("%s -> %v", tc.url, m)
		}
	}
	if m := decode(t, adminReq(c.h, "/admin/parse?lang=en&text=blah+blah+nothing+special+here", "letmein-123")); m["understood"] != false {
		t.Errorf("%v", m)
	}
	if rr := adminReq(c.h, "/admin/parse?lang=en", "letmein-123"); rr.Code != 400 {
		t.Errorf("empty text: %d", rr.Code)
	}
	langs := decode(t, adminReq(c.h, "/admin/languages", "letmein-123"))["languages"].([]any)
	if len(langs) < 11 {
		t.Errorf("languages: %v", langs)
	}
	cmds := decode(t, adminReq(c.h, "/admin/commands", "letmein-123"))["commands"].([]any)
	if len(cmds) != 1 {
		t.Errorf("commands: %v", cmds)
	}
}

func pcmOf(samples []float64) []byte {
	b := make([]byte, 2*len(samples))
	for i, v := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(int16(v)))
	}
	return b
}

func peakAndMean(b []byte) (peak, mean float64) {
	n := len(b) / 2
	for i := 0; i < n; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(b[2*i:])))
		mean += v
		peak = math.Max(peak, math.Abs(v))
	}
	return peak, mean / float64(n)
}

func TestCleanPCM(t *testing.T) {
	// a quiet 440 Hz tone riding on a DC offset
	var in []float64
	for i := 0; i < 16000; i++ {
		in = append(in, 1500+800*math.Sin(2*math.Pi*440*float64(i)/16000))
	}
	out := cleanPCM(pcmOf(in))
	if len(out) != 2*len(in) {
		t.Fatal("length changed")
	}
	pk, mean := peakAndMean(out)
	if math.Abs(mean) > 200 {
		t.Errorf("DC offset left: mean %v", mean)
	}
	if pk < 5000 || pk > 7000 { // an 800-peak tone is lifted by the 8x limit
		t.Errorf("level %v, want about 8 x 800", pk)
	}
	var mid []float64 // a 4000-peak tone is lifted to about 70% of full scale
	for i := 0; i < 16000; i++ {
		mid = append(mid, 4000*math.Sin(2*math.Pi*440*float64(i)/16000))
	}
	if pk, _ := peakAndMean(cleanPCM(pcmOf(mid))); pk < 0.6*32767 || pk > 0.75*32767 {
		t.Errorf("mid level %v not lifted to about 70%%", pk)
	}
	// silence stays silent (no noise amplified)
	quiet := make([]float64, 8000)
	for i := range quiet {
		quiet[i] = float64((i*7)%11 - 5)
	}
	if pk, _ := peakAndMean(cleanPCM(pcmOf(quiet))); pk > 30 {
		t.Errorf("noise floor amplified to %v", pk)
	}
	// a loud recording is not made louder, and nothing wraps around
	var loud []float64
	for i := 0; i < 8000; i++ {
		loud = append(loud, 30000*math.Sin(2*math.Pi*300*float64(i)/16000))
	}
	if pk, _ := peakAndMean(cleanPCM(pcmOf(loud))); pk > 32767 || pk < 20000 {
		t.Errorf("loud peak %v", pk)
	}
	if len(cleanPCM(nil)) != 0 {
		t.Error("nil")
	}
	_ = httptest.NewRecorder
}

func TestPreprocessSwitchKeepsTheRobotWorking(t *testing.T) {
	c := newConv(t, "hello")
	c.s.cfg.STTPreprocess = true
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=5", voiceBody(400), ""))
	if r["rec_behavior"] != "play_animation" {
		t.Fatalf("%v", r)
	}
}
