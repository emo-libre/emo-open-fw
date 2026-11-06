package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
)

// Local answers for the other calls the robot makes in the background, so it does not fall into retry loops.
// Shapes follow emo-libre/Documentation (its "Server" API pages).

func (s *server) handleServerAlt(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"servers": []string{}, "switch": s.cfg.ServerSwitch})
}

// handleReport accepts telemetry. Only a short whitelist of device-health numbers is kept in memory (for the admin API);
// location, time zone, language and everything else is discarded on arrival. Nothing is written to disk.
func (s *server) handleReport(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	s.stats.report(s.deviceID(r), body)
	writeJSON(w, http.StatusOK, map[string]any{"result": "OK"})
}

// handleCloudOnly covers features that need a cloud service (speech, chat, drawing, TTS, dialogue).
func handleCloudOnly(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
	writeJSON(w, http.StatusNotImplemented, map[string]any{"code": 501, "errmessage": "not available in local mode"})
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "errmessage": "not found"})
}

type notice struct {
	ID      int    `json:"id"`
	Message string `json:"message"`
}

// truncRunes cuts s to at most n bytes without splitting a character (the robot stores 256-byte slots).
func truncRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	for len(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}

// handleNotice serves up to five owner-written notices from notices_file ([{"id":1,"message":"..."}]), re-read on every
// request so the file can be edited while the server runs.
func (s *server) handleNotice(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	if s.cfg.NoticesFile != "" {
		if b, err := os.ReadFile(s.cfg.NoticesFile); err == nil {
			var list []notice
			if json.Unmarshal(b, &list) == nil {
				i := 0
				for _, n := range list {
					if n.Message == "" || i >= 5 {
						continue
					}
					out[strconv.Itoa(i)] = map[string]any{"id": n.ID, "message": truncRunes(n.Message, 255)}
					i++
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "errmessage": "ok", "notice": out})
}

// PermissionConfig is the reply to /emo/permission. "normal" with empty text is the plain case; "tagged" makes the robot
// show screen_info (the vendor used it for an ownership line).
type PermissionConfig struct {
	Type       string `json:"type"`
	ScreenInfo string `json:"screen_info"`
}

func (s *server) handlePermission(w http.ResponseWriter, r *http.Request) {
	t := s.cfg.Permission.Type
	if t == "" {
		t = "normal"
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": t, "screen_info": truncRunes(s.cfg.Permission.ScreenInfo, 255)})
}
