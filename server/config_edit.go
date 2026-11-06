package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Editing settings from the admin page. Off unless the config says "admin_allow_edit": true, and even then:
//   - only a short list of settings can change (below); listen addresses, the admin token, custom-command programs and access
//     control stay in the file;
//   - the generic "command" provider (any program with any arguments) cannot be chosen here, only the fixed ones (espeak, piper,
//     whispercpp), so a leaked admin token cannot make the server run an arbitrary program;
//   - every change is checked exactly as at start-up, then saved atomically with one backup (config.json.bak), and takes effect
//     after a restart that the page can trigger.

type editableConfig struct {
	PublicHost    *string         `json:"public_host"`
	AudioBaseURL  *string         `json:"audio_base_url"`
	FreeChat      *bool           `json:"free_chat"`
	LLMCommands   *bool           `json:"llm_commands"`
	STTPreprocess *bool           `json:"stt_preprocess"`
	ActivityLog   *bool           `json:"activity_log"`
	TTSCacheMB    *int            `json:"tts_cache_mb"`
	DNSListen     *string         `json:"dns_listen"`
	DNSNames      *[]string       `json:"dns_names"`
	DNSAnswerIP   *string         `json:"dns_answer_ip"`
	DNSUpstream   *string         `json:"dns_upstream"`
	CaptureOn     *bool           `json:"capture_on"`
	Backends      *BackendsConfig `json:"backends"`
}

// applyEditable returns a copy of cfg with the given changes.
func applyEditable(cfg Config, e editableConfig) Config {
	if e.PublicHost != nil {
		cfg.PublicHost = *e.PublicHost
	}
	if e.AudioBaseURL != nil {
		cfg.AudioBaseURL = *e.AudioBaseURL
	}
	if e.FreeChat != nil {
		cfg.FreeChat = *e.FreeChat
	}
	if e.LLMCommands != nil {
		cfg.LLMCommands = *e.LLMCommands
	}
	if e.STTPreprocess != nil {
		cfg.STTPreprocess = *e.STTPreprocess
	}
	if e.ActivityLog != nil {
		cfg.ActivityLog = *e.ActivityLog
	}
	if e.TTSCacheMB != nil {
		cfg.TTSCacheMB = *e.TTSCacheMB
	}
	if e.DNSListen != nil {
		cfg.DNSListen = *e.DNSListen
	}
	if e.DNSNames != nil {
		cfg.DNSNames = *e.DNSNames
	}
	if e.DNSAnswerIP != nil {
		cfg.DNSAnswerIP = *e.DNSAnswerIP
	}
	if e.DNSUpstream != nil {
		cfg.DNSUpstream = *e.DNSUpstream
	}
	if e.CaptureOn != nil {
		if *e.CaptureOn {
			cfg.Capture = CaptureConfig{Dir: filepath.Join(cfg.DataDir, "capture")}
		} else {
			cfg.Capture = CaptureConfig{}
		}
	}
	if e.Backends != nil {
		cfg.Backends = *e.Backends
	}
	return cfg
}

// checkBackendsForEditing refuses what the page may not set: the open-ended program provider, key names that are not key names.
func checkBackendsForEditing(b BackendsConfig, adminEnv string) error {
	for what, c := range map[string]CapConfig{"stt": b.STT, "llm": b.LLM, "tts": b.TTS, "vision": b.Vision} {
		if c.Type == "command" || len(c.Command.Args) > 0 || len(c.Command.ConvertArgs) > 0 {
			return fmt.Errorf("%s: the page cannot set a custom program; choose a built-in engine, or edit the config file yourself", what)
		}
	}
	if b.Image.Type == "command" || len(b.Image.Command.Args) > 0 {
		return errors.New("image: the page cannot set a custom program; edit the config file yourself")
	}
	raw, _ := json.Marshal(b)
	var generic any
	json.Unmarshal(raw, &generic)
	var walk func(v any) error
	walk = func(v any) error {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				if k == "api_key_env" {
					if name, _ := x.(string); name != "" && (!secretName.MatchString(name) || (adminEnv != "" && name == adminEnv)) {
						return fmt.Errorf("%q is not allowed as a key name", name)
					}
				}
				if err := walk(x); err != nil {
					return err
				}
			}
		case []any:
			for _, x := range t {
				if err := walk(x); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(generic)
}

func (s *server) canEdit() bool { return s.cfg.AdminAllowEdit && s.configPath != "" }

func (s *server) needEdit(w http.ResponseWriter) bool {
	if !s.canEdit() {
		msg := `editing is off: set "admin_allow_edit": true in the config file`
		if s.cfg.AdminAllowEdit {
			msg = "the server was started without a config file (use -c config.json), so there is nowhere to save"
		}
		writeJSON(w, http.StatusForbidden, map[string]any{"code": 403, "errmessage": msg})
		return false
	}
	return true
}

func readSmallJSON(w http.ResponseWriter, r *http.Request, v any, max int64) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, max))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": "could not read that: " + err.Error()})
		return false
	}
	return true
}

