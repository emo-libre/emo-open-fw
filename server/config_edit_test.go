package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tok = "letmein-123"

func adminDo(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// editRig is an admin rig whose settings can be edited and saved into a real file.
func editRig(t *testing.T, fileBody string) (*convRig, string) {
	c := adminRig(t)
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(fileBody), 0o600); err != nil {
		t.Fatal(err)
	}
	c.s.cfg.AdminAllowEdit = true
	c.s.configPath = p
	c.h = c.s.routes()
	return c, p
}

const baseFile = `{"listen":":8443","audio_base_url":"http://192.0.2.10:8080","esp32_file":"image.bin","version_num":4321,"server_switch":"keep-me","admin_token_env":"TEST_ADMIN_TOKEN","admin_allow_edit":true}`

func TestEditingIsOffByDefaultAndNeedsAFile(t *testing.T) {
	c := adminRig(t)
	for _, r := range []struct{ m, p, b string }{{"PUT", "/admin/config", `{}`}, {"PUT", "/admin/secrets", `{"name":"ABC_KEY","value":"x"}`},
		{"PUT", "/admin/commands/file", `{"commands":[]}`}, {"POST", "/admin/restart", ``}, {"DELETE", "/admin/secrets?name=ABC_KEY", ``}} {
		if rr := adminDo(c.h, r.m, r.p, r.b); rr.Code != 403 || !strings.Contains(rr.Body.String(), "admin_allow_edit") {
			t.Errorf("%s %s with editing off: %d %s", r.m, r.p, rr.Code, rr.Body.String())
		}
	}
	// allowed but started without a config file
	c.s.cfg.AdminAllowEdit = true
	if rr := adminDo(c.h, "PUT", "/admin/config", `{}`); rr.Code != 403 || !strings.Contains(rr.Body.String(), "-c config.json") {
		t.Errorf("no file: %d %s", rr.Code, rr.Body.String())
	}
	// and nothing without the token
	req := httptest.NewRequest("PUT", "/admin/config", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("no token: %d", rr.Code)
	}
}

