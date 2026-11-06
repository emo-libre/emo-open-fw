package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The denylist holds salted SHA-256 hashes of secrets that must never appear in the repository or in build output. It is
// generated from tokens read on stdin, so the tokens are never stored. Tokens shorter than minTokenLen are refused: salted
// hashes of short, guessable strings could be brute-forced and would leak them.
const minTokenLen = 12

var tokenRe = regexp.MustCompile(`[A-Za-z0-9_.%\-]{6,}`)
var skipDirs = map[string]bool{".git": true, "build": true, "managed_components": true, "__pycache__": true, "node_modules": true}

type denylist struct {
	salt   string
	lens   []int
	hashes map[string]bool
}

func loadDenylist(path string) (*denylist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := &denylist{hashes: map[string]bool{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "salt="):
			d.salt = line[5:]
		case strings.HasPrefix(line, "lens="):
			for _, p := range strings.Split(line[5:], ",") {
				if n, err := strconv.Atoi(p); err == nil {
					d.lens = append(d.lens, n)
				}
			}
		case line != "":
			d.hashes[line] = true
		}
	}
	if d.salt == "" {
		return nil, fmt.Errorf("%s: no salt line", path)
	}
	return d, sc.Err()
}

// scanData reports the byte offsets of every denylisted token (never the value).
func (d *denylist) scanData(data []byte) []int {
	var offs []int
	for _, m := range tokenRe.FindAllIndex(data, -1) {
		tok := data[m[0]:m[1]]
		h := sha256.New()
		for _, n := range d.lens {
			for i := 0; i+n <= len(tok); i++ {
				h.Reset()
				h.Write([]byte(d.salt))
				h.Write(tok[i : i+n])
				if d.hashes[hex.EncodeToString(h.Sum(nil))] {
					offs = append(offs, m[0]+i)
				}
			}
		}
	}
	return offs
}

func runDenylistScan(args []string) int {
	fs_ := flag.NewFlagSet("denylist-scan", flag.ContinueOnError)
	list := fs_.String("list", "", "denylist file (default: tools/denylist.sha256 next to the repository root)")
	if err := fs_.Parse(args); err != nil {
		return 2
	}
	paths := fs_.Args()
	root := findRepoRoot()
	if *list == "" {
		*list = filepath.Join(root, "tools", "denylist.sha256")
	}
	if len(paths) == 0 {
		paths = []string{root}
	}
	d, err := loadDenylist(*list)
	if err != nil {
		fmt.Fprintln(os.Stderr, "denylist:", err)
		return 2
	}
	absList, _ := filepath.Abs(*list)
	hits, files := 0, 0
	for _, p := range paths {
		err := filepath.WalkDir(p, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() {
				if skipDirs[e.Name()] && path != p {
					return filepath.SkipDir
				}
				return nil
			}
			if abs, _ := filepath.Abs(path); abs == absList || !e.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files++
			for _, off := range d.scanData(data) {
				hits++
				fmt.Printf("DENYLIST HIT: %s (byte offset %d)\n", path, off)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	state := "CLEAN"
	if hits > 0 {
		state = fmt.Sprintf("%d HIT(S)", hits)
	}
	fmt.Printf("scanned %d file(s), %d denylisted value(s): %s\n", files, len(d.hashes), state)
	if hits > 0 {
		return 1
	}
	return 0
}

// findRepoRoot walks up from the working directory to the folder that holds tools/denylist.sha256.
func findRepoRoot() string {
	dir, _ := os.Getwd()
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "tools", "denylist.sha256")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func runDenylistGen(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: emotool denylist-gen OUTFILE   (tokens on stdin, one per line)")
		return 2
	}
	return genDenylist(os.Stdin, args[0], os.Stdout)
}

func genDenylist(in io.Reader, out string, log io.Writer) int {
	set := map[string]bool{}
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			set[t] = true
		}
	}
	var toks []string
	skipped := 0
	for t := range set {
		if len(t) >= minTokenLen {
			toks = append(toks, t)
		} else {
			skipped++
		}
	}
	sort.Strings(toks)
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	saltHex := hex.EncodeToString(salt)
	lensSet := map[int]bool{}
	var lines []string
	for _, t := range toks {
		lensSet[len(t)] = true
		h := sha256.Sum256([]byte(saltHex + t))
		lines = append(lines, hex.EncodeToString(h[:]))
	}
	var lens []string
	li := make([]int, 0, len(lensSet))
	for n := range lensSet {
		li = append(li, n)
	}
	sort.Ints(li)
	for _, n := range li {
		lens = append(lens, strconv.Itoa(n))
	}
	body := "salt=" + saltHex + "\nlens=" + strings.Join(lens, ",") + "\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Fprintf(log, "wrote %d hashes (lengths %v); skipped %d token(s) under %d chars\n", len(toks), li, skipped, minTokenLen)
	return 0
}
