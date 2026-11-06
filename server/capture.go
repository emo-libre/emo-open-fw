package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CaptureConfig turns on a local request recorder so the owner can see exactly what their own robot sends and
// compare it with spec-clean/cloud_api.md. It is OFF by default. Everything stays in dir (mode 0700/0600).
// Authorization and Secret header VALUES are never recorded (only that they were present and their length).
// Request bodies (voice recordings, photos) are saved only when "bodies" is true.
type CaptureConfig struct {
	Dir    string `json:"dir"`
	Bodies bool   `json:"bodies"`
}

var plainHeaders = map[string]bool{
	"Content-Type": true, "Content-Length": true, "Transfer-Encoding": true, "User-Agent": true,
	"Host": true, "Connection": true, "Keep-Alive": true, "Accept": true,
}

type captureWriter struct {
	http.ResponseWriter
	status int
	n      int
	head   bytes.Buffer
}

func (c *captureWriter) WriteHeader(code int) { c.status = code; c.ResponseWriter.WriteHeader(code) }
func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if c.head.Len() < 1024 {
		room := 1024 - c.head.Len()
		if len(b) < room {
			room = len(b)
		}
		c.head.Write(b[:room])
	}
	c.n += len(b)
	return c.ResponseWriter.Write(b)
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func reqBytes(body []byte, c *countingReader) int {
	if c != nil {
		return c.n
	}
	return len(body)
}

func capturing(cfg CaptureConfig, next http.Handler) (http.Handler, error) {
	if cfg.Dir == "" {
		return next, nil
	}
	if err := os.MkdirAll(filepath.Join(cfg.Dir, "bodies"), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(cfg.Dir, "capture.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	seq := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seq++
		n := seq
		mu.Unlock()

		var body []byte
		var counter *countingReader
		if r.Body != nil {
			if cfg.Bodies {
				body, _ = io.ReadAll(io.LimitReader(r.Body, 8<<20))
				r.Body = io.NopCloser(bytes.NewReader(body))
			} else {
				counter = &countingReader{r: r.Body}
				r.Body = io.NopCloser(counter) // count the size without keeping the content
			}
		}
		cw := &captureWriter{ResponseWriter: w}
		t0 := time.Now()
		next.ServeHTTP(cw, r)
		took := time.Since(t0)

		hdr := map[string]string{}
		for k, v := range r.Header {
			ck := http.CanonicalHeaderKey(k)
			if plainHeaders[ck] {
				hdr[ck] = strings.Join(v, ",")
			} else {
				hdr[ck] = fmt.Sprintf("<%d chars>", len(strings.Join(v, ",")))
			}
		}
		rec := map[string]any{
			"n": n, "time": time.Now().UTC().Format(time.RFC3339), "method": r.Method, "path": r.URL.Path,
			"query": r.URL.RawQuery, "proto": r.Proto, "headers": hdr, "chunked": len(r.TransferEncoding) > 0,
			"req_body_bytes": reqBytes(body, counter), "status": cw.status, "resp_bytes": cw.n,
			"resp_type": cw.Header().Get("Content-Type"), "ms": took.Milliseconds(), // how long the robot waited for this answer
		}
		if strings.Contains(cw.Header().Get("Content-Type"), "json") {
			rec["resp_preview"] = cw.head.String()
		}
		line, _ := json.Marshal(rec)
		mu.Lock()
		_, _ = f.Write(append(line, '\n'))
		mu.Unlock()
		if cfg.Bodies && len(body) > 0 {
			name := fmt.Sprintf("%06d_%s.bin", n, strings.ReplaceAll(strings.Trim(r.URL.Path, "/"), "/", "_"))
			_ = os.WriteFile(filepath.Join(cfg.Dir, "bodies", name), body, 0o600)
		}
	}), nil
}
