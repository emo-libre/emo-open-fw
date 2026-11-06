package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const maxImageBytes = 32 << 20

func isImage(b []byte) bool {
	t := http.DetectContentType(b)
	return t == "image/png" || t == "image/jpeg"
}

// ---- Stable Diffusion WebUI (AUTOMATIC1111-compatible) ----

type SDWebUIConfig struct {
	BaseURL        string `json:"base_url"` // e.g. http://127.0.0.1:7860
	TimeoutS       int    `json:"timeout_s"`
	NegativePrompt string `json:"negative_prompt"`
	Width          int    `json:"width"` // defaults used when the robot does not say
	Height         int    `json:"height"`
}

type SDWebUI struct {
	o   *openAI
	cfg SDWebUIConfig
}

func NewSDWebUI(cfg SDWebUIConfig) (*SDWebUI, error) {
	if cfg.TimeoutS == 0 {
		cfg.TimeoutS = 180
	}
	o, err := newOpenAI(OpenAIConfig{BaseURL: cfg.BaseURL, TimeoutS: cfg.TimeoutS})
	return &SDWebUI{o: o, cfg: cfg}, err
}

func (s *SDWebUI) Generate(ctx context.Context, r ImageRequest) ([]byte, error) {
	req := map[string]any{"prompt": r.Prompt, "steps": r.Steps, "cfg_scale": r.CFG, "send_images": true}
	if s.cfg.NegativePrompt != "" {
		req["negative_prompt"] = s.cfg.NegativePrompt
	}
	w, h := r.Width, r.Height
	if w == 0 {
		w, h = s.cfg.Width, s.cfg.Height
	}
	if w > 0 && h > 0 {
		req["width"], req["height"] = w, h
	}
	if r.Sampler != "" {
		req["sampler_name"] = r.Sampler
	}
	path := "/sdapi/v1/txt2img"
	if r.Init != nil {
		path = "/sdapi/v1/img2img"
		req["init_images"] = []string{base64.StdEncoding.EncodeToString(r.Init)}
		req["denoising_strength"] = r.Denoise
	}
	body, _ := json.Marshal(req)
	b, _, err := s.o.do(ctx, "POST", path, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var rep struct {
		Images []string `json:"images"`
	}
	if err := json.Unmarshal(b, &rep); err != nil || len(rep.Images) == 0 {
		return nil, errors.New("backend image: unexpected reply")
	}
	img, err := base64.StdEncoding.DecodeString(rep.Images[0])
	if err != nil || !isImage(img) {
		return nil, errors.New("backend image: reply is not an image")
	}
	return img, nil
}

// ---- OpenAI-compatible /images/generations (text-to-image only) ----

type OpenAIImages struct{ o *openAI }

func NewOpenAIImages(cfg OpenAIConfig) (*OpenAIImages, error) {
	if cfg.TimeoutS == 0 {
		cfg.TimeoutS = 120
	}
	o, err := newOpenAI(cfg)
	return &OpenAIImages{o}, err
}

func (g *OpenAIImages) Generate(ctx context.Context, r ImageRequest) ([]byte, error) {
	if r.Init != nil {
		return nil, ErrUnsupported // image-to-image needs an editing API with masks; not offered here
	}
	req := map[string]any{"model": g.o.cfg.Model, "prompt": r.Prompt, "n": 1, "response_format": "b64_json"}
	if g.o.cfg.Size != "" {
		req["size"] = g.o.cfg.Size
	}
	body, _ := json.Marshal(req)
	b, _, err := g.o.do(ctx, "POST", "/images/generations", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var rep struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &rep); err != nil || len(rep.Data) == 0 {
		return nil, errors.New("backend image: unexpected reply")
	}
	img, err := base64.StdEncoding.DecodeString(rep.Data[0].B64)
	if err != nil || !isImage(img) {
		return nil, errors.New("backend image: reply is not an image")
	}
	return img, nil
}

// ---- local program ----

// CommandImage runs a program: prompt on stdin, image written to {out}; for image-to-image the photo is at {init}.
type CommandImage struct{ cfg CommandConfig }

func NewCommandImage(cfg CommandConfig) *CommandImage { return &CommandImage{cfg} }

func (c *CommandImage) Generate(ctx context.Context, r ImageRequest) ([]byte, error) {
	dir, err := os.MkdirTemp("", "emo-img-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	kv := map[string]string{"out": filepath.Join(dir, "out"), "steps": fmt.Sprint(r.Steps), "cfg": fmt.Sprint(r.CFG),
		"denoise": fmt.Sprint(r.Denoise), "sampler": r.Sampler}
	if r.Init != nil {
		kv["init"] = filepath.Join(dir, "init")
		if err := os.WriteFile(kv["init"], r.Init, 0o600); err != nil {
			return nil, err
		}
	}
	if _, err := run(ctx, c.cfg.TimeoutS, subst(c.cfg.Args, kv), []byte(r.Prompt)); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(kv["out"])
	if err != nil || !isImage(b) {
		return nil, errors.New("backend image command: output is not a PNG/JPEG")
	}
	return b, nil
}

// ---- mock ----

// MockImage returns a small valid PNG (a gradient whose colour depends on the prompt length).
type MockImage struct{}

func (MockImage) Generate(ctx context.Context, r ImageRequest) ([]byte, error) {
	w, h := 64, 48
	if r.Width > 0 && r.Height > 0 && r.Width <= 512 && r.Height <= 512 {
		w, h = r.Width, r.Height
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	base := uint8(len(r.Prompt) * 7)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), base, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- Pollinations: free, keyless text-to-image over a plain GET (the prompt is sent to that service) ----

type PollinationsConfig struct {
	BaseURL  string `json:"base_url"` // default https://image.pollinations.ai
	Model    string `json:"model"`    // optional
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	TimeoutS int    `json:"timeout_s"`
}

type Pollinations struct {
	r   *remote
	cfg PollinationsConfig
}

func NewPollinations(cfg PollinationsConfig) (*Pollinations, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://image.pollinations.ai"
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 120*time.Second)
	if err != nil {
		return nil, err
	}
	return &Pollinations{r: r, cfg: cfg}, nil
}

func (p *Pollinations) Generate(ctx context.Context, q ImageRequest) ([]byte, error) {
	if q.Init != nil {
		return nil, ErrUnsupported // text-to-image only
	}
	v := url.Values{"nologo": {"true"}}
	w, h := p.cfg.Width, p.cfg.Height
	if q.Width > 0 && q.Height > 0 {
		w, h = q.Width, q.Height
	}
	if w > 0 && h > 0 {
		v.Set("width", fmt.Sprint(w))
		v.Set("height", fmt.Sprint(h))
	}
	if p.cfg.Model != "" {
		v.Set("model", p.cfg.Model)
	}
	b, _, err := p.r.call(ctx, "GET", p.r.urlSeg("/prompt/", q.Prompt, v), nil, nil)
	if err != nil {
		return nil, err
	}
	if !isImage(b) {
		return nil, errors.New("pollinations: reply is not a PNG/JPEG")
	}
	return b, nil
}
