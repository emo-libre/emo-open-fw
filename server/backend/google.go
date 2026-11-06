package backend

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"
)

// GoogleConfig is shared by Google Cloud Speech-to-Text and Text-to-Speech (API-key access; both have a monthly free allowance,
// check Google's current terms).
type GoogleConfig struct {
	BaseURL   string  `json:"base_url"`    // default https://speech.googleapis.com (STT) / https://texttospeech.googleapis.com (TTS)
	APIKeyEnv string  `json:"api_key_env"` // NAME of an environment variable holding the API key
	Voice     string  `json:"voice"`       // TTS voice name, e.g. one for your language; empty = Google's default for the language
	Rate      float64 `json:"speaking_rate"`
	TimeoutS  int     `json:"timeout_s"`
}

type GoogleSTT struct {
	r   *remote
	key string
}

func NewGoogleSTT(cfg GoogleConfig) (*GoogleSTT, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://speech.googleapis.com"
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return &GoogleSTT{r: r, key: envKey(cfg.APIKeyEnv)}, nil
}

func (g *GoogleSTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	req := map[string]any{
		"config": map[string]any{"encoding": "LINEAR16", "sampleRateHertz": 16000, "languageCode": bcp47(lang), "enableAutomaticPunctuation": true},
		"audio":  map[string]any{"content": base64.StdEncoding.EncodeToString(wav)},
	}
	b, _, err := g.r.call(ctx, "POST", g.r.url("/v1/speech:recognize", url.Values{"key": {g.key}}), nil, req)
	if err != nil {
		return "", err
	}
	var rep struct {
		Results []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	if err := jsonUnmarshal(b, &rep); err != nil {
		return "", errors.New("google stt: unexpected reply")
	}
	var parts []string
	for _, r := range rep.Results {
		if len(r.Alternatives) > 0 {
			parts = append(parts, strings.TrimSpace(r.Alternatives[0].Transcript))
		}
	}
	return strings.TrimSpace(strings.Join(parts, " ")), nil
}

type GoogleTTS struct {
	r   *remote
	key string
	cfg GoogleConfig
}

func NewGoogleTTS(cfg GoogleConfig) (*GoogleTTS, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://texttospeech.googleapis.com"
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return &GoogleTTS{r: r, key: envKey(cfg.APIKeyEnv), cfg: cfg}, nil
}

func (g *GoogleTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	voice := map[string]any{"languageCode": bcp47(lang)}
	if g.cfg.Voice != "" {
		voice["name"] = g.cfg.Voice
	}
	audio := map[string]any{"audioEncoding": "MP3", "sampleRateHertz": 24000}
	if g.cfg.Rate > 0 {
		audio["speakingRate"] = g.cfg.Rate
	}
	b, _, err := g.r.call(ctx, "POST", g.r.url("/v1/text:synthesize", url.Values{"key": {g.key}}), nil,
		map[string]any{"input": map[string]any{"text": text}, "voice": voice, "audioConfig": audio})
	if err != nil {
		return nil, err
	}
	var rep struct {
		AudioContent string `json:"audioContent"`
	}
	if err := jsonUnmarshal(b, &rep); err != nil {
		return nil, errors.New("google tts: unexpected reply")
	}
	mp3, err := base64.StdEncoding.DecodeString(rep.AudioContent)
	if err != nil || !IsMP3(mp3) {
		return nil, errors.New("google tts: reply is not MP3 audio")
	}
	return mp3, nil
}

// ---- ElevenLabs ----

type ElevenLabsConfig struct {
	BaseURL   string `json:"base_url"` // default https://api.elevenlabs.io
	APIKeyEnv string `json:"api_key_env"`
	VoiceID   string `json:"voice_id"`
	Model     string `json:"model"`  // e.g. a fast multilingual model
	Format    string `json:"format"` // default mp3_22050_32 (small and quick)
	TimeoutS  int    `json:"timeout_s"`
}

type ElevenLabs struct {
	r   *remote
	key string
	cfg ElevenLabsConfig
}

func NewElevenLabs(cfg ElevenLabsConfig) (*ElevenLabs, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.elevenlabs.io"
	}
	if cfg.Format == "" {
		cfg.Format = "mp3_22050_32"
	}
	if cfg.VoiceID == "" {
		return nil, errors.New("elevenlabs: voice_id is required")
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return &ElevenLabs{r: r, key: envKey(cfg.APIKeyEnv), cfg: cfg}, nil
}

func (e *ElevenLabs) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	body := map[string]any{"text": text}
	if e.cfg.Model != "" {
		body["model_id"] = e.cfg.Model
	}
	b, _, err := e.r.call(ctx, "POST", e.r.urlSeg("/v1/text-to-speech/", e.cfg.VoiceID, url.Values{"output_format": {e.cfg.Format}}),
		map[string]string{"xi-api-key": e.key, "Accept": "audio/mpeg"}, body)
	if err != nil {
		return nil, err
	}
	if !IsMP3(b) {
		return nil, errors.New("elevenlabs: reply is not MP3 audio")
	}
	return b, nil
}
