package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Local engines the server can run for you, so nobody has to write a command line. Each uses a fixed argument template: the only
// things a config can choose are a voice name, two numbers and a model path, all validated. Text and audio go through stdin, files or
// loopback HTTP, never through a shell.

// ---- espeak-ng + lame: fast, small, robotic voice in many languages ----

type EspeakConfig struct {
	Voice    string `json:"voice"` // "" = pick by the robot's language
	Speed    int    `json:"speed"` // words per minute, 80-450, default 165
	Pitch    int    `json:"pitch"` // 0-99, default 60
	TimeoutS int    `json:"timeout_s"`
}

var espeakVoiceRe = regexp.MustCompile(`^[a-z]{2,3}([+-][a-z0-9]{1,10}){0,2}$`)

var espeakByLang = map[string]string{"en": "en-us", "de": "de", "fr": "fr-fr", "es": "es", "it": "it", "pt": "pt", "ru": "ru", "ja": "ja",
	"ko": "ko", "zh": "cmn", "vi": "vi", "nl": "nl", "pl": "pl", "tr": "tr", "th": "th", "id": "id"}

type EspeakTTS struct{ cfg EspeakConfig }

func NewEspeakTTS(c EspeakConfig) (*EspeakTTS, error) {
	if c.Voice != "" && !espeakVoiceRe.MatchString(c.Voice) {
		return nil, fmt.Errorf("espeak: %q is not a voice name", c.Voice)
	}
	if c.Speed == 0 {
		c.Speed = 165
	}
	if c.Pitch == 0 {
		c.Pitch = 60
	}
	if c.Speed < 80 || c.Speed > 450 || c.Pitch < 0 || c.Pitch > 99 {
		return nil, errors.New("espeak: speed must be 80-450 and pitch 0-99")
	}
	return &EspeakTTS{c}, nil
}

func need(tool, hint string) (string, error) {
	p, err := exec.LookPath(tool)
	if err != nil {
		return "", fmt.Errorf("%s is not installed (%s)", tool, hint)
	}
	return p, nil
}

func toMP3(ctx context.Context, timeoutS int, dir, raw string) ([]byte, error) {
	lame, err := need("lame", "install the 'lame' package")
	if err != nil {
		return nil, err
	}
	final := filepath.Join(dir, "final.mp3")
	if _, err := run(ctx, timeoutS, []string{lame, "--quiet", "-m", "m", "-b", "32", "--resample", "24", raw, final}, nil); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(final)
	if err != nil {
		return nil, err
	}
	if !IsMP3(b) {
		return nil, errors.New("the encoder did not produce MP3")
	}
	return b, nil
}

