package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"emoserver/backend"
)

const (
	backendTimeout = 25 * time.Second
	sessionIdle    = 30 * time.Minute // chat mode: one long conversation
	freeChatIdle   = 5 * time.Minute  // normal mode: only follow-ups to the last few questions
	freeChatTurns  = 3                // normal mode default; small local models do best with a short context
)

// ---- chat sessions (one per robot) ----

type session struct {
	hist []backend.Message
	last time.Time
}

type sessions struct {
	mu   sync.Mutex
	m    map[string]*session
	max  int           // messages kept
	idle time.Duration // a robot silent this long starts afresh
	now  func() time.Time
}

func newSessions(turns int, idle time.Duration) *sessions {
	if turns <= 0 {
		turns = 8
	}
	return &sessions{m: map[string]*session{}, max: turns * 2, idle: idle, now: time.Now}
}

// newFreeChatSessions is the short memory for normal-mode replies: n turns (0 = freeChatTurns), or nil (stateless) when n < 0.
func newFreeChatSessions(n int) *sessions {
	if n < 0 {
		return nil
	}
	if n == 0 {
		n = freeChatTurns
	}
	return newSessions(n, freeChatIdle)
}

func (s *sessions) reset(dev string) {
	s.mu.Lock()
	delete(s.m, dev)
	s.mu.Unlock()
}

// add appends one message and returns a copy of the history.
func (s *sessions) add(dev, role, content string) []backend.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, v := range s.m {
		if now.Sub(v.last) > s.idle {
			delete(s.m, k)
		}
	}
	ss := s.m[dev]
	if ss == nil {
		ss = &session{}
		s.m[dev] = ss
	}
	ss.hist = append(ss.hist, backend.Message{Role: role, Content: content})
	if len(ss.hist) > s.max {
		ss.hist = ss.hist[len(ss.hist)-s.max:]
	}
	ss.last = now
	return append([]backend.Message(nil), ss.hist...)
}

// ---- helpers ----

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// deviceID returns the robot id from a valid bearer token, or "anon".
func (s *server) deviceID(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if tok, ok := strings.CutPrefix(h, "Bearer "); ok {
		if c, err := verifyJWT(s.key, tok, time.Now()); err == nil {
			if sub, ok := c["sub"].(string); ok {
				return sub
			}
		}
	}
	return "anon"
}

// speechURL synthesises text, stores the MP3 and returns the full URL the robot downloads over plain HTTP.
// cache=true is for fixed lines only (prompts, scripted robot text); conversation replies must pass false.
func (s *server) speechURL(ctx context.Context, text, lang string, cache bool) (string, error) {
	if s.be.TTS == nil {
		return "", backend.ErrUnsupported
	}
	text = tidyReply(text)
	if text == "" {
		return "", fmt.Errorf("nothing to say")
	}
	var mp3 []byte
	key := ""
	if cache && s.ttsCache != nil {
		key = cacheKey(s.ttsFP, lang, text)
		if b, ok := s.ttsCache.get(key); ok {
			mp3 = b
		}
	}
	if mp3 == nil {
		b, fallback, err := s.synthesize(ctx, text, lang)
		if err != nil {
			return "", err
		}
		mp3 = b
		if key != "" && !fallback { // the cache holds the main voice only
			s.ttsCache.put(key, b)
		}
	}
	return strings.TrimRight(s.cfg.AudioBaseURL, "/") + s.store.Put(mp3), nil
}

func (s *server) intentReply(queryText, intent, behavior string, paras map[string]any, lang string, index *int) map[string]any {
	if paras == nil {
		paras = map[string]any{}
	}
	m := map[string]any{
		"queryId": newUUID(),
		"queryResult": map[string]any{
			"resultCode":     newUUID(),
			"queryText":      queryText,
			"intent":         map[string]any{"name": intent, "confidence": 1},
			"rec_behavior":   behavior,
			"behavior_paras": paras,
		},
		"languageCode": lang,
	}
	if index != nil {
		m["index"] = *index // the robot drops a reply whose index does not match its request
	}
	return m
}

func queryIndex(r *http.Request) *int {
	if v := r.URL.Query().Get("index"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return &n
		}
	}
	return nil
}

// ---- /emo/voice/detectintent ----

