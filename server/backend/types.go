// Package backend holds the pluggable speech-to-text, language-model, text-to-speech, vision and weather
// providers. This is the ONLY package allowed to make outbound connections, and only to the hosts the owner
// configured (see guard.go). Nothing here ever talks to the robot vendor.
package backend

import (
	"context"
	"errors"
)

// ErrUnsupported means the configured backend does not provide that capability.
var ErrUnsupported = errors.New("backend: capability not configured")

type Message struct {
	Role      string // "system", "user", "assistant"
	Content   string
	ImageJPEG []byte // optional, vision-capable models only
}

// STT turns 16 kHz mono 16-bit WAV audio into text. lang is a base language code such as "en".
type STT interface {
	Transcribe(ctx context.Context, wav []byte, lang string) (string, error)
}

// LLM produces the assistant's next reply.
type LLM interface {
	Chat(ctx context.Context, msgs []Message) (string, error)
}

// HintedSTT is an STT that accepts a vocabulary hint per request (words the speaker is likely to use), such as whisper.cpp.
type HintedSTT interface {
	TranscribeHint(ctx context.Context, wav []byte, lang, hint string) (string, error)
}

// PartialLLM is an LLM that can stop at the context's deadline and return the text written so far (complete = false)
// instead of failing. Used where an answer must arrive in time, such as replies to the robot.
type PartialLLM interface {
	ChatPartial(ctx context.Context, msgs []Message) (text string, complete bool, err error)
}

// TTS returns MP3 audio for the text (the robot's firmware 3.x plays MP3).
type TTS interface {
	Synthesize(ctx context.Context, text, lang string) ([]byte, error)
}

type Forecast struct {
	Main  string  // sunny | cloudy | rain | snow | dust | fog
	TempC float64 // always Celsius; the robot converts for display according to its own unit setting
}

type Weather interface {
	Forecast(ctx context.Context, city string, laterHours int) (Forecast, error)
}

// ImageRequest is what the robot asks for when it draws. The parameter names are Stable Diffusion WebUI's.
type ImageRequest struct {
	Prompt  string
	Init    []byte // photo to transform (img2img); nil for text-to-image
	Steps   int
	CFG     float64
	Denoise float64 // img2img only
	Sampler string
	Width   int // 0 = backend default
	Height  int
}

// ImageGen returns PNG or JPEG bytes.
type ImageGen interface {
	Generate(ctx context.Context, r ImageRequest) ([]byte, error)
}

// News returns up to n plain-text headlines.
type News interface {
	Headlines(ctx context.Context, n int) ([]string, error)
}

// Set is what the server uses. Any field may be nil: the matching endpoint then reports "not available".
type Set struct {
	STT         STT
	LLM         LLM
	TTS         TTS
	TTSFallback TTS // optional fast voice for replies that would otherwise be too late
	Vision      LLM
	Weather     Weather
	Image       ImageGen
	News        News
}
