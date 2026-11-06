package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"emoserver/backend"
)

// ttsCache keeps synthesised MP3s of FIXED lines (spoken prompts, greetings, the robot's own scripted text) on disk, so a
// repeated line costs a file read instead of a synthesis. Conversation replies and image descriptions are never cached:
// they are personal and never repeat.
type ttsCache struct {
	dir string
	max int64
	mu  sync.Mutex
}

func newTTSCache(dir string, maxMB int) (*ttsCache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &ttsCache{dir: dir, max: int64(maxMB) << 20}, nil
}

// cacheKey binds a line to the language and to the TTS configuration, so changing voice or model never serves stale audio.
func cacheKey(fingerprint, lang, text string) string {
	h := sha256.Sum256([]byte(fingerprint + "\x00" + lang + "\x00" + text))
	return hex.EncodeToString(h[:])
}

func (c *ttsCache) path(key string) string { return filepath.Join(c.dir, key+".mp3") }

func (c *ttsCache) get(key string) ([]byte, bool) {
	p := c.path(key)
	b, err := os.ReadFile(p)
	if err != nil || !backend.IsMP3(b) {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now) // recently used = recently modified (for pruning)
	return b, true
}

func (c *ttsCache) put(key string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tmp := c.path(key) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, c.path(key)); err != nil {
		_ = os.Remove(tmp)
		return
	}
	c.pruneLocked()
}

func (c *ttsCache) pruneLocked() {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type f struct {
		name string
		size int64
		mod  time.Time
	}
	var fs []f
	var total int64
	for _, e := range ents {
		if filepath.Ext(e.Name()) != ".mp3" {
			continue
		}
		if i, err := e.Info(); err == nil {
			fs = append(fs, f{e.Name(), i.Size(), i.ModTime()})
			total += i.Size()
		}
	}
	if total <= c.max {
		return
	}
	sort.Slice(fs, func(a, b int) bool { return fs[a].mod.Before(fs[b].mod) })
	for _, x := range fs {
		if total <= c.max*8/10 {
			break
		}
		if os.Remove(filepath.Join(c.dir, x.name)) == nil {
			total -= x.size
		}
	}
}

func fingerprintTTS(c CapConfig) string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// prewarm synthesises every fixed prompt in the background so the first refusal or greeting is instant.
func (s *server) prewarm(ctx context.Context) {
	if s.be.TTS == nil || s.ttsCache == nil {
		return
	}
	start := time.Now()
	n, fresh := 0, 0
	for lang, m := range s.prompts {
		for key, text := range m {
			if key == "chat_start" {
				text = replaceName(text, "")
			}
			n++
			cctx, cancel := context.WithTimeout(ctx, backendTimeout)
			k := cacheKey(s.ttsFP, lang, tidyReply(text))
			if _, ok := s.ttsCache.get(k); !ok {
				if mp3, err := s.be.TTS.Synthesize(cctx, tidyReply(text), lang); err == nil {
					s.ttsCache.put(k, mp3)
					fresh++
				}
			}
			cancel()
			if ctx.Err() != nil {
				return
			}
		}
	}
	log.Printf("tts prewarm: %d fixed lines, %d synthesised, %d already cached, %s", n, fresh, n-fresh, time.Since(start).Round(time.Millisecond))
}

// listCache returns the sizes of the cached files (for the admin status).
func listCache(dir string) []int64 {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []int64
	for _, e := range ents {
		if filepath.Ext(e.Name()) == ".mp3" {
			if i, err := e.Info(); err == nil {
				out = append(out, i.Size())
			}
		}
	}
	return out
}
