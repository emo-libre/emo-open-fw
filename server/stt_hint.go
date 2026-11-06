package main

import (
	"strings"

	"emoserver/backend"
)

// maxHintChars keeps the hint inside whisper's prompt window (about 224 tokens).
const maxHintChars = 600

// sttHint is the vocabulary hint for one request: the built-in English command names (English only: an English hint pulls
// other languages toward English), then the owner's own command phrases for this language, without duplicates.
func (s *server) sttHint(lang string) string {
	var items []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), ".,;"))
		k := strings.ToLower(p)
		if p == "" || seen[k] || strings.Contains(p, ",") {
			return
		}
		seen[k] = true
		items = append(items, p)
	}
	if lang == "en" {
		for _, p := range strings.Split(strings.TrimSuffix(backend.WhisperHintEN, "."), ",") {
			add(p)
		}
	}
	if s.custom != nil {
		for _, c := range s.custom.list() {
			if c.Lang != "" && c.Lang != lang {
				continue
			}
			for _, p := range c.Phrases {
				add(p)
			}
		}
	}
	out := ""
	for _, it := range items {
		next := it
		if out != "" {
			next = out + ", " + it
		}
		if len(next)+1 > maxHintChars {
			break
		}
		out = next
	}
	if out == "" {
		return ""
	}
	return out + "."
}
