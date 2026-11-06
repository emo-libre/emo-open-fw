package main

import (
	"net/http"
	"strings"
)

// normalizePath collapses repeated leading slashes. The robot builds download URLs by joining host and path, and
// the exact joining is unverified, so "//firmware/..." must route the same as "/firmware/...".
func normalizePath(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "//") {
			r.URL.Path = "/" + strings.TrimLeft(r.URL.Path, "/")
			r.URL.RawPath = ""
		}
		h.ServeHTTP(w, r)
	})
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /time", handleTime)
	mux.HandleFunc("GET /token/{id}", s.tokenHandler())

	mux.HandleFunc("GET /emo/ota/version", s.handleOTAVersion)
	mux.HandleFunc("GET /emo/ota/res/{n}", s.handleOTARes)
	mux.HandleFunc("GET /emo/ota/allres/{n}", s.handleOTARes)
	mux.HandleFunc("GET /firmware/esp32/{name}", s.serveFirmware(s.cfg.ESP32File))
	mux.HandleFunc("GET /firmware/k210/{name}", s.serveFirmware(s.cfg.K210File))

	mux.HandleFunc("GET /emo/notice/latest", s.handleNotice)
	mux.HandleFunc("GET /emo/server/alt", s.handleServerAlt)
	mux.HandleFunc("GET /emo/server/list", s.handleServerAlt)
	mux.HandleFunc("GET /emo/permission", s.handlePermission)
	mux.HandleFunc("POST /emo/report/info", s.handleReport)
	mux.HandleFunc("GET /emo/weather/forecast", s.handleWeatherForecast)
	mux.HandleFunc("GET /emo/chat/start", s.handleChatStart)
	mux.HandleFunc("POST /emo/voice/voiceprompt", s.handleVoicePrompt)
	mux.HandleFunc("POST /emo/voice/detectintent", s.handleDetectIntent)
	mux.HandleFunc("GET /emo/speech/tts", s.handleSpeechTTS)
	mux.HandleFunc("POST /emo/ai/imgrecog", s.handleImageRecog)
	mux.HandleFunc("GET /mp3/download/{file}", s.store.handle)
	mux.HandleFunc("POST /emo/ai/draw/txttoimg", s.handleDraw(false))
	mux.HandleFunc("POST /emo/ai/draw/imgtoimg", s.handleDraw(true))

	for _, p := range []string{"/emo/voice/", "/emo/speech/", "/emo/chat/", "/emo/ai/", "/emo/dialogue/"} {
		mux.HandleFunc(p, handleCloudOnly)
	}
	for _, g := range append(append([]string{}, contentGroups...), "model") {
		mux.HandleFunc("GET /"+g+"/{name}", s.serveContent(g))
	}
	if len(s.adminTok) > 0 && s.cfg.ListenAdmin == "" && s.cfg.ListenAdminTLS == "" { // with listen_admin the admin pages live on their own listener instead
		s.registerAdmin(mux)
	}
	mux.HandleFunc("/", handleNotFound)

	return logged(normalizePath(s.gate(mux)))
}

// audioRoutes serves only generated speech; it is what the plain-HTTP listener exposes.
func (s *server) audioRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /mp3/download/{file}", s.store.handle)
	mux.HandleFunc("/", handleNotFound)
	return logged(mux)
}

func (s *server) registerAdmin(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/ui", s.handleAdminUI)
	mux.HandleFunc("GET /admin/status", s.adminAuth(s.handleAdminStatus))
	mux.HandleFunc("GET /admin/robots", s.adminAuth(s.handleAdminRobots))
	mux.HandleFunc("GET /admin/parse", s.adminAuth(s.handleAdminParse))
	mux.HandleFunc("GET /admin/languages", s.adminAuth(s.handleAdminLanguages))
	mux.HandleFunc("GET /admin/commands", s.adminAuth(s.handleAdminCommands))
	mux.HandleFunc("GET /admin/setup", s.adminAuth(s.handleAdminSetup))
	mux.HandleFunc("GET /admin/activity", s.adminAuth(s.handleAdminActivity))
	mux.HandleFunc("DELETE /admin/activity", s.adminAuth(s.handleAdminActivityClear))
	mux.HandleFunc("GET /admin/selftest", s.adminAuth(s.handleAdminSelfTest))
	mux.HandleFunc("GET /admin/say", s.adminAuth(s.handleAdminSay))
	mux.HandleFunc("GET /admin/detect", s.adminAuth(s.handleAdminDetect))
	mux.HandleFunc("GET /admin/presets", s.adminAuth(s.handleAdminPresets))
	mux.HandleFunc("GET /admin/presets/{id}", s.adminAuth(s.handleAdminPreset))
	mux.HandleFunc("GET /admin/capture", s.adminAuth(s.handleAdminCapture))
	mux.HandleFunc("GET /admin/config", s.adminAuth(s.handleAdminConfigGet))
	mux.HandleFunc("PUT /admin/config", s.adminAuth(s.handleAdminConfigPut))
	mux.HandleFunc("GET /admin/secrets", s.adminAuth(s.handleAdminSecretsGet))
	mux.HandleFunc("PUT /admin/secrets", s.adminAuth(s.handleAdminSecretsPut))
	mux.HandleFunc("DELETE /admin/secrets", s.adminAuth(s.handleAdminSecretsDelete))
	mux.HandleFunc("GET /admin/commands/file", s.adminAuth(s.handleAdminCommandsFileGet))
	mux.HandleFunc("PUT /admin/commands/file", s.adminAuth(s.handleAdminCommandsFilePut))
	mux.HandleFunc("POST /admin/restart", s.adminAuth(s.handleAdminRestart))
}

// adminRoutes is the separate admin listener's handler: the admin page and API, and nothing a robot can use.
func (s *server) adminRoutes() http.Handler {
	mux := http.NewServeMux()
	s.registerAdmin(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin/ui", http.StatusFound) })
	mux.HandleFunc("/", handleNotFound)
	return logged(mux)
}
