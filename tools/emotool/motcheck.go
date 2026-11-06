package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
)

const motFrame = 20

// checkMot validates data against the 20-byte frame layout (spec-clean/mot_format.md) and returns problems and sub-type counts.
func checkMot(data []byte) (frames int, subs map[byte]int, problems []string) {
	subs = map[byte]int{}
	frames = len(data) / motFrame
	if len(data) == 0 || len(data)%motFrame != 0 {
		problems = append(problems, fmt.Sprintf("size %d is not a non-zero multiple of %d", len(data), motFrame))
	}
	for i := 0; i < frames; i++ {
		f := data[i*motFrame : (i+1)*motFrame]
		var sum byte
		for _, b := range f[4:18] {
			sum += b
		}
		switch {
		case !bytes.Equal(f[:4], []byte{0x55, 0xAA, 0x55, 0xAA}):
			problems = append(problems, fmt.Sprintf("frame %d: bad magic", i))
		case f[18] != sum:
			problems = append(problems, fmt.Sprintf("frame %d: bad checksum", i))
		case f[19] != 0xED:
			problems = append(problems, fmt.Sprintf("frame %d: bad terminator", i))
		default:
			subs[f[5]]++
		}
	}
	return
}

// runMotCheck reads files only and prints structure statistics, never content. It exists so the owner can settle the .mot layout
// on real data without any vendor file entering the repository.
func runMotCheck(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: emotool motcheck FILE.mot...")
		return 2
	}
	bad := false
	for _, p := range args {
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		frames, subs, problems := checkMot(data)
		keys := make([]int, 0, len(subs))
		for k := range subs {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		kinds := ""
		for i, k := range keys {
			if i > 0 {
				kinds += ", "
			}
			kinds += fmt.Sprintf("0x%02Xx%d", k, subs[byte(k)])
		}
		if kinds == "" {
			kinds = "-"
		}
		state := "OK"
		if len(problems) > 0 {
			state, bad = "INVALID", true
		}
		fmt.Printf("%s: %s  frames=%d  sub-types: %s\n", p, state, frames, kinds)
		for i, pr := range problems {
			if i == 5 {
				break
			}
			fmt.Println("    " + pr)
		}
	}
	if bad {
		return 1
	}
	return 0
}
