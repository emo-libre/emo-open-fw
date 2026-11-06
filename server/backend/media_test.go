package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSDWebUIMapsRobotParameters(t *testing.T) {
	png, _ := MockImage{}.Generate(context.Background(), ImageRequest{Prompt: "x"})
	var gotPath string
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		got = nil
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{"images": []string{base64.StdEncoding.EncodeToString(png)}})
	}))
	defer srv.Close()
	g, err := NewSDWebUI(SDWebUIConfig{BaseURL: srv.URL, NegativePrompt: "blurry", Width: 256, Height: 256})
	if err != nil {
		t.Fatal(err)
	}
	out, err := g.Generate(context.Background(), ImageRequest{Prompt: "a girl", Steps: 20, CFG: 11, Sampler: "Euler a"})
	if err != nil || !isImage(out) {
		t.Fatal(err)
	}
	if gotPath != "/sdapi/v1/txt2img" || got["prompt"] != "a girl" || got["steps"].(float64) != 20 || got["cfg_scale"].(float64) != 11 ||
		got["sampler_name"] != "Euler a" || got["negative_prompt"] != "blurry" || got["width"].(float64) != 256 {
		t.Fatalf("%s %v", gotPath, got)
	}
	if _, has := got["init_images"]; has {
		t.Fatal("txt2img sent an init image")
	}
	if _, err := g.Generate(context.Background(), ImageRequest{Prompt: "a girl", Steps: 20, CFG: 11, Denoise: 0.2, Init: []byte("\xff\xd8\xff")}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/sdapi/v1/img2img" || got["denoising_strength"].(float64) != 0.2 || len(got["init_images"].([]any)) != 1 {
		t.Fatalf("%s %v", gotPath, got)
	}
	if _, err := NewSDWebUI(SDWebUIConfig{BaseURL: "https://api.living.ai"}); err == nil {
		t.Fatal("vendor URL accepted")
	}
}

func TestSDWebUIRejectsBadReplies(t *testing.T) {
	for name, body := range map[string]string{"empty": `{"images":[]}`, "not base64": `{"images":["@@@"]}`,
		"not an image": `{"images":["` + base64.StdEncoding.EncodeToString([]byte("hello")) + `"]}`, "junk": `nope`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		g, _ := NewSDWebUI(SDWebUIConfig{BaseURL: srv.URL})
		if _, err := g.Generate(context.Background(), ImageRequest{Prompt: "x", Steps: 1, CFG: 1}); err == nil {
			t.Errorf("%s accepted", name)
		}
		srv.Close()
	}
}

func TestOpenAIImagesAndCommandImage(t *testing.T) {
	png, _ := MockImage{}.Generate(context.Background(), ImageRequest{Prompt: "x"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path != "/v1/images/generations" || req["response_format"] != "b64_json" || req["size"] != "256x256" {
			http.Error(w, "bad", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(png)}}})
	}))
	defer srv.Close()
	g, _ := NewOpenAIImages(OpenAIConfig{BaseURL: srv.URL + "/v1", Model: "m", Size: "256x256"})
	if b, err := g.Generate(context.Background(), ImageRequest{Prompt: "a cat"}); err != nil || !isImage(b) {
		t.Fatal(err)
	}
	if _, err := g.Generate(context.Background(), ImageRequest{Prompt: "a cat", Init: []byte{1}}); err != ErrUnsupported {
		t.Fatalf("img2img: %v", err)
	}
	// command: prompt on stdin, init file path available, output must be an image
	cmd := NewCommandImage(CommandConfig{Args: []string{"sh", "-c", "cat >/dev/null; test -z \"$2\" || test -s \"$2\"; printf '\\211PNG\\r\\n\\032\\n' > \"$1\"", "sh", "{out}", "{init}"}})
	if _, err := cmd.Generate(context.Background(), ImageRequest{Prompt: "x"}); err != nil {
		t.Fatal(err)
	}
	bad := NewCommandImage(CommandConfig{Args: []string{"sh", "-c", "echo hi > \"$1\"", "sh", "{out}"}})
	if _, err := bad.Generate(context.Background(), ImageRequest{Prompt: "x"}); err == nil {
		t.Fatal("non-image output accepted")
	}
}

func TestParseFeedsAndHeadlines(t *testing.T) {
	rss := `<?xml version="1.0"?><rss><channel><title>t</title>
	  <item><title>First &amp; <b>big</b> story</title></item>
	  <item><title><![CDATA[Second <i>story</i>]]></title></item><item><title>Third</title></item></channel></rss>`
	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>Atom one</title></entry><entry><title>Third</title></entry></feed>`
	got, err := parseFeed([]byte(rss))
	if err != nil || len(got) != 3 || got[0] != "First & big story" || got[1] != "Second story" {
		t.Fatalf("%v %v", got, err)
	}
	if g, _ := parseFeed([]byte(atom)); len(g) != 2 || g[0] != "Atom one" {
		t.Fatalf("%v", g)
	}
	if _, err := parseFeed([]byte("not xml at all <<<")); err == nil {
		t.Log("lenient parse of junk is fine as long as it yields nothing")
	}
	if cleanTitle(strings.Repeat("é", 400)) == "" || len([]rune(cleanTitle(strings.Repeat("é", 400)))) != 160 {
		t.Fatal("title length bound")
	}

	var hits int
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; io.WriteString(w, rss) }))
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, atom) }))
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer a.Close()
	defer b.Close()
	defer dead.Close()
	n, err := NewRSS(RSSConfig{Feeds: []string{a.URL, dead.URL, b.URL}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := n.Headlines(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	// interleaved, duplicate "Third" dropped, a dead feed skipped
	want := []string{"First & big story", "Atom one", "Second story", "Third"}
	if strings.Join(h, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", h)
	}
	before := hits
	n.Headlines(context.Background(), 2)
	if hits != before {
		t.Fatal("feed not cached")
	}
	n.now = func() time.Time { return time.Now().Add(time.Hour) }
	n.Headlines(context.Background(), 2)
	if hits == before {
		t.Fatal("cache never expires")
	}
	if _, err := NewRSS(RSSConfig{}); err == nil {
		t.Fatal("no feeds accepted")
	}
	if _, err := NewRSS(RSSConfig{Feeds: []string{"https://news.living.ai/rss"}}); err == nil {
		t.Fatal("vendor feed accepted")
	}
	alldead, _ := NewRSS(RSSConfig{Feeds: []string{dead.URL}})
	if _, err := alldead.Headlines(context.Background(), 3); err == nil {
		t.Fatal("no headlines should be an error")
	}
}

func TestMockImageIsValidPNG(t *testing.T) {
	b, err := MockImage{}.Generate(context.Background(), ImageRequest{Prompt: "hello", Width: 32, Height: 24})
	if err != nil || http.DetectContentType(b) != "image/png" {
		t.Fatal(err)
	}
}
