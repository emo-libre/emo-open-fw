package backend

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeWhisper(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "whisper-server")
	if out, err := exec.Command("go", "build", "-o", bin, "./testdata/fakewhisper").CombinedOutput(); err != nil {
		t.Skipf("cannot build the stand-in engine: %v\n%s", err, out)
	}
	return bin
}

func model(t *testing.T, name string) string {
	p := filepath.Join(t.TempDir(), name)
	os.WriteFile(p, []byte("model"), 0o644)
	return p
}

func TestWhisperCppRunsAndReusesTheEngine(t *testing.T) {
	bin := fakeWhisper(t)
	w, err := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-base.en.bin")})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	ctx := context.Background()
	if running, _ := w.Status(); running {
		t.Fatal("started before use")
	}
	got, err := w.Transcribe(ctx, make([]byte, 1000), "de")
	if err != nil || got != "heard 1000 bytes in de" {
		t.Fatalf("%q %v", got, err)
	}
	firstBase := w.base
	if got, _ = w.Transcribe(ctx, make([]byte, 10), "xx-long"); got != "heard 10 bytes in auto" {
		t.Fatalf("odd language: %q", got)
	}
	if w.base != firstBase {
		t.Fatal("the engine was restarted for no reason")
	}
	if running, _ := w.Status(); !running {
		t.Fatal("not running")
	}
	// the engine listens on loopback only
	port := strings.TrimPrefix(w.base, "http://127.0.0.1:")
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second); err != nil {
		t.Fatal(err)
	} else {
		c.Close()
	}
	// killed from outside: the next request starts it again
	killGroup(w.cmd)
	<-w.done
	if got, err = w.Transcribe(ctx, make([]byte, 5), "en"); err != nil || got != "heard 5 bytes in en" {
		t.Fatalf("after a crash: %q %v", got, err)
	}
	// Close stops it for good
	w.Close()
	if running, _ := w.Status(); running {
		t.Fatal("still running after Close")
	}
	if _, err := w.Transcribe(ctx, nil, "en"); err == nil {
		t.Fatal("worked after Close")
	}
}

func TestWhisperCppLanguageOverrideBlankAndSlowStart(t *testing.T) {
	bin := fakeWhisper(t)
	w, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-slow.bin"), Language: "auto"})
	defer w.Close()
	t0 := time.Now()
	got, err := w.Transcribe(context.Background(), make([]byte, 20), "fr")
	if err != nil || got != "heard 20 bytes in auto" || time.Since(t0) < time.Second {
		t.Fatalf("%q %v after %v", got, err, time.Since(t0))
	}
	b, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-blank.bin")})
	defer b.Close()
	if got, err := b.Transcribe(context.Background(), make([]byte, 20), "en"); err != nil || got != "" {
		t.Fatalf("[BLANK_AUDIO] should read as nothing said: %q %v", got, err)
	}
}

func TestWhisperCppReportsAnEngineThatKeepsStopping(t *testing.T) {
	bin := fakeWhisper(t)
	crashModel := model(t, "ggml-crash.bin")
	w, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: crashModel})
	defer w.Close()
	var last error
	for i := 0; i < 4; i++ {
		_, last = w.Transcribe(context.Background(), nil, "en")
		if last == nil {
			t.Fatal("a crashing engine worked")
		}
	}
	if !strings.Contains(last.Error(), "keeps stopping") || !strings.Contains(last.Error(), "boom: cannot load model") {
		t.Fatalf("the cause is not shown: %v", last)
	}
	if _, err := os.Stat("/proc"); err == nil { // no stray processes
		out, _ := exec.Command("pgrep", "-f", crashModel).Output()
		if strings.TrimSpace(string(out)) != "" {
			ps, _ := exec.Command("ps", "-o", "pid,args", "-p", strings.Fields(string(out))[0]).Output()
			t.Fatalf("left processes behind: %s", ps)
		}
	}
}

