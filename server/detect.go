package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoserver/backend"
)

// ---- what is installed on this machine ----

func (s *server) handleAdminDetect(w http.ResponseWriter, r *http.Request) {
	tools := map[string]string{}
	for _, t := range []string{"whisper-server", "espeak-ng", "lame", "piper", "ollama", "ffmpeg"} {
		if p, err := exec.LookPath(t); err == nil {
			tools[t] = p
		} else {
			tools[t] = ""
		}
	}
	home, _ := os.UserHomeDir()
	var whisperModels, piperVoices []string
	for _, d := range []string{filepath.Join(s.cfg.DataDir, "models"), "models", filepath.Join(home, "whisper.cpp", "models"),
		"/usr/share/whisper.cpp/models", "/usr/local/share/whisper.cpp/models", filepath.Join(home, "models")} {
		m, _ := filepath.Glob(filepath.Join(d, "ggml-*.bin"))
		for _, f := range m {
			if abs, err := filepath.Abs(f); err == nil {
				whisperModels = append(whisperModels, abs)
			}
		}
	}
	for _, d := range []string{filepath.Join(s.cfg.DataDir, "piper"), filepath.Join(home, "piper"), filepath.Join(home, ".local", "share", "piper")} {
		m, _ := filepath.Glob(filepath.Join(d, "*.onnx"))
		for _, f := range m {
			if abs, err := filepath.Abs(f); err == nil {
				piperVoices = append(piperVoices, abs)
			}
		}
	}
	sort.Strings(whisperModels)
	sort.Strings(piperVoices)
	if len(whisperModels) > 20 {
		whisperModels = whisperModels[:20]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	ollama := map[string]any{"running": false}
	if models, err := backend.ProbeOllama(ctx); err == nil {
		ollama = map[string]any{"running": true, "models": models}
	}
	engine := map[string]any{"configured": false}
	if wc, ok := s.be.STT.(*backend.WhisperCpp); ok {
		running, out := wc.Status()
		engine = map[string]any{"configured": true, "running": running, "last_output": out}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lan_ips": lanIPs(), "tools": tools, "whisper_models": nonNil(whisperModels),
		"piper_voices": nonNil(piperVoices), "ollama": ollama, "speech_engine": engine})
}

// ---- presets ----

//go:embed presets/*.json
var presetFS embed.FS

var presetInfo = []struct{ id, title, about string }{
	{"local", "Everything on my own computer", "whisper.cpp for listening, Ollama for chat, espeak-ng for the voice. Only weather and news use the internet."},
	{"free-online", "Free online services", "Groq for listening and chat, Google for the voice, OpenWeatherMap for weather. Needs free accounts and keys."},
	{"anthropic", "Claude for chat", "Anthropic's Claude for chat and photo description (paid), Groq for listening, espeak-ng for the voice."},
}

func (s *server) handleAdminPresets(w http.ResponseWriter, r *http.Request) {
	list := []map[string]string{}
	for _, p := range presetInfo {
		list = append(list, map[string]string{"id": p.id, "title": p.title, "about": p.about})
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": list})
}

func (s *server) handleAdminPreset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, p := range presetInfo {
		if p.id == id {
			b, err := presetFS.ReadFile("presets/" + id + ".json")
			if err != nil {
				break
			}
			var doc struct {
				Backends json.RawMessage `json:"backends"`
			}
			if json.Unmarshal(b, &doc) != nil {
				break
			}
			writeJSON(w, http.StatusOK, map[string]any{"backends": doc.Backends})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "errmessage": "no such preset"})
}

// ---- a readable report of what the robot has asked for (capture mode) ----

type capLine struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Bytes  int    `json:"req_body_bytes"`
	Time   string `json:"time"`
}

func (s *server) handleAdminCapture(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Capture.Dir == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	f, err := os.Open(filepath.Join(s.cfg.Capture.Dir, "capture.jsonl"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "total": 0, "routes": []any{}})
		return
	}
	defer f.Close()
	type agg struct {
		Method, Path string
		Count        int
		Statuses     map[int]int
		BiggestBody  int
		Last         string
	}
	byKey := map[string]*agg{}
	total := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l capLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		total++
		path := l.Path
		if strings.HasPrefix(path, "/token/") {
			path = "/token/<robot id>" // never list a robot's id here
		}
		k := l.Method + " " + path
		a := byKey[k]
		if a == nil {
			a = &agg{Method: l.Method, Path: path, Statuses: map[int]int{}}
			byKey[k] = a
		}
		a.Count++
		a.Statuses[l.Status]++
		if l.Bytes > a.BiggestBody {
			a.BiggestBody = l.Bytes
		}
		a.Last = l.Time
	}
	rows := make([]map[string]any, 0, len(byKey))
	for _, a := range byKey {
		unknown := false
		if a.Statuses[404]+a.Statuses[501] > 0 {
			unknown = true
		}
		rows = append(rows, map[string]any{"method": a.Method, "path": a.Path, "count": a.Count, "statuses": a.Statuses,
			"biggest_body_bytes": a.BiggestBody, "last": a.Last, "not_served": unknown})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["count"].(int) > rows[j]["count"].(int) || (rows[i]["count"].(int) == rows[j]["count"].(int) && rows[i]["path"].(string) < rows[j]["path"].(string))
	})
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "total": total, "dir": s.cfg.Capture.Dir, "bodies": s.cfg.Capture.Bodies, "routes": rows})
}
