package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Custom commands: the owner's own phrases mapped to a spoken line, an animation, an explicit robot command, or a local program.
// The file is JSON (custom_commands_file) and is re-read when it changes:
//
//	{"commands": [
//	  {"name": "party", "phrases": ["party mode", "let's party"], "say": "Party time!", "animation": "Happy"},
//	  {"name": "radio", "lang": "en", "phrases": ["turn on the radio"], "exec": ["/usr/local/bin/radio", "on"], "speak_output": true}
//	]}
//
// Custom commands are tried before the built-in ones, so they can also override a built-in phrase. Programs run only when the
// config sets custom_commands_allow_exec; they get no shell, a fixed environment, a timeout, and their output is size-limited.

type customCommand struct {
	Name        string         `json:"name"`
	Lang        string         `json:"lang"`      // "" or "*": every language; otherwise a base code such as "en"
	Phrases     []string       `json:"phrases"`   // any language; accents and punctuation do not matter
	Exact       bool           `json:"exact"`     // the whole utterance must be the phrase
	Contains    bool           `json:"contains"`  // the phrase may sit inside a sentence of any length (default: most of the utterance)
	Say         string         `json:"say"`       // spoken reply
	Animation   string         `json:"animation"` // animation to play
	Intent      string         `json:"intent"`    // explicit robot command (with behavior and paras)
	Behavior    string         `json:"behavior"`
	Paras       map[string]any `json:"paras"`
	Exec        []string       `json:"exec"`         // program and arguments; {text} and {lang} are replaced; no shell
	TimeoutS    int            `json:"timeout_s"`    // 1..30, default 5
	SpeakOutput bool           `json:"speak_output"` // speak what the program prints
	tokens      [][]string
}

type customSet struct {
	mu        sync.Mutex
	path      string
	allowExec bool
	mtime     time.Time
	cmds      []*customCommand
}

var customName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

const customOutputMax = 2000

// customNorm normalises a phrase or an utterance: lower case, no accents, no punctuation, one token per character for CJK text.
func customNorm(s string) string {
	s = foldDiacritics(s)
	for _, r := range s {
		if r >= 0x2E80 {
			return charNormalize(s)
		}
	}
	return normalizeLoose(s)
}

func loadCustom(path string, allowExec bool) (*customSet, error) {
	cs := &customSet{path: path, allowExec: allowExec}
	if path == "" {
		return cs, nil
	}
	if err := cs.reload(); err != nil {
		return nil, err
	}
	return cs, nil
}

func parseCustom(b []byte, allowExec bool) ([]*customCommand, error) {
	var f struct {
		Commands []*customCommand `json:"commands"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}
	if len(f.Commands) > 500 {
		return nil, errors.New("too many custom commands")
	}
	seen := map[string]bool{}
	for _, c := range f.Commands {
		if !customName.MatchString(c.Name) {
			return nil, fmt.Errorf("command name %q must be 1-40 characters of a-z, 0-9, _ or -", c.Name)
		}
		if seen[c.Name] {
			return nil, fmt.Errorf("command %q appears twice", c.Name)
		}
		seen[c.Name] = true
		if len(c.Phrases) == 0 || len(c.Phrases) > 64 {
			return nil, fmt.Errorf("command %q needs 1-64 phrases", c.Name)
		}
		for _, p := range c.Phrases {
			n := customNorm(p)
			if n == "" || utf8.RuneCountInString(n) < 2 {
				return nil, fmt.Errorf("command %q has an empty or one-letter phrase", c.Name)
			}
			c.tokens = append(c.tokens, strings.Fields(n))
		}
		if c.Say == "" && c.Animation == "" && c.Intent == "" && len(c.Exec) == 0 {
			return nil, fmt.Errorf("command %q does nothing: give it say, animation, intent or exec", c.Name)
		}
		if (c.Intent == "") != (c.Behavior == "") {
			return nil, fmt.Errorf("command %q: intent and behavior go together", c.Name)
		}
		if len(c.Say) > 500 {
			return nil, fmt.Errorf("command %q: say is longer than 500 bytes", c.Name)
		}
		if len(c.Exec) > 0 {
			if !allowExec {
				return nil, fmt.Errorf("command %q runs a program, which needs \"custom_commands_allow_exec\": true in the config", c.Name)
			}
			if !filepath.IsAbs(c.Exec[0]) {
				return nil, fmt.Errorf("command %q: the program must be an absolute path", c.Name)
			}
			if c.TimeoutS == 0 {
				c.TimeoutS = 5
			}
			if c.TimeoutS < 1 || c.TimeoutS > 30 {
				return nil, fmt.Errorf("command %q: timeout_s must be 1-30", c.Name)
			}
		}
		if c.Lang == "*" {
			c.Lang = ""
		}
		c.Lang = strings.ToLower(c.Lang)
	}
	return f.Commands, nil
}

func (cs *customSet) reload() error {
	st, err := os.Stat(cs.path)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(cs.path)
	if err != nil {
		return err
	}
	cmds, err := parseCustom(b, cs.allowExec)
	if err != nil {
		return fmt.Errorf("custom commands %s: %w", cs.path, err)
	}
	cs.mu.Lock()
	cs.cmds, cs.mtime = cmds, st.ModTime()
	cs.mu.Unlock()
	return nil
}

// refresh re-reads the file if it changed; a bad edit keeps the old commands.
func (cs *customSet) refresh() {
	if cs == nil || cs.path == "" {
		return
	}
	if st, err := os.Stat(cs.path); err == nil {
		cs.mu.Lock()
		changed := !st.ModTime().Equal(cs.mtime)
		cs.mu.Unlock()
		if changed {
			if err := cs.reload(); err != nil {
				cs.mu.Lock()
				cs.mtime = st.ModTime() // do not retry until the file changes again
				cs.mu.Unlock()
				logf("%v (keeping the previous commands)", err)
			}
		}
	}
}

func (cs *customSet) list() []*customCommand {
	if cs == nil {
		return nil
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]*customCommand(nil), cs.cmds...)
}

// match finds the custom command for an utterance: the one with the longest matching phrase wins.
func (cs *customSet) match(text, lang string) *customCommand {
	if cs == nil || cs.path == "" {
		return nil
	}
	cs.refresh()
	t := customNorm(text)
	tok := strings.Fields(t)
	if len(tok) == 0 {
		return nil
	}
	padded := " " + t + " "
	var best *customCommand
	bestLen := 0
	for _, c := range cs.list() {
		if c.Lang != "" && c.Lang != lang {
			continue
		}
		for _, pt := range c.tokens {
			ph := strings.Join(pt, " ")
			switch {
			case c.Exact:
				if ph != t {
					continue
				}
			case !strings.Contains(padded, " "+ph+" "):
				continue
			case !c.Contains && len(tok) > len(pt)+5:
				continue
			}
			if len(pt) > bestLen {
				best, bestLen = c, len(pt)
			}
		}
	}
	return best
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, "custom: "+format+"\n", a...) }

// runExec runs the command's program (no shell) and returns what it printed, trimmed and size-limited.
func (c *customCommand) runExec(ctx context.Context, text, lang string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutS)*time.Second)
	defer cancel()
	args := make([]string, len(c.Exec)-1)
	for i, a := range c.Exec[1:] {
		a = strings.ReplaceAll(a, "{text}", text)
		args[i] = strings.ReplaceAll(a, "{lang}", lang)
	}
	cmd := exec.CommandContext(ctx, c.Exec[0], args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "EMO_TEXT=" + text, "EMO_LANG=" + lang}
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len() < customOutputMax {
		room := customOutputMax - b.Len()
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil // swallow the rest rather than failing the program
}
