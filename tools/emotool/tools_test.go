package main

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const synthetic = "Synth3tic-Token_9xQ7zK" // not a real secret

func genList(t *testing.T, tokens string) (string, string) {
	t.Helper()
	list := filepath.Join(t.TempDir(), "list")
	var log bytes.Buffer
	if code := genDenylist(strings.NewReader(tokens), list, &log); code != 0 {
		t.Fatal("gen failed")
	}
	return list, log.String()
}

func TestDenylistGenAndScan(t *testing.T) {
	list, log := genList(t, synthetic+"\nshort\n")
	if !strings.Contains(log, "skipped 1") {
		t.Fatal(log)
	}
	raw, _ := os.ReadFile(list)
	if strings.Contains(string(raw), synthetic) {
		t.Fatal("the token leaked into the list")
	}
	d, err := loadDenylist(list)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.scanData([]byte("nothing to see here, just words and numbers 12345"))) != 0 {
		t.Fatal("clean text flagged")
	}
	if len(d.scanData([]byte(`key = "`+synthetic+`";`))) != 1 {
		t.Fatal("plain hit missed")
	}
	if len(d.scanData([]byte("prefix_"+synthetic+"_suffix"))) != 1 {
		t.Fatal("embedded token missed")
	}
	bin := append(append(make([]byte, 64), []byte(synthetic)...), 0)
	rand.Read(bin[:64])
	if len(d.scanData(bin)) != 1 {
		t.Fatal("token inside binary data missed")
	}
	if len(d.scanData([]byte("Synth3tic-Token_9xQ7zX"))) != 0 { // one character off
		t.Fatal("near miss flagged")
	}
}

func TestDenylistScanCommandNeverEchoesValues(t *testing.T) {
	list, _ := genList(t, synthetic)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello "+synthetic), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "ignored.txt"), []byte(synthetic), 0o644)
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	code := runDenylistScan([]string{"-list", list, dir})
	w.Close()
	os.Stdout = old
	var out bytes.Buffer
	out.ReadFrom(r)
	if code != 1 || strings.Contains(out.String(), synthetic) || !strings.Contains(out.String(), "1 HIT") {
		t.Fatalf("code %d output %q", code, out.String())
	}
	if strings.Contains(out.String(), ".git") {
		t.Fatal(".git was scanned")
	}
}

func TestPartitions(t *testing.T) {
	csv := "# c\nfctry, data, nvs, 0x9000, 0x6000,\nphy_init, data, phy, 0xf000, 0x1000,\notadata, data, ota, 0x10000, 0x2000,\n" +
		"ota_0, app, ota_0, 0x20000, 0x500000,\nota_1, app, ota_1, 0x520000, 0x500000,\nstorage, data, spiffs, 0xa20000, 0x100000,\nnvs, data, nvs, 0xb20000, 0x40000,\n"
	rows, err := parsePartitions(csv)
	if err != nil || len(checkPartitions(rows)) != 0 {
		t.Fatalf("%v %v", err, checkPartitions(rows))
	}
	bad, _ := parsePartitions(strings.Replace(csv, "0x500000,\nota_1", "0x400000,\nota_1", 1))
	if len(checkPartitions(bad)) == 0 {
		t.Fatal("a resized slot was accepted")
	}
	short, _ := parsePartitions(strings.Join(strings.Split(csv, "\n")[:6], "\n"))
	if len(checkPartitions(short)) == 0 {
		t.Fatal("a missing partition was accepted")
	}
	if _, err := parsePartitions("a, b\n"); err == nil {
		t.Fatal("malformed row accepted")
	}
	// the project's own table
	b, err := os.ReadFile("../../partitions.csv")
	if err != nil {
		t.Fatal(err)
	}
	own, err := parsePartitions(string(b))
	if err != nil || len(checkPartitions(own)) != 0 {
		t.Fatalf("partitions.csv: %v %v", err, checkPartitions(own))
	}
}

func frame(sub byte, a [4]byte, dur uint16) []byte {
	f := []byte{0x55, 0xAA, 0x55, 0xAA, 2, sub, 0, 0, 0, 0, 0, a[0], a[1], a[2], a[3], 0, byte(dur), byte(dur >> 8), 0, 0xED}
	var s byte
	for _, b := range f[4:18] {
		s += b
	}
	f[18] = s
	return f
}

func TestMotCheck(t *testing.T) {
	good := append(frame(0xFE, [4]byte{90, 90, 90, 90}, 100), frame(1, [4]byte{1, 2, 3, 4}, 0)...)
	n, subs, problems := checkMot(good)
	if n != 2 || len(problems) != 0 || subs[0xFE] != 1 || subs[1] != 1 {
		t.Fatalf("%d %v %v", n, subs, problems)
	}
	bad := append([]byte(nil), good...)
	bad[20+12] ^= 1
	if _, _, p := checkMot(bad); len(p) != 1 || !strings.Contains(p[0], "frame 1: bad checksum") {
		t.Fatalf("%v", p)
	}
	bad = append([]byte(nil), good...)
	bad[19] = 0
	if _, _, p := checkMot(bad); len(p) != 1 || !strings.Contains(p[0], "terminator") {
		t.Fatalf("%v", p)
	}
	bad = append([]byte(nil), good...)
	bad[1] = 0
	if _, _, p := checkMot(bad); len(p) != 1 || !strings.Contains(p[0], "magic") {
		t.Fatalf("%v", p)
	}
	if _, _, p := checkMot(good[:39]); len(p) == 0 {
		t.Fatal("truncated file accepted")
	}
	if _, _, p := checkMot(nil); len(p) == 0 {
		t.Fatal("empty file accepted")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "g.mot"), good, 0o644)
	os.WriteFile(filepath.Join(dir, "b.mot"), bad, 0o644)
	if runMotCheck([]string{filepath.Join(dir, "g.mot")}) != 0 || runMotCheck([]string{filepath.Join(dir, "g.mot"), filepath.Join(dir, "b.mot")}) != 1 ||
		runMotCheck(nil) != 2 || runMotCheck([]string{filepath.Join(dir, "missing.mot")}) != 2 {
		t.Fatal("exit codes")
	}
}

func TestSummarize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "capture.jsonl"), []byte(
		`{"method":"GET","path":"/time","query":"tz=UTC","headers":{"Accept":"*/*"},"status":200}`+"\n"+
			`{"method":"POST","path":"/emo/voice/detectintent","query":"index=1&role=chatgpt","headers":{"Content-Type":"application/octet-stream"},"chunked":true,"req_body_bytes":9000,"status":200}`+"\n"+
			`{"method":"GET","path":"/home/x","query":"","headers":{},"status":404}`+"\n"), 0o644)
	recs, err := readCapture(dir)
	if err != nil || len(recs) != 3 {
		t.Fatalf("%v %d", err, len(recs))
	}
	f, _ := os.CreateTemp(t.TempDir(), "out")
	summarize(f, recs)
	f.Seek(0, 0)
	var b bytes.Buffer
	b.ReadFrom(f)
	out := b.String()
	for _, want := range []string{"3 requests, 3 distinct endpoints", "query keys: index, role", "chunked: true", "body bytes: min 9000 / max 9000", "UNHANDLED", "GET /home/x"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if runSummarize(nil) != 2 || runSummarize([]string{filepath.Join(dir, "nope")}) != 2 {
		t.Fatal("exit codes")
	}
}