func TestWhisperCppValidation(t *testing.T) {
	for name, c := range map[string]WhisperCppConfig{
		"relative model":  {Model: "ggml.bin"},
		"relative binary": {Model: "/x/ggml.bin", Binary: "whisper-server"},
		"threads":         {Model: "/x/ggml.bin", Threads: 500},
		"language":        {Model: "/x/ggml.bin", Language: "english"},
	} {
		if _, err := NewWhisperCpp(c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	w, _ := NewWhisperCpp(WhisperCppConfig{Model: "/nonexistent/ggml.bin", Binary: fakeWhisper(t)})
	if _, err := w.Transcribe(context.Background(), nil, "en"); err == nil || !strings.Contains(err.Error(), "model file was not found") {
		t.Errorf("missing model: %v", err)
	}
	t.Setenv("PATH", "")
	w, _ = NewWhisperCpp(WhisperCppConfig{Model: model(t, "ggml-x.bin")})
	if _, err := w.Transcribe(context.Background(), nil, "en"); err == nil || !strings.Contains(err.Error(), "whisper-server is not installed") {
		t.Errorf("missing program: %v", err)
	}
}

func TestEspeakAndPiperChoicesAreChecked(t *testing.T) {
	for name, c := range map[string]EspeakConfig{"voice": {Voice: "en; rm -rf /"}, "fast": {Speed: 9000}, "pitch": {Pitch: 500}} {
		if _, err := NewEspeakTTS(c); err == nil {
			t.Errorf("espeak %s accepted", name)
		}
	}
	for _, v := range []string{"en-us", "de", "cmn", "en+f3", "pt-br"} {
		if _, err := NewEspeakTTS(EspeakConfig{Voice: v}); err != nil {
			t.Errorf("voice %q: %v", v, err)
		}
	}
	for _, m := range []string{"voice.onnx", "/x/voice.bin", "/x/../../etc/passwd"} {
		if _, err := NewPiperTTS(PiperConfig{Model: m}); err == nil {
			t.Errorf("piper model %q accepted", m)
		}
	}
	t.Setenv("PATH", "")
	e, _ := NewEspeakTTS(EspeakConfig{})
	if _, err := e.Synthesize(context.Background(), "hi", "en"); err == nil || !strings.Contains(err.Error(), "espeak-ng is not installed") {
		t.Errorf("%v", err)
	}
	p, _ := NewPiperTTS(PiperConfig{Model: "/x/v.onnx"})
	if _, err := p.Synthesize(context.Background(), "hi", "en"); err == nil || !strings.Contains(err.Error(), "piper is not installed") {
		t.Errorf("%v", err)
	}
}

func TestEspeakSpeaksWhenInstalled(t *testing.T) {
	if _, err := exec.LookPath("espeak-ng"); err != nil {
		t.Skip("espeak-ng not installed")
	}
	if _, err := exec.LookPath("lame"); err != nil {
		t.Skip("lame not installed")
	}
	e, _ := NewEspeakTTS(EspeakConfig{})
	for _, lang := range []string{"en", "de", "zh", "xx"} {
		b, err := e.Synthesize(context.Background(), "hello", lang)
		if err != nil || !IsMP3(b) || len(b) < 500 {
			t.Errorf("%s: %d bytes, %v", lang, len(b), err)
		}
	}
}

func TestProbeOllamaWithoutOllama(t *testing.T) {
	if _, err := ProbeOllama(context.Background()); err == nil {
		t.Log("an Ollama is running here; nothing to check")
	}
}

// English requests carry the vocabulary hint (or the owner's own, or none); a transcript that only repeats the hint is silence.
func TestWhisperCppHint(t *testing.T) {
	bin := fakeWhisper(t)
	for _, c := range []struct{ prompt, lang, want string }{
		{"", "en", fmt.Sprintf("prompt=%x", WhisperHintEN)},
		{"", "de", "prompt="}, // an English hint would pull other languages towards English
		{"Sprocket, Biscuit.", "de", fmt.Sprintf("prompt=%x", "Sprocket, Biscuit.")},
		{"off", "en", "prompt="},
	} {
		w, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-echo.bin"), Prompt: c.prompt})
		got, err := w.Transcribe(context.Background(), make([]byte, 20), c.lang)
		w.Close()
		if err != nil || got != c.want {
			t.Errorf("prompt %q lang %s: sent %q %v, want %q", c.prompt, c.lang, got, err, c.want)
		}
	}
	p, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-parrot.bin")})
	defer p.Close()
	if got, err := p.Transcribe(context.Background(), make([]byte, 20), "en"); err != nil || got != "" {
		t.Fatalf("a repeated hint should read as nothing said: %q %v", got, err)
	}
	ph, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-phantom.bin")})
	defer ph.Close()
	if got, err := ph.Transcribe(context.Background(), make([]byte, 20), "en"); err != nil || got != "" {
		t.Fatalf(`"you" from silence should read as nothing said: %q %v`, got, err)
	}
	for _, real := range []string{"Thank you.", "Bye!", "you and me", "Yes."} {
		if silencePhantom(real) {
			t.Errorf("%q is real speech, not a phantom", real)
		}
	}
	if !silencePhantom(" You.") || !silencePhantom("Thanks for watching!") {
		t.Error("phantoms not recognised")
	}
	if !sameWords("emo chatgpt", "EMO, ChatGPT.") || sameWords("EMO, ChatGPT mode.", "EMO, ChatGPT.") {
		t.Fatal("sameWords")
	}
	// an offered hint is used; the owner's own prompt still wins over it
	e, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-echo.bin")})
	defer e.Close()
	if got, _ := e.TranscribeHint(context.Background(), make([]byte, 20), "en", "Party mode, Disco."); got != fmt.Sprintf("prompt=%x", "Party mode, Disco.") {
		t.Fatalf("offered hint not sent: %q", got)
	}
	o, _ := NewWhisperCpp(WhisperCppConfig{Binary: bin, Model: model(t, "ggml-echo.bin"), Prompt: "Mine."})
	defer o.Close()
	if got, _ := o.TranscribeHint(context.Background(), make([]byte, 20), "en", "Party mode."); got != fmt.Sprintf("prompt=%x", "Mine.") {
		t.Fatalf("owner's prompt should win: %q", got)
	}
	// silence repeating hint items back to back is nothing heard; one command name said on its own is real speech
	for in, phantom := range map[string]bool{
		"Tic Tac Toe, noughts and crosses": true, "rock paper scissors angry emo.": true, WhisperHintEN: true,
		"Let's play tic tac toe.": false, "Tic Tac Toe": false, "fireworks please": false,
	} {
		if hintPhantom(in, WhisperHintEN) != phantom {
			t.Errorf("hintPhantom(%q) = %v", in, !phantom)
		}
	}
}
