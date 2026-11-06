package backend

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// AnthropicConfig talks to the Anthropic Messages API (text and image understanding). This is a paid API; there is no free tier.
type AnthropicConfig struct {
	BaseURL   string `json:"base_url"`    // default https://api.anthropic.com
	Model     string `json:"model"`       // e.g. a small, fast model for a talking robot
	APIKeyEnv string `json:"api_key_env"` // NAME of an environment variable; the key is never in the config
	System    string `json:"system_prompt"`
	MaxTokens int    `json:"max_tokens"` // default 200: the robot speaks briefly
	TimeoutS  int    `json:"timeout_s"`
	Version   string `json:"anthropic_version"` // default 2023-06-01
}

type Anthropic struct {
	r   *remote
	cfg AnthropicConfig
	key string
}

func NewAnthropic(cfg AnthropicConfig) (*Anthropic, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 200
	}
	if cfg.Version == "" {
		cfg.Version = "2023-06-01"
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic: model is required")
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return &Anthropic{r: r, cfg: cfg, key: envKey(cfg.APIKeyEnv)}, nil
}

func (a *Anthropic) Chat(ctx context.Context, msgs []Message) (string, error) {
	type block map[string]any
	type wire struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	system := a.cfg.System
	var out []wire
	for _, m := range msgs {
		if m.Role == "system" { // the API takes the system prompt separately
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		if len(m.ImageJPEG) > 0 {
			out = append(out, wire{role, []block{
				{"type": "image", "source": block{"type": "base64", "media_type": "image/jpeg", "data": base64.StdEncoding.EncodeToString(m.ImageJPEG)}},
				{"type": "text", "text": m.Content},
			}})
		} else {
			out = append(out, wire{role, m.Content})
		}
	}
	if len(out) == 0 {
		return "", errors.New("anthropic: nothing to send")
	}
	req := map[string]any{"model": a.cfg.Model, "max_tokens": a.cfg.MaxTokens, "messages": out}
	if system != "" {
		req["system"] = system
	}
	b, _, err := a.r.call(ctx, "POST", a.r.url("/v1/messages", nil),
		map[string]string{"x-api-key": a.key, "anthropic-version": a.cfg.Version}, req)
	if err != nil {
		return "", err
	}
	var rep struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := jsonUnmarshal(b, &rep); err != nil {
		return "", errors.New("anthropic: unexpected reply")
	}
	var sb strings.Builder
	for _, c := range rep.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", errors.New("anthropic: empty reply")
	}
	return strings.TrimSpace(sb.String()), nil
}
