package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type capRec struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   string            `json:"query"`
	Headers map[string]string `json:"headers"`
	Chunked bool              `json:"chunked"`
	Bytes   int               `json:"req_body_bytes"`
	Status  int               `json:"status"`
}

func readCapture(dir string) ([]capRec, error) {
	f, err := os.Open(filepath.Join(dir, "capture.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []capRec
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var r capRec
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

func runSummarize(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: emotool summarize <capture-dir>")
		return 2
	}
	recs, err := readCapture(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return summarize(os.Stdout, recs)
}

func summarize(w *os.File, recs []capRec) int {
	type key struct{ m, p string }
	by := map[key][]capRec{}
	for _, r := range recs {
		k := key{r.Method, r.Path}
		by[k] = append(by[k], r)
	}
	keys := make([]key, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].p != keys[b].p {
			return keys[a].p < keys[b].p
		}
		return keys[a].m < keys[b].m
	})
	fmt.Fprintf(w, "%d requests, %d distinct endpoints\n\n", len(recs), len(by))
	var unhandled []string
	for _, k := range keys {
		rs := by[k]
		qk, ct, hn := map[string]bool{}, map[string]bool{}, map[string]bool{}
		status := map[int]int{}
		chunked, lo, hi := false, -1, 0
		for _, r := range rs {
			for _, kv := range strings.Split(r.Query, "&") {
				if kv != "" {
					qk[strings.SplitN(kv, "=", 2)[0]] = true
				}
			}
			c := r.Headers["Content-Type"]
			if c == "" {
				c = "-"
			}
			ct[c] = true
			for h := range r.Headers {
				hn[h] = true
			}
			status[r.Status]++
			chunked = chunked || r.Chunked
			if lo < 0 || r.Bytes < lo {
				lo = r.Bytes
			}
			if r.Bytes > hi {
				hi = r.Bytes
			}
		}
		fmt.Fprintf(w, "%-5s %s  x%d  status=%v\n", k.m, k.p, len(rs), status)
		fmt.Fprintf(w, "        query keys: %s   content-type: %s   chunked: %v\n", joinKeys(qk), joinKeys(ct), chunked)
		if hi > 0 {
			fmt.Fprintf(w, "        body bytes: min %d / max %d\n", lo, hi)
		}
		fmt.Fprintf(w, "        header names: %s\n\n", joinKeys(hn))
		if status[404] > 0 || status[501] > 0 {
			unhandled = append(unhandled, fmt.Sprintf("  %s %s  x%d", k.m, k.p, len(rs)))
		}
	}
	if len(unhandled) > 0 {
		fmt.Fprintln(w, "UNHANDLED (our server answered 404/501) - pin these formats:")
		for _, u := range unhandled {
			fmt.Fprintln(w, u)
		}
	} else {
		fmt.Fprintln(w, "Every captured request was answered by a real handler.")
	}
	return 0
}

func joinKeys(m map[string]bool) string {
	if len(m) == 0 {
		return "-"
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}
