package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Content packs: the owner's OWN media files, offered to the robot's updater in the manifest and served from content_dir.
// Layout: content_dir/{mp3,avi,mot,json,model}/<files>. Nothing is bundled with this project. The robot downloads
// "/<group>/<name>" for every name listed (group arrays hold up to 2048 names; index "0" is the directory).

var contentGroups = []string{"mp3", "avi", "mot", "json"}

const maxGroupFiles = 2048

var safeFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// listFiles returns the regular files with safe names in dir, sorted, at most max.
func listFiles(dir string, max int) []os.FileInfo {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []os.FileInfo
	for _, e := range ents {
		if !safeFile.MatchString(e.Name()) {
			continue
		}
		if i, err := e.Info(); err == nil && i.Mode().IsRegular() {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name() < out[b].Name() })
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// contentManifest builds the "sd" and "model" parts of the update manifest. With no content_dir the groups are empty but present.
func (s *server) contentManifest() (map[string]map[string]string, map[string]any) {
	sd := map[string]map[string]string{}
	for _, g := range contentGroups {
		sd[g] = map[string]string{"0": "/" + g + "/"}
		if s.cfg.ContentDir == "" {
			continue
		}
		for i, f := range listFiles(filepath.Join(s.cfg.ContentDir, g), maxGroupFiles) {
			sd[g][strconv.Itoa(i+1)] = f.Name()
		}
	}
	model := map[string]any{"0": "/model/"}
	if s.cfg.ContentDir != "" {
		for i, f := range listFiles(filepath.Join(s.cfg.ContentDir, "model"), maxGroupFiles) {
			model[strconv.Itoa(i+1)] = map[string]any{"0": f.Name(), "1": f.Size()}
		}
	}
	return sd, model
}

func (s *server) serveContent(group string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if s.cfg.ContentDir == "" || !safeFile.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(s.cfg.ContentDir, group, name))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, name, time.Time{}, f)
	}
}
