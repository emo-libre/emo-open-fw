package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"emoserver/backend"
)

// DrawConfig shapes the pictures sent back to the robot.
type DrawConfig struct {
	Format      string `json:"format"`      // "png" (default), "jpeg", or "passthrough" (exactly what the backend returned)
	MaxDim      int    `json:"max_dim"`     // downscale so the longer side is at most this (0 = keep)
	Concurrency int    `json:"concurrency"` // simultaneous drawings (image models are heavy); default 1
}

const (
	drawTimeout   = 3 * time.Minute
	maxPromptLen  = 600
	maxPhotoBytes = 8 << 20
)

func clampF(v, lo, hi, def float64) float64 {
	if v != v || v < lo || v > hi { // NaN or out of range
		return def
	}
	return v
}

func cleanSampler(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 30 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' || r == '+' || r == '_' || r == '-') {
			return ""
		}
	}
	return s
}

// handleDraw serves /emo/ai/draw/txttoimg and /emo/ai/draw/imgtoimg. The robot passes Stable-Diffusion-style parameters in
// the query (prompt, steps, cfg_scale, denoising_strength, sampler_index); an img2img request carries the photo as the body.
// Reply: {"code":200,"errmessage":"ok","data":"<base64 image>"}.
func (s *server) handleDraw(img2img bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fail := func(code int, msg string) {
			writeJSON(w, code, map[string]any{"code": code, "errmessage": msg})
		}
		if s.be.Image == nil {
			fail(http.StatusNotImplemented, "not available in local mode")
			return
		}
		q := r.URL.Query()
		req := backend.ImageRequest{
			Prompt:  strings.TrimSpace(truncRunes(q.Get("prompt"), maxPromptLen)),
			Steps:   int(clampF(parseF(q.Get("steps")), 1, 50, 20)),
			CFG:     clampF(parseF(q.Get("cfg_scale")), 1, 30, 7),
			Sampler: cleanSampler(q.Get("sampler_index")),
		}
		if req.Prompt == "" {
			fail(http.StatusBadRequest, "no prompt")
			return
		}
		if img2img {
			photo, err := io.ReadAll(io.LimitReader(r.Body, maxPhotoBytes+1))
			if err != nil || len(photo) == 0 || len(photo) > maxPhotoBytes {
				fail(http.StatusBadRequest, "bad photo")
				return
			}
			if t := http.DetectContentType(photo); t != "image/jpeg" && t != "image/png" {
				fail(http.StatusBadRequest, "photo is not a JPEG/PNG")
				return
			}
			req.Init = photo
			req.Denoise = clampF(parseF(q.Get("denoising_strength")), 0, 1, 0.2)
		}
		select { // image models are heavy: one at a time by default, others are told to come back
		case s.drawSem <- struct{}{}:
			defer func() { <-s.drawSem }()
		default:
			fail(http.StatusServiceUnavailable, "busy")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), drawTimeout)
		defer cancel()
		raw, err := s.be.Image.Generate(ctx, req)
		if err != nil {
			fail(http.StatusBadGateway, "image backend failed")
			return
		}
		out, err := finishImage(raw, s.cfg.Draw)
		if err != nil {
			fail(http.StatusBadGateway, "bad image from backend")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 200, "errmessage": "ok", "data": base64.StdEncoding.EncodeToString(out)})
	}
}

func parseF(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return -1e9 // out of every range -> default
	}
	return f
}

// finishImage applies the configured size limit and format.
func finishImage(raw []byte, c DrawConfig) ([]byte, error) {
	if c.Format == "passthrough" {
		return raw, nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if c.MaxDim > 0 {
		img = shrink(img, c.MaxDim)
	}
	var buf bytes.Buffer
	if c.Format == "jpeg" {
		flat := image.NewRGBA(img.Bounds()) // JPEG has no alpha: flatten onto white
		draw.Draw(flat, flat.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
		err = jpeg.Encode(&buf, flat, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&buf, img)
	}
	return buf.Bytes(), err
}

// shrink scales down (never up) so the longer side is at most max, using area averaging.
func shrink(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	nw, nh := max, max*h/w
	if h > w {
		nh, nw = max, max*w/h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
		}
	}
	return dst
}
