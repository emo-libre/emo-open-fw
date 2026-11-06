package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// stageTimes collects how long each backend took while one voice request was answered, so slow STT, model or speech can be
// told apart (the robot gives up on a reply that is too slow, and the capture alone cannot say which stage was the cause).
type stageTimes struct {
	mu sync.Mutex
	ms map[string]int // stage -> total milliseconds (a stage may run more than once, e.g. several TTS lines)
}

type stageKey struct{}

func withStages(ctx context.Context) (context.Context, *stageTimes) {
	st := &stageTimes{ms: map[string]int{}}
	return context.WithValue(ctx, stageKey{}, st), st
}

// timeStage starts timing stage for the request in ctx; call the returned func when the stage ends. No-op outside a voice request.
func timeStage(ctx context.Context, stage string) func() {
	st, _ := ctx.Value(stageKey{}).(*stageTimes)
	if st == nil {
		return func() {}
	}
	t0 := time.Now()
	return func() {
		d := int(time.Since(t0).Milliseconds())
		st.mu.Lock()
		st.ms[stage] += d
		st.mu.Unlock()
	}
}

// snapshot returns a copy of the times.
func (st *stageTimes) snapshot() map[string]int {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make(map[string]int, len(st.ms))
	for k, v := range st.ms {
		out[k] = v
	}
	return out
}

// String gives "stt 812 ms, llm 4301 ms, tts 52 ms" in pipeline order; stages that did not run are left out.
func (st *stageTimes) String() string {
	m := st.snapshot()
	var parts []string
	for _, k := range []string{"stt", "llm", "vision", "tts"} {
		if v, ok := m[k]; ok {
			parts = append(parts, fmt.Sprintf("%s %d ms", k, v))
		}
	}
	return strings.Join(parts, ", ")
}
