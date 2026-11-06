package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"emoserver/backend"
)

const (
	defaultReplyBudget = 9 * time.Second         // the robot seems to stop waiting ~10 s after the upload (spec-clean/cloud_api.md, [I])
	fallbackReserve    = 1000 * time.Millisecond // left for the fast fallback voice when the main voice runs late
	speakReserve       = 2500 * time.Millisecond // left after the model for speech: the main voice (~1-1.5 s) plus fallbackReserve
)

// replyBudget is how long a voice request may take after its upload has arrived: reply_budget_ms, 0 = defaultReplyBudget,
// negative = the old generous backendTimeout (for robots or setups where waiting longer is known to work).
func (s *server) replyBudget() time.Duration {
	switch ms := s.cfg.ReplyBudgetMs; {
	case ms == 0:
		return defaultReplyBudget
	case ms < 0:
		return backendTimeout
	default:
		return time.Duration(ms) * time.Millisecond
	}
}

// synthesize speaks text with the main voice. When a fallback voice is configured and ctx has a deadline, the main voice must
// finish fallbackReserve before it; if it is late or fails, the fallback voice speaks instead (fallback = true).
func (s *server) synthesize(ctx context.Context, text, lang string) (mp3 []byte, fallback bool, err error) {
	done := timeStage(ctx, "tts")
	defer done()
	fb := s.be.TTSFallback
	mainCtx := ctx
	if dl, ok := ctx.Deadline(); ok && fb != nil {
		var cancel context.CancelFunc
		mainCtx, cancel = context.WithDeadline(ctx, dl.Add(-fallbackReserve))
		defer cancel()
	}
	t0 := time.Now()
	b, err := s.be.TTS.Synthesize(mainCtx, text, lang)
	if err == nil {
		log.Printf("tts synthesised %d bytes in %d ms", len(b), time.Since(t0).Milliseconds()) // never the text
		return b, false, nil
	}
	if fb == nil || ctx.Err() != nil {
		return nil, false, err
	}
	log.Printf("tts: main voice failed or too slow after %d ms (%v); the fallback voice speaks", time.Since(t0).Milliseconds(), err)
	t1 := time.Now()
	if b, err = fb.Synthesize(ctx, text, lang); err != nil {
		return nil, false, err
	}
	log.Printf("tts fallback synthesised %d bytes in %d ms", len(b), time.Since(t1).Milliseconds())
	return b, true, nil
}

// holdWriter delays the reply until a fixed moment (debug_reply_delay_ms): with simple commands, raising the delay step by step
// shows exactly when the robot stops accepting answers.
type holdWriter struct {
	http.ResponseWriter
	ctx   context.Context
	until time.Time
	held  bool
}

func (h *holdWriter) hold() {
	if h.held {
		return
	}
	h.held = true
	if d := time.Until(h.until); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-h.ctx.Done():
		}
	}
}

func (h *holdWriter) WriteHeader(code int) { h.hold(); h.ResponseWriter.WriteHeader(code) }

func (h *holdWriter) Write(b []byte) (int, error) { h.hold(); return h.ResponseWriter.Write(b) }

// askModel gets a conversation reply. With a deadline and a model that can stream, the model must stop speakReserve before
// the deadline; if it is cut off, the whole sentences it had written are used (a short answer the robot hears beats a long one
// it drops). On the robot a reply was lost when the model took 7 s where it usually takes 2-4 s.
func (s *server) askModel(ctx context.Context, msgs []backend.Message) (string, error) {
	done := timeStage(ctx, "llm")
	defer done()
	p, streams := s.be.LLM.(backend.PartialLLM)
	dl, hasDeadline := ctx.Deadline()
	if !streams || !hasDeadline {
		return s.be.LLM.Chat(ctx, msgs)
	}
	mctx, cancel := context.WithDeadline(ctx, dl.Add(-speakReserve))
	defer cancel()
	text, complete, err := p.ChatPartial(mctx, msgs)
	if err != nil || complete {
		return text, err
	}
	cut := wholeSentences(text)
	log.Printf("model out of time: speaking %d of the %d characters it wrote", len(cut), len(text))
	return cut, nil
}

// wholeSentences keeps text up to the end of its last complete sentence ("" when there is none).
func wholeSentences(text string) string {
	text = strings.TrimSpace(text)
	end := strings.LastIndexAny(text, ".!?…")
	if end < 0 {
		return ""
	}
	rest := text[end:]
	_, size := utf8.DecodeRuneInString(rest)
	end += size
	for end < len(text) && strings.IndexByte(`"')`, text[end]) >= 0 { // a closing quote or bracket belongs to the sentence
		end++
	}
	return strings.TrimSpace(text[:end])
}

// endOnSentence drops a trailing unfinished sentence (a reply cut by the model's token cap) when the reply has a finished
// one before it: "Canberra is the capital. It was chosen in" -> "Canberra is the capital." A reply with no sentence end is kept.
func endOnSentence(text string) string {
	t := strings.TrimSpace(text)
	if t == "" || strings.ContainsAny(t[len(t)-1:], ".!?\"')") || strings.HasSuffix(t, "\u2026") {
		return t
	}
	if cut := wholeSentences(t); cut != "" {
		return cut
	}
	return t
}
