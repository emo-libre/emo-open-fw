package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	voiceMaxBytes = 1 << 20 // far above the ~96 KB the robot sends
	pcmRate       = 16000
)

// decodeVoiceBody extracts the PCM from the voice upload. Wire layout (see spec-clean/cloud_api.md):
// 0x22 | PCM signed 16-bit little-endian mono 16 kHz | 0x22. The framing bytes are stripped when both are present.
func decodeVoiceBody(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, voiceMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > voiceMaxBytes {
		return nil, errors.New("voice body too large")
	}
	if len(b) >= 2 && b[0] == 0x22 && b[len(b)-1] == 0x22 {
		b = b[1 : len(b)-1]
	}
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	if len(b) == 0 {
		return nil, errors.New("empty audio")
	}
	return b, nil
}

// wavFromPCM wraps 16-bit mono PCM in a RIFF/WAVE header.
func wavFromPCM(pcm []byte, rate int) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcm)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcm)))
	return append(h, pcm...)
}

// ---- generated speech store: MP3s the robot downloads over plain HTTP ----

type audioItem struct {
	id    int
	data  []byte
	token string
	at    time.Time
}

type audioStore struct {
	mu    sync.Mutex
	items map[string]*audioItem // keyed by file name
	total int
	max   int
	ttl   time.Duration
	seq   int
	now   func() time.Time
}

func newAudioStore() *audioStore {
	return &audioStore{items: map[string]*audioItem{}, max: 64 << 20, ttl: time.Hour, now: time.Now}
}

func (a *audioStore) evict() {
	now := a.now()
	for k, it := range a.items {
		if now.Sub(it.at) > a.ttl {
			a.total -= len(it.data)
			delete(a.items, k)
		}
	}
	for a.total > a.max {
		var oldest string
		var ot time.Time
		for k, it := range a.items {
			if oldest == "" || it.at.Before(ot) {
				oldest, ot = k, it.at
			}
		}
		if oldest == "" {
			break
		}
		a.total -= len(a.items[oldest].data)
		delete(a.items, oldest)
	}
}

// Put stores mp3 and returns the path+query the robot should request (relative to the audio base URL).
func (a *audioStore) Put(mp3 []byte) string {
	tok := make([]byte, 32)
	_, _ = rand.Read(tok)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	name := fmt.Sprintf("%d_%d.mp3", a.now().Unix(), a.seq)
	it := &audioItem{id: a.seq, data: mp3, token: hex.EncodeToString(tok), at: a.now()}
	a.items[name] = it
	a.total += len(mp3)
	a.evict()
	return fmt.Sprintf("/mp3/download/%s?id=%d&token=%s", name, it.id, it.token)
}

func (a *audioStore) get(name, id, token string) ([]byte, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.evict()
	it, ok := a.items[name]
	if !ok || strconv.Itoa(it.id) != id || subtle.ConstantTimeCompare([]byte(it.token), []byte(token)) != 1 {
		return nil, false
	}
	return it.data, true
}

func (a *audioStore) handle(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !strings.HasSuffix(name, ".mp3") {
		http.NotFound(w, r)
		return
	}
	data, ok := a.get(name, r.URL.Query().Get("id"), r.URL.Query().Get("token"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// cleanPCM prepares a quiet, slightly offset recording for speech-to-text: it removes the DC offset and low rumble with a
// one-pole high-pass filter (about 80 Hz), then raises the level so the loudest sample reaches about 70% of full scale, never
// by more than 8x. Silence and already-loud recordings are left at their level. Input and output are 16-bit little-endian mono.
func cleanPCM(pcm []byte) []byte {
	n := len(pcm) / 2
	if n == 0 {
		return pcm
	}
	const alpha = 0.97 // fc = (1-alpha)*fs/(2*pi) ~ 76 Hz at 16 kHz
	out := make([]float64, n)
	prevIn, prevOut := float64(int16(binary.LittleEndian.Uint16(pcm))), 0.0 // start settled: no jump from the first sample's offset
	peak := 0.0
	for i := 0; i < n; i++ {
		x := float64(int16(binary.LittleEndian.Uint16(pcm[2*i:])))
		y := alpha * (prevOut + x - prevIn)
		prevIn, prevOut = x, y
		out[i] = y
		if a := math.Abs(y); a > peak {
			peak = a
		}
	}
	gain := 1.0
	if peak > 300 && peak < 0.7*32767 { // above the noise floor, below the target
		gain = math.Min(8, 0.7*32767/peak)
	}
	res := make([]byte, n*2)
	for i, y := range out {
		v := math.Round(y * gain)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(res[2*i:], uint16(int16(v)))
	}
	return res
}
