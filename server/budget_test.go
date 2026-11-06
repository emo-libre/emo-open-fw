package main

import (
	"context"
	"testing"
	"time"

	"emoserver/backend"
)

// slow backends that honour the request deadline, like real HTTP backends do
type slowTTS struct{ d time.Duration }

func (s slowTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	select {
	case <-time.After(s.d):
		return []byte{0xFF, 0xFB, 1}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type slowLLM struct{ d time.Duration }

func (s slowLLM) Chat(ctx context.Context, m []backend.Message) (string, error) {
	select {
	case <-time.After(s.d):
		return "Late but lovely.", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestReplyBudgetSetting(t *testing.T) {
	s := &server{}
	for ms, want := range map[int]time.Duration{0: defaultReplyBudget, -1: backendTimeout, 4500: 4500 * time.Millisecond} {
		s.cfg.ReplyBudgetMs = ms
		if got := s.replyBudget(); got != want {
			t.Errorf("reply_budget_ms %d: %v, want %v", ms, got, want)
		}
	}
}

// A model slower than the budget no longer holds the robot for 25 s: the request ends at the budget.
func TestVoiceReplyEndsAtTheBudget(t *testing.T) {
	c := newConv(t, "tell me about dinosaurs")
	c.s.cfg.ReplyBudgetMs = 300
	c.s.be.LLM = slowLLM{5 * time.Second}
	t0 := time.Now()
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if took := time.Since(t0); took > 2*time.Second {
		t.Fatalf("reply took %v with a 300 ms budget", took)
	}
	if r["rec_behavior"] != "play_animation" {
		t.Fatalf("a model past the budget should give the not-understood reply: %v", r)
	}
}

// When the main voice cannot finish in time, the fallback voice speaks the reply inside the budget; its audio is not cached
// as the main voice's.
func TestSlowVoiceFallsBack(t *testing.T) {
	c := newConv(t, "tell me about dinosaurs")
	c.s.cfg.ReplyBudgetMs = 1500 // the main voice gets 500 ms (1500 minus the 1000 ms reserve), and needs 3 s
	c.s.be.TTS = slowTTS{3 * time.Second}
	fb := &recTTS{}
	c.s.be.TTSFallback = fb
	t0 := time.Now()
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if took := time.Since(t0); took > 1500*time.Millisecond+500*time.Millisecond {
		t.Fatalf("reply took %v", took)
	}
	if r["rec_behavior"] != "speak" || len(fb.texts) != 1 || fb.texts[0] != c.llm.reply && fb.texts[0] != tidyReply(c.llm.reply) {
		t.Fatalf("fallback voice did not speak the reply: %v (fallback heard %q)", r, fb.texts)
	}
	// fixed lines spoken by the fallback must not enter the cache
	cache, err := newTTSCache(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	c.s.ttsCache = cache
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if _, err := c.s.speechURL(ctx, "A fixed line.", "en", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.s.ttsCache.get(cacheKey(c.s.ttsFP, "en", "A fixed line.")); ok {
		t.Fatal("fallback audio was cached as the main voice")
	}
	// without a fallback, the main voice keeps the whole budget (and here makes it)
	c2 := newConv(t, "tell me about dinosaurs")
	c2.s.cfg.ReplyBudgetMs = 1500
	c2.s.be.TTS = slowTTS{700 * time.Millisecond}
	if _, r := qr(t, c2.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), "")); r["rec_behavior"] != "speak" {
		t.Fatalf("main voice within budget, no fallback: %v", r)
	}
}

// debug_reply_delay_ms holds the reply until that long after the upload, and does not count against the budget.
func TestDebugReplyDelay(t *testing.T) {
	c := newConv(t, "can you dance for me")
	c.s.cfg.DebugReplyDelayMs = 400
	t0 := time.Now()
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	if took := time.Since(t0); took < 400*time.Millisecond {
		t.Fatalf("reply not held: %v", took)
	}
	if r["rec_behavior"] != "dance" {
		t.Fatalf("held reply changed: %v", r)
	}
}

func TestWholeSentences(t *testing.T) {
	for in, want := range map[string]string{
		"Hello there! I was about to": "Hello there!",
		"Done. Fully done.":           "Done. Fully done.",
		"no sentence end yet":         "",
		`She said "hi." and then`:     `She said "hi."`,
		"Wait… what? Oh I see the":    "Wait… what?",
		"":                            "",
	} {
		if g := wholeSentences(in); g != want {
			t.Errorf("wholeSentences(%q) = %q, want %q", in, g, want)
		}
	}
}

// partLLM stands in for a streaming model that is cut off at its deadline.
type partLLM struct {
	text, plain string
	complete    bool
	left        time.Duration // time the model was given before the reply deadline would pass
}

func (p *partLLM) Chat(ctx context.Context, m []backend.Message) (string, error) { return p.plain, nil }

func (p *partLLM) ChatPartial(ctx context.Context, m []backend.Message) (string, bool, error) {
	if dl, ok := ctx.Deadline(); ok {
		p.left = time.Until(dl)
	}
	return p.text, p.complete, nil
}

// A model cut off at its deadline still gets its whole sentences spoken; with nothing whole, the robot gets "not understood".
func TestModelCutOffStillSpeaks(t *testing.T) {
	c := newConv(t, "tell me about dinosaurs")
	c.s.cfg.ReplyBudgetMs = 9000
	m := &partLLM{text: "Dinosaurs were huge! Some of them could", plain: "unused"}
	c.s.be.LLM = m
	_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(50), ""))
	p := r["behavior_paras"].(map[string]any)
	if r["rec_behavior"] != "speak" || p["txt"] != "Dinosaurs were huge!" || c.tts.texts[0] != "Dinosaurs were huge!" {
		t.Fatalf("cut-off reply: %v (tts %q)", r, c.tts.texts)
	}
	if want := 9*time.Second - speakReserve; m.left > want || m.left < want-time.Second {
		t.Fatalf("model was given %v, want about %v (budget minus the speaking reserve)", m.left, want)
	}
	m.text = "Some of them could"
	if _, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=2", voiceBody(50), "")); r["rec_behavior"] != "play_animation" {
		t.Fatalf("no whole sentence should be not-understood: %v", r)
	}
	m.text, m.complete = "A complete answer with no full stop", true
	if _, r = qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=3", voiceBody(50), "")); r["behavior_paras"].(map[string]any)["txt"] != "A complete answer with no full stop" {
		t.Fatalf("a finished reply must not be trimmed: %v", r)
	}
}
