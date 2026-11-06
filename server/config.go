package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"emoserver/backend"
)

// Config is the server configuration (JSON file, all fields optional).
type Config struct {
	Listen        string `json:"listen"`       // HTTPS listen address, e.g. ":8443"; "" disables HTTPS
	ListenHTTP    string `json:"listen_http"`  // optional plain-HTTP listener (development only)
	DataDir       string `json:"data_dir"`     // holds cert.pem, key.pem, jwt.key
	FirmwareDir   string `json:"firmware_dir"` // directory the firmware files are served from
	PublicHost    string `json:"public_host"`  // host[:port] the robot should download from (goes into the manifest)
	VersionNum    int    `json:"version_num"`  // must be higher than the version the robot runs
	VersionName   string `json:"version_name"`
	ESP32File     string `json:"esp32_file"`     // file name inside firmware_dir
	K210File      string `json:"k210_file"`      // optional; omitted from the manifest when empty
	ServerSwitch  string `json:"server_switch"`  // value of "switch" in /emo/server/alt; "" = no switch
	LegacyCiphers bool   `json:"legacy_ciphers"` // allow static-RSA TLS 1.2 suites for very old mbedTLS clients

	// Conversation path (all optional; each missing backend just disables its endpoint).
	AudioBaseURL      string           `json:"audio_base_url"` // http://host:port the robot downloads generated speech from (plain HTTP)
	ListenAudio       string           `json:"listen_audio"`   // plain-HTTP listener for those downloads
	Backends          BackendsConfig   `json:"backends"`
	PromptsFile       string           `json:"prompts_file"`         // extra/overriding spoken lines, JSON {lang:{key:text}}
	FreeChat          bool             `json:"free_chat"`            // answer anything the local rules do not handle with the language model
	HistoryTurns      int              `json:"history_turns"`        // chat turns remembered per robot
	ReplyBudgetMs     int              `json:"reply_budget_ms"`      // a voice reply must be ready this soon after the upload: 0 = 9000, -1 = 25 s
	DebugReplyDelayMs int              `json:"debug_reply_delay_ms"` // test only: hold every voice reply until this long after the upload, to find the robot's cut-off
	FreeChatTurns     int              `json:"free_chat_turns"`      // normal-mode turns remembered for follow-ups (5 min idle): 0 = 3, -1 = none
	Persona           string           `json:"persona"`              // character for model replies: "" = built-in EMO, "off" = none, else your own text
	Capture           CaptureConfig    `json:"capture"`
	NoticesFile       string           `json:"notices_file"` // JSON [{"id":1,"message":"..."}], up to 5 shown
	Permission        PermissionConfig `json:"permission"`
	Draw              DrawConfig       `json:"draw"`
	BetaVersionNum    int              `json:"beta_version_num"` // optional beta channel pushed with the version check
	BetaVersionName   string           `json:"beta_version_name"`
	ContentDir        string           `json:"content_dir"`     // the owner's own mp3/avi/mot/json/model files, offered to the robot's updater
	AdminTokenEnv     string           `json:"admin_token_env"` // NAME of an env var holding the admin API token; unset = no admin API
	RequireToken      bool             `json:"require_token"`   // /emo/* needs a valid token issued by this server
	AllowedDevices    []string         `json:"allowed_devices"` // only these robot ids get tokens (empty = any)
	PacksDir          string           `json:"packs_dir"`       // per-language command phrase packs (<lang>.json)
	LLMCommands       bool             `json:"llm_commands"`    // let the language model also pick simple commands
	TTSCacheMB        int              `json:"tts_cache_mb"`    // disk cache for fixed lines; 0 = 128, -1 = off
	NoPrewarm         bool             `json:"no_prewarm"`      // skip loading the model and synthesising the fixed prompts at start-up

	CustomCommandsFile string `json:"custom_commands_file"`       // the owner's own commands (see custom.go); re-read when the file changes
	CustomAllowExec    bool   `json:"custom_commands_allow_exec"` // let custom commands run local programs
	STTPreprocess      bool   `json:"stt_preprocess"`

	// Admin access and editing.
	AdminAutoToken bool `json:"admin_auto_token"` // make and keep an admin token in data_dir/admin_token (no environment variable needed)
	AdminAllowEdit bool `json:"admin_allow_edit"` // let the admin page change settings, secrets and your commands (never programs)

	// Setup helpers.
	ListenAdmin    string   `json:"listen_admin"`     // separate plain-HTTP listener for the admin page and API (use 127.0.0.1:8090); "" = share the main listener
	ListenAdminTLS string   `json:"listen_admin_tls"` // the same admin page over HTTPS with this server\'s own certificate (for example ":8444")
	ActivityLog    bool     `json:"activity_log"`     // keep the last 100 things the robot heard (in memory only) for the admin page
	DNSListen      string   `json:"dns_listen"`       // e.g. ":53": answer dns_names with dns_answer_ip, forward the rest to dns_upstream
	DNSNames       []string `json:"dns_names"`        // host names to point at this server (the robot's API host name)
	DNSAnswerIP    string   `json:"dns_answer_ip"`    // this machine's IPv4 address on the robot's network
	DNSUpstream    string   `json:"dns_upstream"`     // host:port of the resolver for every other name; "" = refuse them             // remove DC offset / rumble and lift quiet recordings before speech-to-text
}

