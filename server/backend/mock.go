package backend

import (
	"context"
	"strings"
)

// Mock backends: deterministic, offline. For tests and for trying the server without any models.

type MockSTT struct{ Text string }

func (m MockSTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	return m.Text, nil
}

type MockLLM struct{ Reply string }

func (m MockLLM) Chat(ctx context.Context, msgs []Message) (string, error) {
	if m.Reply != "" {
		return m.Reply, nil
	}
	last := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			last = msgs[i].Content
			break
		}
	}
	return "You said: " + strings.TrimSpace(last), nil
}

type MockTTS struct{}

// silentMP3Frame is one MPEG-1 Layer III frame (128 kbit/s, 44.1 kHz, mono-ish header) of digital silence.
func silentMP3(frames int) []byte {
	const frameLen = 417
	out := make([]byte, 0, frames*frameLen)
	for i := 0; i < frames; i++ {
		f := make([]byte, frameLen)
		f[0], f[1], f[2], f[3] = 0xFF, 0xFB, 0x90, 0xC4
		out = append(out, f...)
	}
	return out
}

func (MockTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	n := len(text)/8 + 2 // roughly 26 ms per frame
	if n > 400 {
		n = 400
	}
	return silentMP3(n), nil
}
