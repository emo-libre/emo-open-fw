package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"emoserver/backend"
)

const (
	esp32URLPrefix = "firmware/esp32/"
	k210URLPrefix  = "firmware/k210/"
)

type versionReply struct {
	VersionName string `json:"version-name"`
	VersionNum  int    `json:"version-num"`
	TestNum     int    `json:"test-version-num,omitempty"` // optional beta channel
	TestName    string `json:"test-version-name,omitempty"`
}

type updatesBlock struct {
	Host      string                       `json:"host"`
	ESP32     string                       `json:"esp32"`
	ESPSha256 string                       `json:"esp_sha256,omitempty"` // informational; the stock client ignores it
	K210      string                       `json:"k210,omitempty"`
	SD        map[string]map[string]string `json:"sd"`
	Model     map[string]any               `json:"model"`
}

type manifest struct {
	VersionNum  int          `json:"version-num"`
	VersionName string       `json:"version-name"`
	Updates     updatesBlock `json:"updates"`
}

// server holds what the handlers need.
type server struct {
	cfg      Config
	key      []byte
	be       backend.Set
	store    *audioStore
	sess     *sessions
	freeSess *sessions // normal-mode follow-up memory; nil = stateless
	prompts  prompts

	ttsCache *ttsCache
	ttsFP    string

	adminTok []byte
	drawSem  chan struct{}
	stats    *stats
	packs    packSet
	custom   *customSet
	act      *activityLog
	throttle authThrottle
	dns      *dnsHelper
	testMu   sync.Mutex

	secrets       *secretStore
	newAdminToken bool   // the admin token was made on this start
	configPath    string // the file the admin page saves settings into
	restartFn     func() // restarts the program (replaced in tests)
	started       time.Time
}

func newServer(cfg Config, key []byte) (*server, error) {
	sec, err := loadSecrets(cfg.DataDir) // before the backends: they read their keys from the environment
	if err != nil {
		return nil, err
	}
	be, err := buildBackends(cfg.Backends)
	if err != nil {
		return nil, err
	}
	pr, err := loadPrompts(cfg.PromptsFile)
	if err != nil {
		return nil, fmt.Errorf("prompts: %w", err)
	}
	s := &server{cfg: cfg, key: key, be: be, store: newAudioStore(), sess: newSessions(cfg.HistoryTurns, sessionIdle), freeSess: newFreeChatSessions(cfg.FreeChatTurns), prompts: pr,
		ttsFP: fingerprintTTS(cfg.Backends.TTS), stats: newStats(), started: time.Now(), act: &activityLog{on: cfg.ActivityLog}}
	conc := cfg.Draw.Concurrency
	if conc < 1 {
		conc = 1
	}
	s.drawSem = make(chan struct{}, conc)
	var created bool
	if s.adminTok, created, err = resolveAdminToken(cfg); err != nil {
		return nil, fmt.Errorf("admin token: %w", err)
	}
	s.newAdminToken = created
	s.secrets = sec
	if s.packs, err = loadPacks(cfg.PacksDir); err != nil {
		return nil, fmt.Errorf("packs: %w", err)
	}
	if s.custom, err = loadCustom(cfg.CustomCommandsFile, cfg.CustomAllowExec); err != nil {
		return nil, err
	}
	if be.TTS != nil && cfg.TTSCacheMB >= 0 {
		mb := cfg.TTSCacheMB
		if mb == 0 {
			mb = 128
		}
		if s.ttsCache, err = newTTSCache(filepath.Join(cfg.DataDir, "tts_cache"), mb); err != nil {
			return nil, fmt.Errorf("tts cache: %w", err)
		}
	}
	return s, nil
}

func (s *server) handleOTAVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, versionReply{VersionName: s.cfg.VersionName, VersionNum: s.cfg.VersionNum,
		TestNum: s.cfg.BetaVersionNum, TestName: s.cfg.BetaVersionName})
}

// handleOTARes answers /emo/ota/res/<n> and /emo/ota/allres/<n>: the update manifest.
//
// The SD and model groups are emitted as well-formed but empty groups (name index "0" holds the directory, no
// entries) so a client that walks them finds nothing to fetch. Whether the stock client tolerates their absence is
// unverified, so they are always present. The k210 key appears only when a K210 image is configured.
func (s *server) handleOTARes(w http.ResponseWriter, r *http.Request) {
	if _, err := strconv.Atoi(r.PathValue("n")); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errcode": 400, "errmsg": "bad version"})
		return
	}
	espPath := filepath.Join(s.cfg.FirmwareDir, s.cfg.ESP32File)
	img, err := os.ReadFile(espPath)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"errcode": 503, "errmsg": "firmware image not available"})
		return
	}
	sum := sha256.Sum256(img)
	sd, model := s.contentManifest()
	m := manifest{
		VersionNum:  s.cfg.VersionNum,
		VersionName: s.cfg.VersionName,
		Updates: updatesBlock{
			Host:      s.cfg.PublicHost,
			ESP32:     esp32URLPrefix + s.cfg.ESP32File,
			ESPSha256: hex.EncodeToString(sum[:]),
			SD:        sd,
			Model:     model,
		},
	}
	if s.cfg.K210File != "" {
		if _, err := os.Stat(filepath.Join(s.cfg.FirmwareDir, s.cfg.K210File)); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"errcode": 503, "errmsg": "k210 image not available"})
			return
		}
		m.Updates.K210 = k210URLPrefix + s.cfg.K210File
	}
	writeJSON(w, http.StatusOK, m)
}

// serveFirmware serves exactly the configured file names; nothing else in firmware_dir is reachable.
func (s *server) serveFirmware(configured string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if configured == "" || r.PathValue("name") != configured {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(s.cfg.FirmwareDir, configured))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, configured, time.Time{}, f)
	}
}