func (s *server) handleDetectIntent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := baseLang(q.Get("languagecode"))
	index := queryIndex(r)
	chatMode := q.Get("role") == "chatgpt"
	dev := s.deviceID(r)
	t0 := time.Now()
	heard, result := "", "nothing heard"
	rctx, stages := withStages(r.Context())
	defer func() {
		took := time.Since(t0)
		log.Printf("voice %s: %s; total %d ms; %s", lang, stages, took.Milliseconds(), result) // never the words heard
		s.act.add(dev, lang, heard, result, took, stages.snapshot())
	}()

	fail := func(text string) {
		writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
	}

	pcm, err := decodeVoiceBody(r.Body)
	if err != nil || s.be.STT == nil {
		if s.be.STT == nil {
			result = "no speech-to-text configured"
		}
		fail("")
		return
	}
	if d := s.cfg.DebugReplyDelayMs; d > 0 {
		w = &holdWriter{ResponseWriter: w, ctx: r.Context(), until: time.Now().Add(time.Duration(d) * time.Millisecond)}
		log.Printf("debug: this voice reply is held until %d ms after the upload", d)
	}
	ctx, cancel := context.WithTimeout(rctx, s.replyBudget()) // the robot stops listening for the answer soon after the upload ends
	defer cancel()
	if s.cfg.STTPreprocess {
		pcm = cleanPCM(pcm)
	}
	doneSTT := timeStage(ctx, "stt")
	var text string
	if hs, ok := s.be.STT.(backend.HintedSTT); ok { // whisper.cpp: hint the robot's command words and the owner's own phrases
		text, err = hs.TranscribeHint(ctx, wavFromPCM(pcm, pcmRate), lang, s.sttHint(lang))
	} else {
		text, err = s.be.STT.Transcribe(ctx, wavFromPCM(pcm, pcmRate), lang)
	}
	doneSTT()
	text = strings.TrimSpace(text)
	if err != nil || text == "" {
		result = "speech-to-text failed or heard nothing"
		fail("")
		return
	}
	heard = text

	if chatMode {
		if isGoodbye(text) {
			result = "chat ended"
			s.sess.reset(dev)
			writeJSON(w, http.StatusOK, s.intentReply(text, "chatgpt_end", "play_animation",
				map[string]any{"animation_name": "chatgpt_end"}, lang, index))
			return
		}
		result = "chat reply"
		s.converse(ctx, w, dev, text, lang, index, s.sess)
		return
	}
	if rt, ok := s.parse(text, lang); ok {
		result = "command: " + rt.Intent
		s.answerIntent(ctx, w, rt, text, lang, index, q.Get("locale"))
		return
	}
	if !s.cfg.FreeChat {
		result = "not understood"
		fail(text)
		return
	}
	if s.cfg.LLMCommands && s.be.LLM != nil {
		result = "model: command or reply"
		s.converseWithCommands(ctx, w, text, lang, index, q.Get("locale"))
		return
	}
	result = "conversation reply"
	s.converse(ctx, w, dev, text, lang, index, s.freeSess)
}

// converse answers with the language model and speaks the result. hist is the memory to use (chat mode's or normal mode's);
// nil answers this one sentence alone.
func (s *server) converse(ctx context.Context, w http.ResponseWriter, dev, text, lang string, index *int, hist *sessions) {
	fail := func() {
		writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
	}
	if s.be.LLM == nil {
		fail()
		return
	}
	var msgs []backend.Message
	if hist != nil {
		msgs = hist.add(dev, "user", text)
	} else {
		msgs = []backend.Message{{Role: "user", Content: text}}
	}
	reply, err := s.askModel(ctx, append(s.personaMessages(lang), msgs...))
	reply = tidyReply(endOnSentence(reply))
	if err != nil || reply == "" {
		fail()
		return
	}
	u, err := s.speechURL(ctx, reply, lang, false)
	if err != nil {
		fail()
		return
	}
	if hist != nil {
		hist.add(dev, "assistant", reply)
	}
	writeJSON(w, http.StatusOK, s.intentReply(text, "chatgpt_speak", "speak",
		map[string]any{"txt": reply, "url": u, "listen": 1}, lang, index))
}

// ---- /emo/chat/start ----

func (s *server) handleChatStart(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := baseLang(q.Get("lang"))
	name := strings.TrimSpace(q.Get("name"))
	if len(name) > 40 {
		name = name[:40]
	}
	s.sess.reset(s.deviceID(r))

	greet := replaceName(s.prompts.text(lang, "chat_start"), name)
	ctx, cancel := context.WithTimeout(r.Context(), backendTimeout)
	defer cancel()
	u, err := s.speechURL(ctx, greet, lang, true)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 1, "errmsg": "speech unavailable", "responsetag": "chatstart"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"errcode": 0, "url": u, "errmsg": "OK", "responsetag": "chatstart"})
}

// ---- /emo/voice/voiceprompt ----

func (s *server) handleVoicePrompt(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<16))
	q := r.URL.Query()
	lang := baseLang(q.Get("languagecode"))
	kind := q.Get("type")
	text := s.prompts.text(lang, kind)
	ctx, cancel := context.WithTimeout(r.Context(), backendTimeout)
	defer cancel()
	u, err := s.speechURL(ctx, text, lang, true)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 1, "errmsg": "speech unavailable",
			"errmessage": "speech unavailable", "responsetag": "voiceprompt"})
		return
	}
	// Both spellings of the status field are sent: the robot's handler reads "errmessage", the sibling chat-start reply uses "errmsg".
	writeJSON(w, http.StatusOK, map[string]any{"errcode": 0, "errmsg": "ok", "errmessage": "ok", "url": u, "responsetag": "voiceprompt"})
}

