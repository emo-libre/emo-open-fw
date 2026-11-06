package backend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

// OpenAIConfig describes an OpenAI-compatible HTTP API. That covers OpenAI itself and most local servers
// (llama.cpp, Ollama, vLLM, LocalAI, faster-whisper servers, ...). Only the pieces you use need to be set.
type OpenAIConfig struct {
	BaseURL   string  `json:"base_url"` // e.g. http://127.0.0.1:11434/v1
	Model     string  `json:"model"`
	APIKeyEnv string  `json:"api_key_env"` // NAME of an environment variable; the key itself is never in the config
	Voice     string  `json:"voice"`       // TTS only
	System    string  `json:"system_prompt"`
	MaxTokens int     `json:"max_tokens"`
	Temp      float64 `json:"temperature"`
	TimeoutS  int     `json:"timeout_s"`
	Size      string  `json:"size"` // images only, e.g. "512x512"
	// KeepAlive asks a local model server to keep the model loaded between replies (Ollama: "-1" = always, "30m", "24h").
	// Loading a model takes far longer than the robot waits for an answer. "" = not sent (OpenAI itself rejects the field).
	KeepAlive string `json:"keep_alive"`
}

// keepAliveValue gives the JSON value for keep_alive: Ollama accepts a whole number (seconds; negative = forever) only as a number,
// and a duration such as "30m" only as a string.
func keepAliveValue(s string) any {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return strings.TrimSpace(s)
}

type openAI struct {
	c    *http.Client
	base *url.URL
	cfg  OpenAIConfig
	key  string
}

func newOpenAI(cfg OpenAIConfig) (*openAI, error) {
	c, u, err := NewGuardedClient(cfg.BaseURL, time.Duration(cfg.TimeoutS)*time.Second)
	if err != nil {
		return nil, err
	}
	key := ""
	if cfg.APIKeyEnv != "" {
		key = os.Getenv(cfg.APIKeyEnv)
	}
	return &openAI{c: c, base: u, cfg: cfg, key: key}, nil
}

func (o *openAI) endpoint(p string) string {
	u := *o.base
	u.Path = strings.TrimRight(u.Path, "/") + p
	return u.String()
}

func (o *openAI) do(ctx context.Context, method, path, contentType string, body io.Reader) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, o.endpoint(path), body)
	if err != nil {
		return nil, "", err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if o.key != "" {
		req.Header.Set("Authorization", "Bearer "+o.key)
	}
	resp, err := o.c.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("backend %s: HTTP %d", path, resp.StatusCode)
	}
	return b, resp.Header.Get("Content-Type"), nil
}

// ---- STT ----

type OpenAISTT struct{ o *openAI }

func NewOpenAISTT(cfg OpenAIConfig) (*OpenAISTT, error) {
	o, err := newOpenAI(cfg)
	return &OpenAISTT{o}, err
}

func (s *OpenAISTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "speech.wav")
	fw.Write(wav)
	if s.o.cfg.Model != "" {
		mw.WriteField("model", s.o.cfg.Model)
	}
	if lang != "" {
		mw.WriteField("language", lang)
	}
	mw.WriteField("response_format", "json")
	mw.Close()
	b, _, err := s.o.do(ctx, "POST", "/audio/transcriptions", mw.FormDataContentType(), &buf)
	if err != nil {
		return "", err
	}
	var r struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", fmt.Errorf("backend stt: bad reply: %w", err)
	}
	return strings.TrimSpace(r.Text), nil
}

// ---- LLM (and vision) ----

type OpenAILLM struct{ o *openAI }

func NewOpenAILLM(cfg OpenAIConfig) (*OpenAILLM, error) {
	o, err := newOpenAI(cfg)
	return &OpenAILLM{o}, err
}