func (e *EspeakTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	if len([]rune(text)) > 1000 {
		text = string([]rune(text)[:1000])
	}
	bin, err := need("espeak-ng", "install the 'espeak-ng' package")
	if err != nil {
		return nil, err
	}
	voice := e.cfg.Voice
	if voice == "" {
		if voice = espeakByLang[strings.ToLower(lang)]; voice == "" {
			voice = "en-us"
		}
	}
	dir, err := os.MkdirTemp("", "emo-tts-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	raw := filepath.Join(dir, "raw.wav")
	if _, err := run(ctx, e.cfg.TimeoutS, []string{bin, "-v", voice, "-s", strconv.Itoa(e.cfg.Speed), "-p", strconv.Itoa(e.cfg.Pitch), "-w", raw, "--stdin"}, []byte(text)); err != nil {
		return nil, err
	}
	return toMP3(ctx, e.cfg.TimeoutS, dir, raw)
}

// ---- Piper: small neural voices ----

type PiperConfig struct {
	Model    string `json:"model"` // path to a .onnx voice file
	TimeoutS int    `json:"timeout_s"`
}

type PiperTTS struct{ cfg PiperConfig }

func NewPiperTTS(c PiperConfig) (*PiperTTS, error) {
	if !filepath.IsAbs(c.Model) || !strings.HasSuffix(c.Model, ".onnx") {
		return nil, errors.New("piper: model must be the absolute path of a .onnx voice file")
	}
	return &PiperTTS{c}, nil
}

func (p *PiperTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	if len([]rune(text)) > 1000 {
		text = string([]rune(text)[:1000])
	}
	bin, err := need("piper", "see https://github.com/rhasspy/piper")
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(p.cfg.Model); err != nil || !st.Mode().IsRegular() {
		return nil, errors.New("piper: the voice file was not found")
	}
	dir, err := os.MkdirTemp("", "emo-tts-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	raw := filepath.Join(dir, "raw.wav")
	if _, err := run(ctx, p.cfg.TimeoutS, []string{bin, "--model", p.cfg.Model, "--output_file", raw}, []byte(text)); err != nil {
		return nil, err
	}
	return toMP3(ctx, p.cfg.TimeoutS, dir, raw)
}

// ---- whisper.cpp, started and watched by the server ----

type WhisperCppConfig struct {
	Binary   string `json:"binary"`   // "" = whisper-server found on PATH
	Model    string `json:"model"`    // absolute path of a ggml model file (ggml-base.en.bin, ggml-small.bin, ...)
	Threads  int    `json:"threads"`  // 0 = the program's default
	Language string `json:"language"` // "" = the robot's language; "auto" = let the model decide
	Prompt   string `json:"prompt"`   // words to expect: "" = WhisperHintEN for English, "off" = none, else your own
	TimeoutS int    `json:"timeout_s"`
}

// WhisperHintEN primes English transcription with the names the robot's commands use but a speech model rarely expects
// (on the owner's robot it heard "ChatGPT mode" as "Jack Cheepity Mode" and "tic tac toe" as "Tic Tac 2"). On silent audio
// the engine may repeat its hint, so a transcript that is the hint, or two of its items in a row, is read as silence
// (hintPhantom); a person never says two command names back to back.
const WhisperHintEN = "EMO, ChatGPT mode, Tic Tac Toe, noughts and crosses, Rock Paper Scissors, Angry EMO, fix the bugs, hands up, fireworks, " +
	"flip a coin, roll the dice, heads or tails, Bluetooth, home station, firmware, selfie, zodiac sign, lucky number, good night EMO."

// whisperHint returns the hint to send: the owner's own prompt (or none with "off") always wins; otherwise the hint the server
// offered for this request (its vocabulary plus the owner's custom phrases), else the built-in English one.
func (w *WhisperCpp) whisperHint(language, offered string) string {
	p := strings.TrimSpace(w.cfg.Prompt)
	switch {
	case strings.EqualFold(p, "off"):
		return ""
	case p != "":
		return p
	case strings.TrimSpace(offered) != "":
		return strings.TrimSpace(offered)
	case language == "en":
		return WhisperHintEN
	}
	return ""
}

// hintPhantom reports a transcript that only repeats the hint: the whole hint, or two of its comma-separated items back to back.
func hintPhantom(t, hint string) bool {
	if hint == "" {
		return false
	}
	if sameWords(t, hint) {
		return true
	}
	spaced := func(s string) string {
		return " " + strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return ' '
		}, s)), " ") + " "
	}
	var items []string
	for _, it := range strings.Split(hint, ",") {
		if n := strings.TrimSpace(spaced(it)); n != "" {
			items = append(items, n)
		}
	}
	text := spaced(t)
	for i := 0; i+1 < len(items); i++ {
		if strings.Contains(text, " "+items[i]+" "+items[i+1]+" ") {
			return true
		}
	}
	return false
}

// silencePhantoms are transcripts whisper models produce from silence or room noise (3 s of digital silence gives "you" with
// ggml-base.en). Real short answers people give ("bye", "thank you", "yes") are deliberately not here.
var silencePhantoms = []string{"you", "thanks for watching", "thank you for watching", "thanks for watching bye"}

func silencePhantom(t string) bool {
	for _, p := range silencePhantoms {
		if sameWords(t, p) {
			return true
		}
	}
	return false
}

// sameWords compares two texts ignoring case, spaces and punctuation.
func sameWords(a, b string) bool {
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, s)
	}
	return norm(a) == norm(b)
}

// WhisperCpp runs `whisper-server` (from the whisper.cpp project) on a loopback port and sends it recordings. The model stays loaded
// between requests, so answers take a second or two instead of the many seconds a program that reloads the model each time needs.
// The server is started on first use, restarted if it stops (waiting a little between attempts), and stopped by Close.
type WhisperCpp struct {
	cfg WhisperCppConfig

	mu      sync.Mutex
	cmd     *exec.Cmd
	done    chan struct{}
	client  *http.Client
	base    string
	fails   int
	lastTry time.Time
	tail    *tailBuffer
	closed  bool
}

type tailBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.b = append(t.b, p...)
	if len(t.b) > 2048 {
		t.b = t.b[len(t.b)-2048:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.b))
}