// ---- /emo/speech/tts ----

func (s *server) handleSpeechTTS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx, cancel := context.WithTimeout(r.Context(), backendTimeout)
	defer cancel()
	u, err := s.speechURL(ctx, q.Get("q"), baseLang(q.Get("l")), true)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "errmessage": "speech unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "errmessage": "ok", "url": u})
}

// ---- /emo/weather/forecast ----

func (s *server) handleWeatherForecast(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	later, _ := strconv.Atoi(q.Get("later"))
	if s.be.Weather == nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 0, "errmsg": "api error"}) // the shape the robot treats as "no data"
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backendTimeout)
	defer cancel()
	f, err := s.be.Weather.Forecast(ctx, q.Get("city"), later)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 0, "errmsg": "api error"})
		return
	}
	// Celsius only: the robot converts to its own display unit setting.
	writeJSON(w, http.StatusOK, map[string]any{"weather": map[string]any{"main": f.Main, "temp_c": f.TempC}})
}

// ---- /emo/ai/imgrecog (photo description) ----

func (s *server) handleImageRecog(w http.ResponseWriter, r *http.Request) {
	if s.be.Vision == nil || s.be.TTS == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"code": 501, "errmessage": "not available in local mode"})
		return
	}
	q := r.URL.Query()
	lang := baseLang(q.Get("languageCode"))
	index := queryIndex(r)
	jpg, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil || len(jpg) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "errmessage": "no image"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*backendTimeout)
	defer cancel()
	doneVision := timeStage(ctx, "vision")
	desc, err := s.be.Vision.Chat(ctx, []backend.Message{{Role: "user",
		Content: "Describe what you see in one short, friendly sentence.", ImageJPEG: jpg}})
	doneVision()
	desc = tidyReply(desc)
	if err != nil || desc == "" {
		writeJSON(w, http.StatusOK, s.intentReply("", "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
		return
	}
	u, err := s.speechURL(ctx, desc, lang, false)
	if err != nil {
		writeJSON(w, http.StatusOK, s.intentReply("", "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
		return
	}
	writeJSON(w, http.StatusOK, s.intentReply("", "image_description", "speak",
		map[string]any{"txt": desc, "url": u}, lang, index))
}

// answerIntent turns a recognised command into the reply, filling in anything that needs a backend.
func (s *server) answerIntent(ctx context.Context, w http.ResponseWriter, rt nluResult, text, lang string, index *int, city string) {
	if rt.Custom != nil {
		s.answerCustom(ctx, w, rt, text, lang, index)
		return
	}
	if rt.NeedWeather {
		if s.be.Weather != nil {
			if f, err := s.be.Weather.Forecast(ctx, city, 0); err == nil {
				rt.Paras["weather"] = map[string]any{"main": f.Main, "temp": float64(int(f.TempC*100+sign(f.TempC)*0.5)) / 100}
				writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, rt.Behavior, rt.Paras, lang, index))
				return
			}
		}
		// no weather source: say so rather than stay silent
		if u, err := s.speechURL(ctx, s.prompts.text(lang, "weather_unavailable"), lang, true); err == nil {
			writeJSON(w, http.StatusOK, s.intentReply(text, "weather", "speak", map[string]any{"url": u}, lang, index))
			return
		}
		writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
		return
	}
	if rt.NeedNews {
		n := s.cfg.Backends.News.Count
		if n < 1 {
			n = 3
		}
		if n > 10 {
			n = 10
		}
		if urls, err := s.newsURLs(ctx, n, lang); err == nil && len(urls) > 0 {
			list := make([]map[string]any, len(urls))
			for i, u := range urls {
				list[i] = map[string]any{"id": i + 1, "url": u}
			}
			rt.Paras["news"] = map[string]any{"count": len(urls), "urls": list}
			writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, rt.Behavior, rt.Paras, lang, index))
			return
		}
		if u, err := s.speechURL(ctx, s.prompts.text(lang, "news_unavailable"), lang, true); err == nil {
			writeJSON(w, http.StatusOK, s.intentReply(text, "news", "speak", map[string]any{"url": u}, lang, index))
			return
		}
		writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
		return
	}
	writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, rt.Behavior, rt.Paras, lang, index))
}