// editableView is what the page shows and edits.
func (s *server) editableView() map[string]any {
	c := s.cfg
	return map[string]any{
		"public_host": c.PublicHost, "audio_base_url": c.AudioBaseURL, "free_chat": c.FreeChat, "llm_commands": c.LLMCommands,
		"stt_preprocess": c.STTPreprocess, "activity_log": c.ActivityLog, "tts_cache_mb": c.TTSCacheMB,
		"dns_listen": c.DNSListen, "dns_names": nonNil(c.DNSNames), "dns_answer_ip": c.DNSAnswerIP, "dns_upstream": c.DNSUpstream,
		"capture_on": c.Capture.Dir != "", "backends": c.Backends,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (s *server) handleAdminConfigGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"can_edit": s.canEdit(), "config_path": s.configPath, "config": s.editableView(),
		"lan_ips": lanIPs(), "admin_urls": s.adminURLs(), "key_names": sortedNames(s.secrets.names(s.usedKeyNames())),
	})
}

// usedKeyNames lists the key names the current backends refer to.
func (s *server) usedKeyNames() []string {
	raw, _ := json.Marshal(s.cfg.Backends)
	var generic any
	json.Unmarshal(raw, &generic)
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				if name, ok := x.(string); ok && k == "api_key_env" && name != "" {
					out = append(out, name)
				}
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(generic)
	return out
}

func (s *server) handleAdminConfigPut(w http.ResponseWriter, r *http.Request) {
	if !s.needEdit(w) {
		return
	}
	var e editableConfig
	if !readSmallJSON(w, r, &e, 256<<10) {
		return
	}
	if e.Backends != nil {
		if err := checkBackendsForEditing(*e.Backends, s.cfg.AdminTokenEnv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": err.Error()})
			return
		}
	}
	cand := applyEditable(s.cfg, e)
	if err := cand.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": strings.TrimPrefix(err.Error(), "config: ")})
		return
	}
	if _, err := buildBackends(cand.Backends); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": err.Error()})
		return
	}
	// write only the keys that changed into the file, keeping everything else (and its order is not important) as it was
	old, err := os.ReadFile(s.configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "could not read the config file"})
		return
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(old, &doc); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "the config file is not valid JSON; fix it by hand first"})
		return
	}
	put := func(key string, v any) {
		b, _ := json.Marshal(v)
		doc[key] = b
	}
	if e.PublicHost != nil {
		put("public_host", cand.PublicHost)
	}
	if e.AudioBaseURL != nil {
		put("audio_base_url", cand.AudioBaseURL)
	}
	if e.FreeChat != nil {
		put("free_chat", cand.FreeChat)
	}
	if e.LLMCommands != nil {
		put("llm_commands", cand.LLMCommands)
	}
	if e.STTPreprocess != nil {
		put("stt_preprocess", cand.STTPreprocess)
	}
	if e.ActivityLog != nil {
		put("activity_log", cand.ActivityLog)
	}
	if e.TTSCacheMB != nil {
		put("tts_cache_mb", cand.TTSCacheMB)
	}
	if e.DNSListen != nil {
		put("dns_listen", cand.DNSListen)
	}
	if e.DNSNames != nil {
		put("dns_names", nonNil(cand.DNSNames))
	}
	if e.DNSAnswerIP != nil {
		put("dns_answer_ip", cand.DNSAnswerIP)
	}
	if e.DNSUpstream != nil {
		put("dns_upstream", cand.DNSUpstream)
	}
	if e.CaptureOn != nil {
		put("capture", cand.Capture)
	}
	if e.Backends != nil {
		put("backends", cand.Backends)
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	// the saved file must load exactly as the candidate does
	if _, err := decodeConfig(out); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "the new settings did not load back: " + err.Error()})
		return
	}
	_ = writeFileAtomic(s.configPath+".bak", old, 0o600)
	if err := writeFileAtomic(s.configPath, append(out, '\n'), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "could not save: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "restart_needed": true, "backup": filepath.Base(s.configPath) + ".bak"})
}

// ---- keys ----

func (s *server) handleAdminSecretsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"can_edit": s.canEdit(), "names": s.secrets.names(s.usedKeyNames())})
}

