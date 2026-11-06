package main

import (
	_ "embed"
	"net/http"
	"sort"
	"strings"
)

//go:embed ui/admin.html
var adminPage []byte

// handleAdminUI serves the page. It holds no data: the page asks for the admin token and calls the API with it.
func (s *server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; media-src blob:; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(adminPage)
}

// handleAdminParse shows what the robot would be told for a phrase, without calling any backend.
func (s *server) handleAdminParse(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(r.URL.Query().Get("text"))
	lang := baseLang(r.URL.Query().Get("lang"))
	if text == "" || len(text) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": "text is required (up to 500 bytes)"})
		return
	}
	rt, ok := s.parse(text, lang)
	if !ok {
		out := map[string]any{"understood": false, "language": lang}
		switch {
		case !s.cfg.FreeChat:
			out["then"] = "the robot is told it was not understood"
		case s.cfg.LLMCommands && s.be.LLM != nil:
			out["then"] = "the language model may pick a command, or answer in conversation"
		default:
			out["then"] = "the language model answers in conversation"
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	out := map[string]any{"understood": true, "language": lang, "intent": rt.Intent, "behavior": rt.Behavior, "paras": rt.Paras}
	if rt.NeedWeather {
		out["needs"] = "weather"
		if rt.City != "" {
			out["city"] = rt.City
		}
	}
	if rt.NeedNews {
		out["needs"] = "news"
	}
	if rt.Custom != nil {
		out["custom"] = rt.Custom.Name
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAdminLanguages lists the loaded language packs.
func (s *server) handleAdminLanguages(w http.ResponseWriter, r *http.Request) {
	type row struct {
		Code    string `json:"code"`
		Phrases int    `json:"phrases"`
		Lexicon bool   `json:"lexicon"`
	}
	var rows []row
	for code, pl := range s.packs {
		rows = append(rows, row{code, len(pl.phrases), pl.lex != nil})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Code < rows[j].Code })
	writeJSON(w, http.StatusOK, map[string]any{"languages": rows})
}

// handleAdminCommands lists the owner's custom commands (never their program arguments).
func (s *server) handleAdminCommands(w http.ResponseWriter, r *http.Request) {
	list := []map[string]any{}
	for _, c := range s.custom.list() {
		m := map[string]any{"name": c.Name, "phrases": c.Phrases}
		if c.Lang != "" {
			m["lang"] = c.Lang
		}
		if len(c.Exec) > 0 {
			m["runs_program"] = true
		}
		list = append(list, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": list})
}
