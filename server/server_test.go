package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, withK210 bool) (*server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	fw := filepath.Join(dir, "fw")
	if err := os.MkdirAll(fw, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fw, "image.bin"), []byte("ESP-IMAGE-BYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fw, "secret.txt"), []byte("do not serve"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.FirmwareDir = fw
	cfg.ESP32File = "image.bin"
	cfg.PublicHost = "192.0.2.10:8443"
	cfg.VersionNum = 1234
	cfg.VersionName = "open-test"
	if withK210 {
		if err := os.WriteFile(filepath.Join(fw, "k.bin"), []byte("K210"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg.K210File = "k.bin"
	}
	key, err := loadOrCreateKey(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	return s, s.routes(), dir
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decode(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, rr.Body.String())
	}
	return m
}

func TestTimeShapeAndOffsetKey(t *testing.T) {
	_, h, _ := newTestServer(t, false)
	rr := get(h, "/time?tz=Australia/Eucla")
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	m := decode(t, rr)
	if _, bad := m["Offset"]; bad {
		t.Fatal("upstream bug present: key serialised as Offset")
	}
	if m["offset"].(float64) != 31500 { // UTC+8:45, no DST
		t.Fatalf("offset = %v", m["offset"])
	}
	if d := time.Now().Unix() - int64(m["time"].(float64)); d < -2 || d > 2 {
		t.Fatalf("time off by %d", d)
	}
	if decode(t, get(h, "/time?tz=Not/AZone"))["offset"] == nil {
		t.Fatal("unknown zone should fall back, not fail")
	}
}

func TestTokenIssuedLocally(t *testing.T) {
	s, h, _ := newTestServer(t, false)
	rr := get(h, "/token/aabbccddeeff?version=42&version_name=3.2.0")
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	m := decode(t, rr)
	if m["type"] != "Bearer" || m["expire_in"].(float64) != tokenLifetime {
		t.Fatalf("%v", m)
	}
	claims, err := verifyJWT(s.key, m["access_token"].(string), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "aabbccddeeff" || claims["version"] != "42" || claims["name"] != "3.2.0" {
		t.Fatalf("claims %v", claims)
	}
	for _, k := range []string{"exp", "nbf", "iat"} {
		if claims[k] == nil {
			t.Fatalf("missing claim %s", k)
		}
	}
	// tampering and expiry are rejected
	tok := m["access_token"].(string)
	if _, err := verifyJWT(s.key, tok[:len(tok)-2]+"xx", time.Now()); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := verifyJWT(s.key, tok, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := verifyJWT([]byte("another key another key another!"), tok, time.Now()); err == nil {
		t.Fatal("token verified with the wrong key")
	}
	for _, bad := range []string{"/token/AABBCCDDEEFF", "/token/zz", "/token/aabbccddeeff00"} {
		if rr := get(h, bad); rr.Code != 400 {
			t.Fatalf("%s -> %d", bad, rr.Code)
		}
	}
}

func TestOTAVersionAndManifest(t *testing.T) {
	_, h, _ := newTestServer(t, false)
	v := decode(t, get(h, "/emo/ota/version?type=1&version_num=42"))
	if v["version-num"].(float64) != 1234 || v["version-name"] != "open-test" {
		t.Fatalf("%v", v)
	}
	for _, p := range []string{"/emo/ota/res/42", "/emo/ota/allres/42"} {
		rr := get(h, p)
		if rr.Code != 200 {
			t.Fatal(p, rr.Code)
		}
		m := decode(t, rr)
		u := m["updates"].(map[string]any)
		if u["host"] != "192.0.2.10:8443" || u["esp32"] != "firmware/esp32/image.bin" {
			t.Fatalf("%v", u)
		}
		if _, has := u["k210"]; has {
			t.Fatal("k210 key present without a configured image")
		}
		if len(u["esp_sha256"].(string)) != 64 {
			t.Fatal("esp_sha256")
		}
		for _, g := range []string{"mp3", "avi", "mot", "json"} {
			if u["sd"].(map[string]any)[g] == nil {
				t.Fatalf("sd group %s missing", g)
			}
		}
		if u["model"] == nil {
			t.Fatal("model missing")
		}
	}
	if rr := get(h, "/emo/ota/res/abc"); rr.Code != 400 {
		t.Fatal(rr.Code)
	}
}

func TestManifestWithK210AndMissingFiles(t *testing.T) {
	s, h, _ := newTestServer(t, true)
	u := decode(t, get(h, "/emo/ota/res/1"))["updates"].(map[string]any)
	if u["k210"] != "firmware/k210/k.bin" {
		t.Fatalf("%v", u)
	}
	os.Remove(filepath.Join(s.cfg.FirmwareDir, "k.bin"))
	if rr := get(h, "/emo/ota/res/1"); rr.Code != 503 {
		t.Fatalf("missing k210 image -> %d", rr.Code)
	}
	os.Remove(filepath.Join(s.cfg.FirmwareDir, "image.bin"))
	if rr := get(h, "/emo/ota/res/1"); rr.Code != 503 {
		t.Fatalf("missing esp image -> %d", rr.Code)
	}
}

func TestFirmwareServingIsRestricted(t *testing.T) {
	_, h, _ := newTestServer(t, false)
	if rr := get(h, "/firmware/esp32/image.bin"); rr.Code != 200 || rr.Body.String() != "ESP-IMAGE-BYTES" {
		t.Fatal(rr.Code, rr.Body.String())
	}
	if rr := get(h, "//firmware/esp32/image.bin"); rr.Code != 200 { // double slash from URL joining
		t.Fatalf("double slash -> %d", rr.Code)
	}
	for _, p := range []string{"/firmware/esp32/secret.txt", "/firmware/esp32/../secret.txt", "/firmware/esp32/%2e%2e%2fsecret.txt",
		"/firmware/k210/image.bin", "/firmware/k210/", "/firmware/"} {
		if rr := get(h, p); rr.Code == 200 {
			t.Fatalf("%s served: %q", p, rr.Body.String())
		}
	}
	// ranged request works (resumable download)
	req := httptest.NewRequest("GET", "/firmware/esp32/image.bin", nil)
	req.Header.Set("Range", "bytes=0-2")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 206 || rr.Body.String() != "ESP" {
		t.Fatal(rr.Code, rr.Body.String())
	}
}

func TestLocalStubs(t *testing.T) {
	_, h, _ := newTestServer(t, false)
	if m := decode(t, get(h, "/emo/notice/latest")); m["code"].(float64) != 200 {
		t.Fatal(m)
	}
	if m := decode(t, get(h, "/emo/permission")); m["type"] != "normal" {
		t.Fatal(m)
	}
	if m := decode(t, get(h, "/emo/server/alt")); m["switch"] != "" {
		t.Fatal(m)
	}
	req := httptest.NewRequest("POST", "/emo/report/info", strings.NewReader(`{"batt":3900}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || decode(t, rr)["result"] != "OK" {
		t.Fatal(rr.Code)
	}
	for _, p := range []string{"/emo/voice/unknown", "/emo/speech/other", "/emo/ai/draw/imgtoimg", "/emo/dialogue/general"} {
		if rr := get(h, p); rr.Code != 501 {
			t.Fatalf("%s -> %d", p, rr.Code)
		}
	}
	if rr := get(h, "/nonsense"); rr.Code != 404 {
		t.Fatal(rr.Code)
	}
	if rr := get(h, "/home/anything"); rr.Code != 404 { // upstream forwarded this to the vendor
		t.Fatal(rr.Code)
	}
}

func TestLogsNeverContainCredentialsOrBodies(t *testing.T) {
	_, h, _ := newTestServer(t, false)
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	req := httptest.NewRequest("POST", "/emo/report/info?city=Springfield&lat=1.5", strings.NewReader(`{"city":"SecretVille"}`))
	req.Header.Set("Authorization", "Bearer SUPERSECRETTOKEN")
	req.Header.Set("Secret", "SIGNATUREVALUE")
	h.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if out == "" {
		t.Fatal("nothing logged")
	}
	for _, leak := range []string{"SUPERSECRETTOKEN", "SIGNATUREVALUE", "SecretVille", "Springfield", "lat=1.5"} {
		if strings.Contains(out, leak) {
			t.Fatalf("log leaked %q: %s", leak, out)
		}
	}
}

func TestSelfSignedHTTPS(t *testing.T) {
	dir := t.TempDir()
	cert, err := ensureCert(dir, "192.0.2.10:8443")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	foundIP := false
	for _, ip := range leaf.IPAddresses {
		foundIP = foundIP || ip.String() == "192.0.2.10"
	}
	if !foundIP {
		t.Fatal("public host missing from SAN")
	}
	for _, n := range leaf.DNSNames {
		if strings.Contains(n, "living") {
			t.Fatalf("certificate names a vendor host: %s", n)
		}
	}
	if st, err := os.Stat(filepath.Join(dir, "key.pem")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key.pem mode: %v %v", st, err)
	}
	if _, err := ensureCert(dir, "ignored"); err != nil { // second call reuses the stored pair
		t.Fatal(err)
	}

	// a TLS 1.2-only client (like the robot) can fetch a response
	s, _, _ := newTestServer(t, false)
	ts := httptest.NewUnstartedServer(s.routes())
	ts.TLS = tlsConfig(cert, false)
	ts.StartTLS()
	defer ts.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}}}
	resp, err := client.Get(ts.URL + "/time")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.TLS.Version != tls.VersionTLS12 || !strings.Contains(string(b), `"time"`) {
		t.Fatalf("tls %x body %s", resp.TLS.Version, b)
	}
}

func TestConfigValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "c.json")
		os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	if _, err := LoadConfig(write(`{"version_num": 5, "esp32_file": "a.bin"}`)); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"unknown field": `{"vendor_host": "x"}`,
		"path in name":  `{"esp32_file": "../a.bin"}`,
		"no listeners":  `{"listen": "", "listen_http": ""}`,
		"zero version":  `{"version_num": 0}`,
	} {
		if _, err := LoadConfig(write(body)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

// The server must never talk to anything. This guards against re-introducing the upstream forwarding.
func TestNoOutboundCallsInSource(t *testing.T) {
	forbidden := []string{"http.NewRequest", "http.Get(", "http.Post(", "http.PostForm(", "http.Head(", "http.DefaultClient",
		"net.Dial", "net.DialTimeout", "tls.Dial", "httputil.NewSingleHostReverseProxy", "ReverseProxy", ".living.ai", "InsecureSkipVerify"}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if f == "dns.go" {
			// The DNS helper's one outbound packet: a UDP question to the resolver the owner configured (dns_upstream), never to a name from the robot.
			if n := bytes.Count(b, []byte(`net.DialTimeout("udp", d.upstream,`)); n != 1 || bytes.Count(b, []byte("net.Dial")) != 1 {
				t.Errorf("dns.go: expected exactly one dial, to d.upstream; found %d", n)
			}
			b = bytes.ReplaceAll(b, []byte(`net.DialTimeout("udp", d.upstream,`), nil)
		}
		for _, bad := range forbidden {
			if bytes.Contains(b, []byte(bad)) {
				t.Errorf("%s contains %q", f, bad)
			}
		}
	}
	// and the process-wide transport really is the denying one in main()
	if _, err := (denyTransport{}).RoundTrip(httptest.NewRequest("GET", "http://example.invalid/", nil)); err == nil {
		t.Fatal("denyTransport allowed a request")
	}
}