func defaultConfig() Config {
	return Config{
		Listen:       ":8443",
		DataDir:      "./data",
		FirmwareDir:  "./firmware",
		VersionNum:   1000,
		VersionName:  "open-1.0.0",
		ESP32File:    "emo-open-fw.bin",
		ListenAudio:  ":8080",
		FreeChat:     true,
		HistoryTurns: 8,
		Draw:         DrawConfig{Format: "png", Concurrency: 1},
	}
}

// LoadConfig reads path (if non-empty) over the defaults and validates the result.
func LoadConfig(path string) (Config, error) {
	c := defaultConfig()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return c, err
		}
		dec := json.NewDecoder(bytesReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return c, fmt.Errorf("%s: %w", path, err)
		}
	}
	return c, c.validate()
}

func (c Config) validate() error {
	if c.Listen == "" && c.ListenHTTP == "" {
		return errors.New("config: both listen and listen_http are empty")
	}
	if c.VersionNum <= 0 {
		return errors.New("config: version_num must be positive")
	}
	for _, f := range []string{c.ESP32File, c.K210File} {
		if f != "" && f != filepath.Base(f) {
			return fmt.Errorf("config: %q must be a bare file name inside firmware_dir", f)
		}
	}
	if c.ESP32File == "" {
		return errors.New("config: esp32_file is required")
	}
	if (c.BetaVersionNum != 0) != (c.BetaVersionName != "") {
		return errors.New("config: beta_version_num and beta_version_name go together")
	}
	for _, id := range c.AllowedDevices {
		if !deviceID.MatchString(id) {
			return fmt.Errorf("config: allowed_devices entry %q is not 12 lower-case hex characters", id)
		}
	}
	if err := validDNSConfig(c); err != nil {
		return err
	}
	if (c.ListenAdmin != "" || c.ListenAdminTLS != "") && c.AdminTokenEnv == "" && !c.AdminAutoToken {
		return errors.New("config: listen_admin / listen_admin_tls need admin_token_env or admin_auto_token")
	}
	if c.ListenAdminTLS != "" && c.Listen == "" {
		return errors.New("config: listen_admin_tls needs the HTTPS certificate, which is made when listen is set")
	}
	if c.AdminAllowEdit && c.AdminTokenEnv == "" && !c.AdminAutoToken {
		return errors.New("config: admin_allow_edit needs an admin token (admin_token_env or admin_auto_token)")
	}
	if c.Backends.TTS.Type != "" && c.Backends.TTS.Type != "none" {
		u, err := url.Parse(c.AudioBaseURL)
		if err != nil || u.Scheme != "http" || u.Host == "" {
			return errors.New("config: audio_base_url must be an http:// URL the robot can reach (it downloads speech over plain HTTP)")
		}
		if backend.BlockedHost(u.Host) {
			return errors.New("config: audio_base_url points at the robot vendor")
		}
		if c.ListenAudio == "" {
			return errors.New("config: listen_audio is required when text-to-speech is enabled")
		}
	}
	return nil
}