// request builds the chat request body shared by Chat and ChatPartial.
func (l *OpenAILLM) request(msgs []Message) map[string]any {
	type part struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	type wireMsg struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	var wire []wireMsg
	if l.o.cfg.System != "" {
		wire = append(wire, wireMsg{"system", l.o.cfg.System})
	}
	for _, m := range msgs {
		if len(m.ImageJPEG) > 0 {
			img := &struct {
				URL string `json:"url"`
			}{"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(m.ImageJPEG)}
			wire = append(wire, wireMsg{m.Role, []part{{Type: "text", Text: m.Content}, {Type: "image_url", ImageURL: img}}})
		} else {
			wire = append(wire, wireMsg{m.Role, m.Content})
		}
	}
	req := map[string]any{"model": l.o.cfg.Model, "messages": wire}
	if l.o.cfg.MaxTokens > 0 {
		req["max_tokens"] = l.o.cfg.MaxTokens
	}
	if l.o.cfg.Temp > 0 {
		req["temperature"] = l.o.cfg.Temp
	}
	if l.o.cfg.KeepAlive != "" {
		req["keep_alive"] = keepAliveValue(l.o.cfg.KeepAlive)
	}
	return req
}

func (l *OpenAILLM) Chat(ctx context.Context, msgs []Message) (string, error) {
	req := l.request(msgs)
	body, _ := json.Marshal(req)
	b, _, err := l.o.do(ctx, "POST", "/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
		return "", errors.New("backend llm: unexpected reply")
	}
	return strings.TrimSpace(r.Choices[0].Message.Content), nil
}

// ---- TTS ----

type OpenAITTS struct{ o *openAI }

func NewOpenAITTS(cfg OpenAIConfig) (*OpenAITTS, error) {
	o, err := newOpenAI(cfg)
	return &OpenAITTS{o}, err
}

func (t *OpenAITTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	voice := t.o.cfg.Voice
	if voice == "" {
		voice = "alloy"
	}
	body, _ := json.Marshal(map[string]any{"model": t.o.cfg.Model, "input": text, "voice": voice, "response_format": "mp3"})
	b, ct, err := t.o.do(ctx, "POST", "/audio/speech", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if !IsMP3(b) {
		return nil, fmt.Errorf("backend tts: reply (%s) is not MP3 audio", ct)
	}
	return b, nil
}

// IsMP3 reports whether b starts with an ID3 tag or an MPEG audio frame sync.
func IsMP3(b []byte) bool {
	if len(b) >= 3 && string(b[:3]) == "ID3" {
		return true
	}
	return len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0
}

// ChatPartial streams the reply. If ctx ends before the model has finished, it returns the text written so far with
// complete = false (and no error), so a caller with a deadline can still use the first sentences. A server that ignores
// "stream" and answers with one ordinary JSON reply is handled too.
func (l *OpenAILLM) ChatPartial(ctx context.Context, msgs []Message) (string, bool, error) {
	req := l.request(msgs)
	req["stream"] = true
	body, _ := json.Marshal(req)
	hreq, err := http.NewRequestWithContext(ctx, "POST", l.o.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if l.o.key != "" {
		hreq.Header.Set("Authorization", "Bearer "+l.o.key)
	}
	resp, err := l.o.c.Do(hreq)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", false, fmt.Errorf("backend /chat/completions: HTTP %d", resp.StatusCode)
	}
	var text, raw strings.Builder
	sawData := false
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxResponseBytes))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			if !sawData {
				raw.WriteString(line)
			}
			continue
		}
		sawData = true
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			return strings.TrimSpace(text.String()), true, nil
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &ch) == nil && len(ch.Choices) > 0 {
			text.WriteString(ch.Choices[0].Delta.Content)
		}
	}
	if ctx.Err() != nil { // the caller's deadline: hand back what was written
		if t := strings.TrimSpace(text.String()); t != "" {
			return t, false, nil
		}
		return "", false, ctx.Err()
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	if !sawData { // not a stream after all: one ordinary reply
		var r struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(raw.String()), &r) != nil || len(r.Choices) == 0 {
			return "", false, errors.New("backend llm: unexpected reply")
		}
		return strings.TrimSpace(r.Choices[0].Message.Content), true, nil
	}
	return strings.TrimSpace(text.String()), true, nil // stream ended without [DONE]
}
