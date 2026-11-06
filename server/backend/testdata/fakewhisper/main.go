// A stand-in for whisper.cpp's whisper-server, used only by the tests. It speaks just enough of /inference.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	model := flag.String("m", "", "")
	host := flag.String("host", "127.0.0.1", "")
	port := flag.Int("port", 0, "")
	flag.Int("t", 0, "")
	flag.Parse()
	name := filepath.Base(*model)
	if strings.Contains(name, "crash") {
		fmt.Fprintln(os.Stderr, "boom: cannot load model")
		os.Exit(3)
	}
	if strings.Contains(name, "slow") {
		time.Sleep(1200 * time.Millisecond)
	}
	http.HandleFunc("/inference", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			http.Error(w, "bad", 400)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file", 400)
			return
		}
		n := 0
		buf := make([]byte, 4096)
		for {
			k, err := f.Read(buf)
			n += k
			if err != nil {
				break
			}
		}
		text := fmt.Sprintf(" heard %d bytes in %s\n", n, r.FormValue("language"))
		if strings.Contains(name, "blank") {
			text = " [BLANK_AUDIO]\n"
		}
		if strings.Contains(name, "echo") { // shows what hint the client sent
			text = fmt.Sprintf(" prompt=%x\n", r.FormValue("prompt")) // hex, so the client's phantom guard does not see the hint itself
		}
		if strings.Contains(name, "phantom") { // what a real model writes for silence
			text = " you\n"
		}
		if strings.Contains(name, "parrot") { // a real engine sometimes repeats its hint on silent audio
			text = " " + r.FormValue("prompt") + "\n"
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