// answerCustom carries out one of the owner's own commands: run the program (if any), then speak, play the animation, or send
// the explicit robot command.
func (s *server) answerCustom(ctx context.Context, w http.ResponseWriter, rt nluResult, text, lang string, index *int) {
	c := rt.Custom
	say, cache := c.Say, true
	if len(c.Exec) > 0 {
		out, err := c.runExec(ctx, text, lang)
		switch {
		case err != nil:
			logf("%s: %v", c.Name, err)
			say, cache = s.prompts.text(lang, "not_execute"), true
		case c.SpeakOutput && out != "":
			if utf8.RuneCountInString(out) > 300 {
				out = string([]rune(out)[:300])
			}
			say, cache = out, false // what a program prints is never cached
		}
	}
	if c.Intent != "" {
		writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, rt.Behavior, rt.Paras, lang, index))
		return
	}
	if say != "" && s.be.TTS != nil {
		if u, err := s.speechURL(ctx, say, lang, cache); err == nil {
			writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, "speak", map[string]any{"url": u}, lang, index))
			return
		}
	}
	anim := c.Animation
	if anim == "" {
		anim = "Happy"
	}
	writeJSON(w, http.StatusOK, s.intentReply(text, rt.Intent, "play_animation", map[string]any{"animation_name": anim}, lang, index))
}

// newsURLs reads the headlines and turns each into a short speech clip (several at once; they are never cached).
func (s *server) newsURLs(ctx context.Context, n int, lang string) ([]string, error) {
	if s.be.News == nil || s.be.TTS == nil {
		return nil, backend.ErrUnsupported
	}
	heads, err := s.be.News.Headlines(ctx, n)
	if err != nil {
		return nil, err
	}
	urls := make([]string, len(heads))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, h := range heads {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, h string) {
			defer wg.Done()
			defer func() { <-sem }()
			if u, err := s.speechURL(ctx, h, lang, false); err == nil {
				urls[i] = u
			}
		}(i, h)
	}
	wg.Wait()
	out := urls[:0]
	for _, u := range urls {
		if u != "" {
			out = append(out, u)
		}
	}
	return out, nil
}

// converseWithCommands asks the model for either a command from the safe list or a short reply, in one call.
func (s *server) converseWithCommands(ctx context.Context, w http.ResponseWriter, text, lang string, index *int, city string) {
	instr := "You control a small robot. If the user is clearly asking the robot to do ONE of these actions, answer with JSON only: " +
		`{"command":"<action>"}` + ". Actions: " + strings.Join(llmKeys(), ", ") +
		". Otherwise answer with JSON only: " + `{"reply":"<one or two short plain sentences>"}` + ". Never add other text."
	msgs := append(s.personaMessages(lang), backend.Message{Role: "system", Content: instr}, backend.Message{Role: "user", Content: text})
	doneLLM := timeStage(ctx, "llm")
	out, err := s.be.LLM.Chat(ctx, msgs)
	doneLLM()
	if err != nil {
		writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
			map[string]any{"animation_name": "dont_understand"}, lang, index))
		return
	}
	cmd, reply := parseModelChoice(out)
	if cmd != "" {
		if t, ok := packTemplates[cmd]; ok && llmSafe(cmd) {
			s.answerIntent(ctx, w, t(), text, lang, index, city)
			return
		}
		reply = "" // an unknown or unsafe command is ignored; fall back to a plain answer below
	}
	if reply == "" {
		reply = strings.TrimSpace(out)
		if strings.HasPrefix(reply, "{") {
			reply = "" // unparseable structured output: do not read braces aloud
		}
	}
	s.speakReply(ctx, w, text, reply, lang, index)
}

// parseModelChoice extracts {"command":..} or {"reply":..} from a model answer (tolerating code fences and extra words).
func parseModelChoice(out string) (cmd, reply string) {
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j <= i {
		return "", ""
	}
	var v struct {
		Command string `json:"command"`
		Reply   string `json:"reply"`
	}
	if json.Unmarshal([]byte(out[i:j+1]), &v) != nil {
		return "", ""
	}
	return strings.TrimSpace(v.Command), strings.TrimSpace(v.Reply)
}

func (s *server) speakReply(ctx context.Context, w http.ResponseWriter, text, reply, lang string, index *int) {
	reply = tidyReply(endOnSentence(reply))
	if reply != "" {
		if u, err := s.speechURL(ctx, reply, lang, false); err == nil {
			writeJSON(w, http.StatusOK, s.intentReply(text, "chatgpt_speak", "speak",
				map[string]any{"txt": reply, "url": u, "listen": 1}, lang, index))
			return
		}
	}
	writeJSON(w, http.StatusOK, s.intentReply(text, "dont_understand", "play_animation",
		map[string]any{"animation_name": "dont_understand"}, lang, index))
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// replaceName fills the {name} slot of a prompt: ", Alex" or nothing.
func replaceName(prompt, name string) string {
	if name != "" {
		return strings.ReplaceAll(prompt, "{name}", ", "+name)
	}
	return strings.ReplaceAll(prompt, "{name}", "")
}
