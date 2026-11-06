package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CommandConfig runs a local program (no shell). Placeholders in Args: {wav} {lang} {out} {in}.
type CommandConfig struct {
	Args        []string `json:"args"`
	ConvertArgs []string `json:"convert_args"` // TTS only: turn the program's WAV output ({in}) into MP3 ({out}), e.g. ffmpeg
	OutFormat   string   `json:"out_format"`   // TTS only: "mp3" (default) or "wav" (requires convert_args)
	TimeoutS    int      `json:"timeout_s"`
}

func subst(args []string, kv map[string]string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		for k, v := range kv {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		out[i] = a
	}
	return out
}

func run(ctx context.Context, timeoutS int, args []string, stdin []byte) ([]byte, error) {
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("backend command: empty args")
	}
	if timeoutS <= 0 {
		timeoutS = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("backend command %q failed: %w", filepath.Base(args[0]), err)
	}
	return out.Bytes(), nil
}

type CommandSTT struct{ cfg CommandConfig }

func NewCommandSTT(cfg CommandConfig) *CommandSTT { return &CommandSTT{cfg} }

func (c *CommandSTT) Transcribe(ctx context.Context, wav []byte, lang string) (string, error) {
	f, err := os.CreateTemp("", "emo-stt-*.wav")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(wav); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	out, err := run(ctx, c.cfg.TimeoutS, subst(c.cfg.Args, map[string]string{"wav": f.Name(), "lang": lang}), nil)
	return strings.TrimSpace(string(out)), err
}

type CommandTTS struct{ cfg CommandConfig }

func NewCommandTTS(cfg CommandConfig) (*CommandTTS, error) {
	if cfg.OutFormat == "wav" && len(cfg.ConvertArgs) == 0 {
		return nil, errors.New("backend tts command: out_format wav needs convert_args (the robot plays MP3)")
	}
	return &CommandTTS{cfg}, nil
}

// Synthesize passes the text on stdin; the program must write its audio to {out}.
func (c *CommandTTS) Synthesize(ctx context.Context, text, lang string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "emo-tts-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	raw := filepath.Join(dir, "raw")
	if _, err := run(ctx, c.cfg.TimeoutS, subst(c.cfg.Args, map[string]string{"out": raw, "lang": lang}), []byte(text)); err != nil {
		return nil, err
	}
	final := raw
	if len(c.cfg.ConvertArgs) > 0 {
		final = filepath.Join(dir, "final.mp3")
		if _, err := run(ctx, c.cfg.TimeoutS, subst(c.cfg.ConvertArgs, map[string]string{"in": raw, "out": final}), nil); err != nil {
			return nil, err
		}
	}
	b, err := os.ReadFile(final)
	if err != nil {
		return nil, err
	}
	if !IsMP3(b) {
		return nil, errors.New("backend tts command: output is not MP3 (set convert_args)")
	}
	return b, nil
}