func TestSavingSettingsKeepsTheRestOfTheFileAndMakesABackup(t *testing.T) {
	c, p := editRig(t, baseFile)
	body := `{"public_host":"192.168.1.77:8443","audio_base_url":"http://192.168.1.77:8080","free_chat":false,"activity_log":true,
	  "dns_listen":":53","dns_names":["api.example.test"],"dns_answer_ip":"192.168.1.77","capture_on":true,
	  "backends":{"tts":{"type":"espeak","espeak":{"speed":150}},"stt":{"type":"whispercpp","whispercpp":{"model":"/m/ggml-base.en.bin"}},"weather":{"type":"open-meteo"}}}`
	rr := adminDo(c.h, "PUT", "/admin/config", body)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"restart_needed":true`) {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	saved, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("the saved file does not load: %v", err)
	}
	if saved.PublicHost != "192.168.1.77:8443" || saved.FreeChat || !saved.ActivityLog || saved.DNSListen != ":53" || saved.Backends.TTS.Type != "espeak" ||
		saved.Backends.TTS.Espeak.Speed != 150 || saved.Backends.STT.WhisperCpp.Model != "/m/ggml-base.en.bin" || saved.Backends.Weather.Type != "open-meteo" ||
		!strings.HasSuffix(saved.Capture.Dir, "capture") {
		t.Fatalf("%+v", saved)
	}
	if saved.VersionNum != 4321 || saved.ServerSwitch != "keep-me" || !saved.AdminAllowEdit || saved.AdminTokenEnv != "TEST_ADMIN_TOKEN" {
		t.Fatalf("settings the page does not manage were lost: %+v", saved)
	}
	if bak, err := os.ReadFile(p + ".bak"); err != nil || string(bak) != baseFile {
		t.Fatalf("backup: %v %q", err, bak)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	// the running server is unchanged until the restart
	if c.s.cfg.PublicHost == "192.168.1.77:8443" {
		t.Fatal("changed live")
	}
	// the page reads back what is saved? (running config) and the editable view has no secrets
	m := decode(t, adminDo(c.h, "GET", "/admin/config", ""))
	if m["can_edit"] != true || m["config_path"] != p {
		t.Fatalf("%v", m)
	}
}

func TestSavingRefusesWhatThePageMayNotSet(t *testing.T) {
	c, p := editRig(t, baseFile)
	before, _ := os.ReadFile(p)
	for name, body := range map[string]string{
		"command tts":     `{"backends":{"tts":{"type":"command","command":{"args":["/bin/echo"]}}}}`,
		"command stt":     `{"backends":{"stt":{"type":"command","command":{"args":["/bin/echo"]}}}}`,
		"hidden command":  `{"backends":{"llm":{"type":"openai","command":{"args":["/bin/sh","-c","id"]},"openai":{"base_url":"http://127.0.0.1:1/v1","model":"m"}}}}`,
		"command image":   `{"backends":{"image":{"type":"command","command":{"args":["/bin/echo"]}}}}`,
		"lower key name":  `{"backends":{"llm":{"type":"openai","openai":{"base_url":"http://127.0.0.1:1/v1","model":"m","api_key_env":"home"}}}}`,
		"admin token key": `{"backends":{"llm":{"type":"openai","openai":{"base_url":"http://127.0.0.1:1/v1","model":"m","api_key_env":"TEST_ADMIN_TOKEN"}}}}`,
		"vendor host":     `{"backends":{"llm":{"type":"openai","openai":{"base_url":"https://api.living.ai/v1","model":"m"}}}}`,
		"unknown type":    `{"backends":{"tts":{"type":"magic"}}}`,
		"listen address":  `{"listen":":1"}`,
		"exec switch":     `{"custom_commands_allow_exec":true}`,
		"admin token":     `{"admin_token_env":"OTHER"}`,
		"bad dns":         `{"dns_listen":":53","dns_names":[]}`,
		"bad piper":       `{"backends":{"tts":{"type":"piper","piper":{"model":"../../etc/passwd"}}}}`,
		"bad speed":       `{"backends":{"tts":{"type":"espeak","espeak":{"speed":99999}}}}`,
		"not json":        `{nope`,
	} {
		if rr := adminDo(c.h, "PUT", "/admin/config", body); rr.Code != 400 {
			t.Errorf("%s accepted: %d %s", name, rr.Code, rr.Body.String())
		}
	}
	if after, _ := os.ReadFile(p); !bytes.Equal(before, after) {
		t.Fatal("a refused change altered the file")
	}
}

func TestKeysAreWriteOnly(t *testing.T) {
	c, _ := editRig(t, baseFile)
	t.Setenv("ALREADY_SET_KEY", "from-the-environment")
	rr := adminDo(c.h, "PUT", "/admin/secrets", `{"name":"GROQ_API_KEY","value":"gsk_super_secret_value"}`)
	if rr.Code != 200 || strings.Contains(rr.Body.String(), "gsk_super_secret_value") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if os.Getenv("GROQ_API_KEY") != "gsk_super_secret_value" {
		t.Fatal("not exported to the backends")
	}
	t.Cleanup(func() { os.Unsetenv("GROQ_API_KEY") })
	for _, p := range []string{"/admin/secrets", "/admin/config", "/admin/status", "/admin/setup", "/admin/detect"} {
		if body := adminDo(c.h, "GET", p, "").Body.String(); strings.Contains(body, "gsk_super_secret_value") {
			t.Errorf("%s leaks the key", p)
		}
	}
	m := decode(t, adminDo(c.h, "GET", "/admin/secrets", ""))
	if names := m["names"].(map[string]any); names["GROQ_API_KEY"] != "saved here" {
		t.Fatalf("%v", m)
	}
	file := filepath.Join(c.s.cfg.DataDir, "secrets.json")
	if st, err := os.Stat(file); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file: %v %v", err, st)
	}
	for name, body := range map[string]string{"lower": `{"name":"groq","value":"x"}`, "empty": `{"name":"GROQ_API_KEY","value":""}`,
		"newline": `{"name":"GROQ_API_KEY","value":"a\nb"}`, "admin": `{"name":"TEST_ADMIN_TOKEN","value":"x"}`, "extra": `{"name":"GROQ_API_KEY","value":"x","y":1}`} {
		if rr := adminDo(c.h, "PUT", "/admin/secrets", body); rr.Code != 400 {
			t.Errorf("%s accepted: %d", name, rr.Code)
		}
	}
	// a restart reads the file back, but an environment variable that is already set wins
	os.Unsetenv("GROQ_API_KEY")
	os.WriteFile(file, []byte(`{"GROQ_API_KEY":"from-file","ALREADY_SET_KEY":"from-file"}`), 0o600)
	if _, err := loadSecrets(c.s.cfg.DataDir); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GROQ_API_KEY") != "from-file" || os.Getenv("ALREADY_SET_KEY") != "from-the-environment" {
		t.Fatalf("%q %q", os.Getenv("GROQ_API_KEY"), os.Getenv("ALREADY_SET_KEY"))
	}
	c.s.secrets, _ = loadSecrets(c.s.cfg.DataDir)
	if rr := adminDo(c.h, "DELETE", "/admin/secrets?name=GROQ_API_KEY", ""); rr.Code != 200 || os.Getenv("GROQ_API_KEY") != "" {
		t.Fatalf("delete: %d", rr.Code)
	}
}

func TestEditingYourOwnCommandsKeepsPrograms(t *testing.T) {
	c, p := editRig(t, baseFile)
	echo := "/bin/echo"
	if _, err := os.Stat(echo); err != nil {
		echo = "/usr/bin/echo"
	}
	file := filepath.Join(t.TempDir(), "custom.json")
	os.WriteFile(file, []byte(`{"commands":[{"name":"prog","phrases":["run the thing"],"exec":["`+echo+`","x"]}]}`), 0o600)
	c.s.cfg.CustomAllowExec = true
	c.s.cfg.CustomCommandsFile = file
	var err error
	if c.s.custom, err = loadCustom(file, true); err != nil {
		t.Fatal(err)
	}
	c.h = c.s.routes()
	rr := adminDo(c.h, "PUT", "/admin/commands/file", `{"commands":[{"name":"party","phrases":["party mode"],"say":"Party time!"},{"name":"wave","lang":"en","phrases":["do the wave"],"animation":"Hi","contains":true}]}`)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if cmds, err := parseCustom(readFile(t, file), true); err != nil || len(cmds) != 3 || cmds[0].Name != "prog" || len(cmds[0].Exec) != 2 {
		t.Fatalf("the program command was not kept: %v %v", cmds, err)
	}
	m := decode(t, adminDo(c.h, "GET", "/admin/commands/file", ""))
	if m["kept_in_file"].(float64) != 1 || len(m["commands"].([]any)) != 2 {
		t.Fatalf("%v", m)
	}
	// live: the new phrase works at once
	if r, ok := c.s.parse("party mode", "en"); !ok || r.Custom == nil || r.Custom.Say != "Party time!" {
		t.Fatalf("not live: %+v %v", r, ok)
	}
	for name, body := range map[string]string{"bad name": `{"commands":[{"name":"Bad Name","phrases":["a b"],"say":"x"}]}`,
		"nothing to do": `{"commands":[{"name":"x","phrases":["hello there"]}]}`, "exec smuggled": `{"commands":[{"name":"x","phrases":["hello there"],"exec":["/bin/true"]}]}`} {
		if rr := adminDo(c.h, "PUT", "/admin/commands/file", body); rr.Code != 400 {
			t.Errorf("%s accepted: %d %s", name, rr.Code, rr.Body.String())
		}
	}
	_ = p

	// first time ever: the file is created and remembered in the config
	c2, p2 := editRig(t, baseFile)
	if rr := adminDo(c2.h, "PUT", "/admin/commands/file", `{"commands":[{"name":"hello-you","phrases":["greet them"],"say":"Hello everyone"}]}`); rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	saved, err := LoadConfig(p2)
	if err != nil || !strings.HasSuffix(saved.CustomCommandsFile, "custom_commands.json") {
		t.Fatalf("%+v %v", saved.CustomCommandsFile, err)
	}
	if r, ok := c2.s.parse("greet them", "en"); !ok || r.Custom == nil {
		t.Fatal("not live")
	}
}

func readFile(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRestartIsAskedForNotAssumed(t *testing.T) {
	c, _ := editRig(t, baseFile)
	done := make(chan struct{}, 1)
	c.s.restartFn = func() { done <- struct{}{} }
	if rr := adminDo(c.h, "POST", "/admin/restart", ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), "restarting") {
		t.Fatalf("%d", rr.Code)
	}
	<-done
}

func TestFirstRunMakesAUsableConfigAndAToken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := createStarterConfig(p); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("the starter config does not load: %v", err)
	}
	if cfg.ListenAdmin != ":8090" || cfg.ListenAdminTLS != ":8444" || !cfg.AdminAutoToken || !cfg.AdminAllowEdit || len(cfg.Backends.STT.Type+cfg.Backends.TTS.Type) != 0 {
		t.Fatalf("%+v", cfg)
	}
	cfg.DataDir = filepath.Join(dir, "data")
	a, created, err := resolveAdminToken(cfg)
	if err != nil || !created || len(a) != 32 {
		t.Fatalf("%q %v %v", a, created, err)
	}
	if st, _ := os.Stat(filepath.Join(cfg.DataDir, "admin_token")); st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %v", st.Mode())
	}
	b, created, _ := resolveAdminToken(cfg)
	if created || string(a) != string(b) {
		t.Fatal("the token changed on the second start")
	}
	t.Setenv("MY_TOKEN_VAR", "from-the-environment-0123456789")
	cfg.AdminTokenEnv = "MY_TOKEN_VAR"
	if e, _, _ := resolveAdminToken(cfg); string(e) != "from-the-environment-0123456789" {
		t.Fatal("the environment variable must win")
	}
	cfg.AdminTokenEnv, cfg.AdminAutoToken = "", false
	if n, _, _ := resolveAdminToken(cfg); len(n) != 0 {
		t.Fatal("a token appeared from nowhere")
	}
	// a whole server starts from it
	s, err := newServer(func() Config { c, _ := LoadConfig(p); c.DataDir = filepath.Join(dir, "data2"); return c }(), []byte("0123456789abcdef0123456789abcdef"))
	if err != nil || len(s.adminTok) != 32 || !s.newAdminToken {
		t.Fatalf("%v", err)
	}
}

func TestPresetsAreSafeToApplyFromThePage(t *testing.T) {
	c, _ := editRig(t, baseFile)
	list := decode(t, adminDo(c.h, "GET", "/admin/presets", ""))["presets"].([]any)
	if len(list) != 3 {
		t.Fatalf("%v", list)
	}
	for _, p := range list {
		id := p.(map[string]any)["id"].(string)
		var doc struct{ Backends BackendsConfig }
		if err := json.Unmarshal(adminDo(c.h, "GET", "/admin/presets/"+id, "").Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if err := checkBackendsForEditing(doc.Backends, "TEST_ADMIN_TOKEN"); err != nil {
			t.Errorf("preset %s cannot be applied from the page: %v", id, err)
		}
		if _, err := buildBackends(doc.Backends); err != nil {
			t.Errorf("preset %s: %v", id, err)
		}
		body, _ := json.Marshal(map[string]any{"backends": doc.Backends})
		if rr := adminDo(c.h, "PUT", "/admin/config", string(body)); rr.Code != 200 {
			t.Errorf("saving preset %s: %d %s", id, rr.Code, rr.Body.String())
		}
	}
	if rr := adminDo(c.h, "GET", "/admin/presets/nope", ""); rr.Code != 404 {
		t.Errorf("%d", rr.Code)
	}
}

func TestDetectAndCaptureReports(t *testing.T) {
	c, _ := editRig(t, baseFile)
	m := decode(t, adminDo(c.h, "GET", "/admin/detect", ""))
	for _, k := range []string{"lan_ips", "tools", "whisper_models", "piper_voices", "ollama", "speech_engine"} {
		if m[k] == nil {
			t.Errorf("detect lacks %q: %v", k, m)
		}
	}
	if cap := decode(t, adminDo(c.h, "GET", "/admin/capture", "")); cap["enabled"] != false {
		t.Fatalf("%v", cap)
	}
	// with capture on, the report counts routes and never shows a robot id
	dir := t.TempDir()
	c.s.cfg.Capture = CaptureConfig{Dir: dir}
	h, err := capturing(c.s.cfg.Capture, c.s.routes())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/time", "/time", "/token/aabbccddeeff", "/emo/dialogue/start", "/nothing/here"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	rep := adminDo(c.h, "GET", "/admin/capture", "")
	if strings.Contains(rep.Body.String(), "aabbccddeeff") {
		t.Fatal("a robot id is in the report")
	}
	var out struct {
		Total  int
		Routes []map[string]any
	}
	json.Unmarshal(rep.Body.Bytes(), &out)
	if out.Total != 5 || out.Routes[0]["path"] != "/time" || out.Routes[0]["count"].(float64) != 2 {
		t.Fatalf("%+v", out)
	}
	notServed := 0
	for _, r := range out.Routes {
		if r["not_served"] == true {
			notServed++
		}
	}
	if notServed != 2 {
		t.Fatalf("not-served routes: %d in %+v", notServed, out.Routes)
	}
}
