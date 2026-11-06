package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type part struct {
	name, typ, sub string
	off, size      int64
}

// stockTable is the flash layout of the reference unit (spec-clean/hardware_map.md).
var stockTable = []part{
	{"fctry", "data", "nvs", 0x009000, 0x006000},
	{"phy_init", "data", "phy", 0x00f000, 0x001000},
	{"otadata", "data", "ota", 0x010000, 0x002000},
	{"ota_0", "app", "ota_0", 0x020000, 0x500000},
	{"ota_1", "app", "ota_1", 0x520000, 0x500000},
	{"storage", "data", "spiffs", 0xa20000, 0x100000},
	{"nvs", "data", "nvs", 0xb20000, 0x040000},
}

func parsePartitions(text string) ([]part, error) {
	var out []part
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 5 {
			return nil, fmt.Errorf("malformed row: %q", line)
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		off, err1 := strconv.ParseInt(f[3], 0, 64)
		size, err2 := strconv.ParseInt(f[4], 0, 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("bad number in row: %q", line)
		}
		out = append(out, part{f[0], f[1], f[2], off, size})
	}
	return out, nil
}

func checkPartitions(rows []part) []string {
	var problems []string
	for i := 0; i < len(rows) || i < len(stockTable); i++ {
		var a, b *part
		if i < len(rows) {
			a = &rows[i]
		}
		if i < len(stockTable) {
			b = &stockTable[i]
		}
		if a == nil || b == nil || *a != *b {
			problems = append(problems, fmt.Sprintf("row %d: have %v want %v", i, a, b))
		}
	}
	end := int64(0x9000)
	for _, r := range rows {
		if r.off < end || r.off+r.size > 0x1000000 {
			problems = append(problems, "overlap or out of range at "+r.name)
		}
		end = r.off + r.size
	}
	return problems
}

func runPartitions(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: emotool partitions partitions.csv")
		return 2
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	rows, err := parsePartitions(string(b))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if p := checkPartitions(rows); len(p) > 0 {
		fmt.Fprintln(os.Stderr, "partition table differs from the stock layout:")
		for _, x := range p {
			fmt.Fprintln(os.Stderr, "  "+x)
		}
		return 1
	}
	fmt.Println("partitions.csv matches the stock layout")
	return 0
}
