package main

import (
	"context"
	"log"
	"time"

	"emoserver/backend"
)

// warmModel asks the language model one tiny question at start-up, so a local model server loads the model now rather than
// during the robot's first question (loading took ~24 s on the owner's laptop; the robot waits about 10 s). With the backend's
// keep_alive set, the model then stays loaded.
func (s *server) warmModel(ctx context.Context) {
	s.warmSTT(ctx)
	if s.be.LLM == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute) // a cold load of a large model can be slow; this is in the background
	defer cancel()
	t0 := time.Now()
	if _, err := s.be.LLM.Chat(ctx, []backend.Message{{Role: "user", Content: "Reply with the single word: ok"}}); err != nil {
		log.Printf("model warm-up failed after %d ms: %v", time.Since(t0).Milliseconds(), err)
		return
	}
	log.Printf("model warm-up: ready in %d ms", time.Since(t0).Milliseconds())
}

// warmSTT sends speech-to-text a short silence at start-up: a managed engine (whisper.cpp) is started on first use, and that
// start (~3-4 s on the owner's laptop) would otherwise come out of the robot's first question.
func (s *server) warmSTT(ctx context.Context) {
	if s.be.STT == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	t0 := time.Now()
	if _, err := s.be.STT.Transcribe(ctx, silentWAV(), "en"); err != nil {
		log.Printf("speech-to-text warm-up failed after %d ms: %v", time.Since(t0).Milliseconds(), err)
		return
	}
	log.Printf("speech-to-text warm-up: ready in %d ms", time.Since(t0).Milliseconds())
}
