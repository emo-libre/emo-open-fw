package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sseServer streams deltas as an OpenAI-compatible server does, pausing `gap` after the first `fast` deltas.
func sseServer(t *testing.T, deltas []string, fast int, gap time.Duration, done bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		if req["stream"] != true {
			t.Errorf("stream not requested: %v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		for i, d := range deltas {
			if i == fast {
				select {
				case <-time.After(gap):
				case <-r.Context().Done():
					return
				}
			}
			c, _ := json.Marshal(d)
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", c)
			f.Flush()
		}
		if done {
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
}

func TestChatPartial(t *testing.T) {
	deltas := []string{"Hello", " there!", " I was about", " to say more."}
	ask := func(srv *httptest.Server, wait time.Duration) (string, bool, error) {
		l, err := NewOpenAILLM(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), wait)
		defer cancel()
		return l.ChatPartial(ctx, []Message{{Role: "user", Content: "hi"}})
	}
	full := sseServer(t, deltas, -1, 0, true)
	defer full.Close()
	if text, complete, err := ask(full, 5*time.Second); err != nil || !complete || text != "Hello there! I was about to say more." {
		t.Fatalf("full stream: %q %v %v", text, complete, err)
	}
	slow := sseServer(t, deltas, 2, 5*time.Second, true) // stalls after "Hello there!"
	defer slow.Close()
	t0 := time.Now()
	if text, complete, err := ask(slow, 400*time.Millisecond); err != nil || complete || text != "Hello there!" {
		t.Fatalf("cut off: %q complete=%v %v", text, complete, err)
	}
	if time.Since(t0) > 2*time.Second {
		t.Fatal("did not stop at the deadline")
	}
	nothing := sseServer(t, deltas, 0, 5*time.Second, true) // stalls before writing anything
	defer nothing.Close()
	if _, _, err := ask(nothing, 300*time.Millisecond); err == nil {
		t.Fatal("cut off with nothing written should be an error")
	}
	noDone := sseServer(t, deltas[:2], -1, 0, false)
	defer noDone.Close()
	if text, complete, err := ask(noDone, 5*time.Second); err != nil || !complete || text != "Hello there!" {
		t.Fatalf("stream without [DONE]: %q %v %v", text, complete, err)
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { // ignores "stream"
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Plain answer."}}]}`)
	}))
	defer plain.Close()
	if text, complete, err := ask(plain, 5*time.Second); err != nil || !complete || text != "Plain answer." {
		t.Fatalf("non-streaming server: %q %v %v", text, complete, err)
	}
}
