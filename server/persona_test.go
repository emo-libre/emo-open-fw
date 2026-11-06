package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emoserver/backend"
)

// The built-in character goes first on every model call, in both free-chat paths, and can be replaced or turned off.
func TestPersonaIsSentToTheModel(t *testing.T) {
	cases := []struct {
		name, persona, want string // want "" = no system message expected
		commands            bool
	}{
		{"default free chat", "", "You are EMO", false},
		{"default with commands", "", "You are EMO", true},
		{"owner's own text", "You are Sprocket, a grumpy toaster.", "Sprocket", false},
		{"turned off", "off", "", false},
		{"turned off, any case", "OFF", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newConv(t, "do you love me")
			c.s.cfg.Persona = tc.persona
			c.s.cfg.LLMCommands = tc.commands
			c.llm.reply = "Of course I do!"
			if tc.commands {
				c.llm.reply = `{"reply":"Of course I do!"}`
			}
			_, r := qr(t, c.post("/emo/voice/detectintent?languagecode=en&index=1", voiceBody(30), ""))
			if r["rec_behavior"] != "speak" || len(c.llm.calls) != 1 {
				t.Fatalf("not answered by the model: %v (calls %d)", r, len(c.llm.calls))
			}
			msgs := c.llm.calls[0]
			if last := msgs[len(msgs)-1]; last.Role != "user" || last.Content != "do you love me" {
				t.Fatalf("user turn missing or not last: %+v", msgs)
			}
			first := msgs[0]
			if tc.want == "" {
				for _, m := range msgs {
					if strings.Contains(m.Content, "You are EMO") {
						t.Fatalf("persona sent although turned off: %+v", msgs)
					}
				}
				return
			}
			if first.Role != "system" || !strings.Contains(first.Content, tc.want) {
				t.Fatalf("first message is not the persona: %+v", first)
			}
			if tc.commands && (len(msgs) != 3 || msgs[1].Role != "system" || !strings.Contains(msgs[1].Content, "JSON")) {
				t.Fatalf("command instruction should follow the persona: %+v", msgs)
			}
		})
	}
}

// The built-in persona must keep the rules that fix what the robot actually said in testing.
func TestDefaultPersonaRules(t *testing.T) {
	for _, must := range []string{"EMO", "robot", "Never call yourself an AI", "language model", "two short sentences", "no emoticons", "stage directions", "Never pretend", "real answer first", "never make things up"} {
		if !strings.Contains(defaultPersona, must) {
			t.Errorf("default persona lost the rule %q", must)
		}
	}
}

// With no max_tokens in the config, an OpenAI-compatible model (ollama included) is still capped.
func TestOpenAILLMDefaultMaxTokens(t *testing.T) {
	for _, set := range []struct{ cfg, want int }{{0, defaultLLMMaxTokens}, {42, 42}} {
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got)
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Beep!"}}]}`)
		}))
		cc := BackendsConfig{LLM: CapConfig{Type: "openai", OpenAI: backend.OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", MaxTokens: set.cfg}}}
		bs, err := buildBackends(cc)
		if err != nil {
			srv.Close()
			t.Fatal(err)
		}
		if _, err := bs.LLM.Chat(context.Background(), []backend.Message{{Role: "user", Content: "hi"}}); err != nil {
			srv.Close()
			t.Fatal(err)
		}
		srv.Close()
		if n, _ := got["max_tokens"].(float64); int(n) != set.want {
			t.Fatalf("config %d: sent max_tokens %v, want %d", set.cfg, got["max_tokens"], set.want)
		}
	}
}

// The model is told the robot's language by name; an unknown code falls back to "the language the person used".
func TestPersonaLanguage(t *testing.T) {
	s := &server{}
	for lang, want := range map[string]string{"en": "Always reply in English.", "de": "Always reply in German.", "xx": "same language the person used", "": "same language the person used"} {
		m := s.personaMessages(lang)
		if len(m) != 1 || !strings.HasSuffix(m[0].Content, want+".") && !strings.HasSuffix(m[0].Content, want) {
			t.Errorf("lang %q: persona does not end with %q", lang, want)
		}
	}
	s.cfg.Persona = "off"
	if m := s.personaMessages("de"); m != nil {
		t.Errorf("persona off still sends %+v", m)
	}
}

// keep_alive goes on the wire in the form Ollama accepts (a whole number as a number, a duration as a string), or not at all.
func TestOpenAILLMKeepAlive(t *testing.T) {
	for _, c := range []struct {
		cfg  string
		want any // nil = field absent
	}{{"", nil}, {"-1", float64(-1)}, {" 24h ", "24h"}, {"30m", "30m"}} {
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &got)
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		}))
		bs, err := buildBackends(BackendsConfig{LLM: CapConfig{Type: "openai", OpenAI: backend.OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", KeepAlive: c.cfg}}})
		if err == nil {
			_, err = bs.LLM.Chat(context.Background(), []backend.Message{{Role: "user", Content: "hi"}})
		}
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		v, present := got["keep_alive"]
		if c.want == nil && present || c.want != nil && v != c.want {
			t.Errorf("keep_alive %q: sent %#v (present %v), want %#v", c.cfg, v, present, c.want)
		}
	}
}

// The start-up warm-up asks the model one short question (so it is loaded before the robot's first one), and copes with no model.
func TestWarmModel(t *testing.T) {
	c := newConv(t, "")
	c.llm.reply = "ok"
	c.s.warmModel(context.Background())
	if len(c.llm.calls) != 1 || len(c.llm.calls[0]) != 1 || c.llm.calls[0][0].Role != "user" {
		t.Fatalf("warm-up calls: %+v", c.llm.calls)
	}
	stt := &recSTT{}
	c.s.be.STT = stt
	c.s.warmModel(context.Background())
	if stt.n != 1 {
		t.Fatalf("speech-to-text not warmed: %d calls", stt.n)
	}
	c.s.be.LLM, c.s.be.STT = nil, nil
	c.s.warmModel(context.Background()) // must not panic
}

type recSTT struct{ n int }

func (r *recSTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	r.n++
	return "", nil
}

// A reply cut by the token cap ends on its last whole sentence; finished replies and replies with no sentence end are kept.
func TestEndOnSentence(t *testing.T) {
	for in, want := range map[string]string{
		"Canberra is the capital of Australia. It was chosen in": "Canberra is the capital of Australia.",
		"Eight legs! And I love watching them":                   "Eight legs!",
		"Eight legs, just like an octopus has eight arms":        "Eight legs, just like an octopus has eight arms",
		"About 384,400 kilometres.":                              "About 384,400 kilometres.",
		"She said \"hi.\"":                                       "She said \"hi.\"",
	} {
		if g := endOnSentence(in); g != want {
			t.Errorf("endOnSentence(%q) = %q, want %q", in, g, want)
		}
	}
}