func (s *server) handleAdminSecretsPut(w http.ResponseWriter, r *http.Request) {
	if !s.needEdit(w) {
		return
	}
	var in struct{ Name, Value string }
	if !readSmallJSON(w, r, &in, 4<<10) {
		return
	}
	if s.cfg.AdminTokenEnv != "" && in.Name == s.cfg.AdminTokenEnv {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": "that name is the admin token's"})
		return
	}
	if err := s.secrets.set(in.Name, in.Value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "names": s.secrets.names(s.usedKeyNames())})
}

func (s *server) handleAdminSecretsDelete(w http.ResponseWriter, r *http.Request) {
	if !s.needEdit(w) {
		return
	}
	if err := s.secrets.del(r.URL.Query().Get("name")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "could not save"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "names": s.secrets.names(s.usedKeyNames())})
}

// ---- your own commands (without programs) ----

type simpleCommand struct {
	Name      string   `json:"name"`
	Lang      string   `json:"lang"`
	Phrases   []string `json:"phrases"`
	Say       string   `json:"say"`
	Animation string   `json:"animation"`
	Contains  bool     `json:"contains"`
	Exact     bool     `json:"exact"`
}

func (s *server) customFile() string {
	if s.cfg.CustomCommandsFile != "" {
		return s.cfg.CustomCommandsFile
	}
	return filepath.Join(s.cfg.DataDir, "custom_commands.json")
}

func (s *server) handleAdminCommandsFileGet(w http.ResponseWriter, r *http.Request) {
	var simple []simpleCommand
	protected := 0
	for _, c := range s.custom.list() {
		if len(c.Exec) > 0 || c.Intent != "" {
			protected++
			continue
		}
		simple = append(simple, simpleCommand{c.Name, c.Lang, c.Phrases, c.Say, c.Animation, c.Contains, c.Exact})
	}
	if simple == nil {
		simple = []simpleCommand{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"can_edit": s.canEdit(), "commands": simple, "kept_in_file": protected, "file": s.customFile()})
}

func (s *server) handleAdminCommandsFilePut(w http.ResponseWriter, r *http.Request) {
	if !s.needEdit(w) {
		return
	}
	var in struct {
		Commands []simpleCommand `json:"commands"`
	}
	if !readSmallJSON(w, r, &in, 256<<10) {
		return
	}
	var all []*customCommand
	for _, c := range s.custom.list() { // programs and explicit commands are kept exactly as they are
		if len(c.Exec) > 0 || c.Intent != "" {
			all = append(all, c)
		}
	}
	for _, c := range in.Commands {
		all = append(all, &customCommand{Name: c.Name, Lang: c.Lang, Phrases: c.Phrases, Say: c.Say, Animation: c.Animation, Contains: c.Contains, Exact: c.Exact})
	}
	b, _ := json.MarshalIndent(map[string]any{"commands": all}, "", "  ")
	if _, err := parseCustom(b, s.cfg.CustomAllowExec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": err.Error()})
		return
	}
	path := s.customFile()
	if err := writeFileAtomic(path, append(b, '\n'), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "errmessage": "could not save"})
		return
	}
	if s.cfg.CustomCommandsFile == "" { // first time: remember the file in the config too
		s.custom.mu.Lock()
		s.custom.path = path
		s.custom.allowExec = s.cfg.CustomAllowExec
		s.custom.mu.Unlock()
		if old, err := os.ReadFile(s.configPath); err == nil {
			var doc map[string]json.RawMessage
			if json.Unmarshal(old, &doc) == nil {
				doc["custom_commands_file"], _ = json.Marshal(path)
				nb, _ := json.MarshalIndent(doc, "", "  ")
				if _, err := decodeConfig(nb); err == nil {
					_ = writeFileAtomic(s.configPath, append(nb, '\n'), 0o600)
				}
			}
		}
	}
	s.custom.refresh()
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "restart_needed": false})
}

// ---- restart ----

// closeAll stops what the server started: the DNS helper and any speech engine it launched.
func (s *server) closeAll() {
	if s.dns != nil {
		s.dns.close()
	}
	if c, ok := s.be.STT.(io.Closer); ok {
		c.Close()
	}
}

func (s *server) handleAdminRestart(w http.ResponseWriter, r *http.Request) {
	if !s.needEdit(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restarting": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		if s.restartFn != nil {
			s.restartFn()
			return
		}
		s.closeAll()
		exe, err := os.Executable()
		if err != nil {
			exe = os.Args[0]
		}
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			logf("restart failed: %v (stop and start the server by hand)", err)
		}
	}()
}

func decodeConfig(b []byte) (Config, error) {
	c := defaultConfig()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	return c, c.validate()
}
