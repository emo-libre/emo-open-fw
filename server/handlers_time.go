package main

import (
	"encoding/json"
	"net/http"
	"time"
)

type timeReply struct {
	Time   int64 `json:"time"`
	Offset int   `json:"offset"` // upstream's struct tag was misspelt ("jason"), serialising this as "Offset"
}

// handleTime answers GET /time?tz=<IANA zone>. Unknown or missing zone falls back to the server's local zone.
func handleTime(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	loc := time.Local
	if tz := r.URL.Query().Get("tz"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	_, off := now.In(loc).Zone()
	writeJSON(w, http.StatusOK, timeReply{Time: now.Unix(), Offset: off})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