func NewWhisperCpp(c WhisperCppConfig) (*WhisperCpp, error) {
	if !filepath.IsAbs(c.Model) {
		return nil, errors.New("whispercpp: model must be the absolute path of a ggml model file")
	}
	if c.Binary != "" && !filepath.IsAbs(c.Binary) {
		return nil, errors.New("whispercpp: binary must be an absolute path (or empty to use whisper-server from PATH)")
	}
	if c.Threads < 0 || c.Threads > 64 {
		return nil, errors.New("whispercpp: threads must be 0-64")
	}
	if c.Language != "" && c.Language != "auto" && !regexp.MustCompile(`^[a-z]{2,3}$`).MatchString(c.Language) {
		return nil, errors.New("whispercpp: language must be a code like en, or auto")
	}
	return &WhisperCpp{cfg: c}, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (w *WhisperCpp) alive() bool {
	if w.done == nil {
		return false
	}
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// ensure makes sure the engine is running and ready, starting it if needed.
func (w *WhisperCpp) ensure(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("whispercpp: stopped")
	}
	if w.alive() {
		return nil
	}
	if w.fails >= 3 {
		return fmt.Errorf("whisper-server keeps stopping: %s", orNone(w.tail))
	}
	if wait := 2*time.Second - time.Since(w.lastTry); w.fails > 0 && wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	w.lastTry = time.Now()
	bin := w.cfg.Binary
	if bin == "" {
		p, err := need("whisper-server", "build whisper.cpp and put whisper-server on PATH, or set \"binary\"")
		if err != nil {
			return err
		}
		bin = p
	}
	if st, err := os.Stat(w.cfg.Model); err != nil || !st.Mode().IsRegular() {
		return errors.New("whispercpp: the model file was not found")
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	args := []string{"-m", w.cfg.Model, "--host", "127.0.0.1", "--port", strconv.Itoa(port)}
	if w.cfg.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(w.cfg.Threads))
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8"}
	w.tail = &tailBuffer{}
	cmd.Stdout, cmd.Stderr = w.tail, w.tail
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		w.fails++
		return fmt.Errorf("whispercpp: could not start: %w", err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	w.cmd, w.done = cmd, done

	// wait until it answers HTTP (loading a model takes a few seconds)
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	probe, _, err := NewGuardedClient(base, 400*time.Millisecond)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/", nil)
		if resp, err := probe.Do(req); err == nil {
			resp.Body.Close()
			break
		}
		select {
		case <-done:
			w.fails++
			return fmt.Errorf("whisper-server stopped while starting: %s", orNone(w.tail))
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			killGroup(cmd)
			w.fails++
			return errors.New("whisper-server did not become ready within two minutes")
		}
	}
	w.base = base
	t := 60 * time.Second
	if w.cfg.TimeoutS > 0 {
		t = time.Duration(w.cfg.TimeoutS) * time.Second
	}
	c, _, err := NewGuardedClient(w.base, t)
	if err != nil {
		return err
	}
	w.client = c
	w.fails = 0
	return nil
}

func orNone(t *tailBuffer) string {
	if t == nil || t.String() == "" {
		return "(it printed nothing)"
	}
	s := t.String()
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}

var bracketOnly = regexp.MustCompile(`^\s*[\[(][^\])]*[\])]\s*$`)

func (w *WhisperCpp) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	return w.TranscribeHint(ctx, wav, lang, "")
}

// TranscribeHint is Transcribe with a vocabulary hint offered by the caller (see whisperHint for who wins).
func (w *WhisperCpp) TranscribeHint(ctx context.Context, wav []byte, lang, offered string) (string, error) {
	if err := w.ensure(ctx); err != nil {
		return "", err
	}
	w.mu.Lock()
	client, base := w.client, w.base
	w.mu.Unlock()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "audio.wav")
	fw.Write(wav)
	language := w.cfg.Language
	if language == "" {
		language = strings.ToLower(lang)
		if len(language) < 2 || len(language) > 3 {
			language = "auto"
		}
	}
	mw.WriteField("language", language)
	hint := w.whisperHint(language, offered)
	if hint != "" {
		mw.WriteField("prompt", hint)
	}
	mw.WriteField("response_format", "json")
	mw.WriteField("temperature", "0.0")
	mw.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/inference", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("whispercpp: the speech engine did not answer")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("whispercpp: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", errors.New("whispercpp: unreadable answer")
	}
	t := strings.TrimSpace(strings.ReplaceAll(out.Text, "\n", " "))
	if bracketOnly.MatchString(t) { // "[BLANK_AUDIO]", "(silence)": nothing was said
		return "", nil
	}
	if hintPhantom(t, hint) { // the engine repeated its hint: nothing was said
		return "", nil
	}
	if silencePhantom(t) { // whisper models write these on silence or noise
		return "", nil
	}
	return t, nil
}

// Close stops the engine.
func (w *WhisperCpp) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.alive() {
		killGroup(w.cmd)
		<-w.done
	}
	return nil
}

// Status reports whether the engine is running right now (for the admin page).
func (w *WhisperCpp) Status() (running bool, lastOutput string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.alive(), orNone(w.tail)
}

// ---- Ollama on this machine ----

// ProbeOllama lists the models a local Ollama offers, or fails if none is running on the default port.
func ProbeOllama(ctx context.Context) ([]string, error) {
	c, u, err := NewGuardedClient("http://127.0.0.1:11434", 1500*time.Millisecond)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String()+"/api/tags", nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.New("not running")
	}
	defer resp.Body.Close()
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return nil, errors.New("unexpected answer")
	}
	var names []string
	for _, m := range out.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
