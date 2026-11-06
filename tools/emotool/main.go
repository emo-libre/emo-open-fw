// Command emotool bundles the repository's helper tools (all Go, standard library only):
//
//	emotool summarize <capture-dir>          summarise a capture written by emoserver
//	emotool motcheck <file.mot>...           check your OWN .mot files against the 20-byte frame layout
//	emotool denylist-scan [-list f] [paths]  fail if any denylisted secret appears in files (default: the repository)
//	emotool denylist-gen <out>               write a salted-hash denylist from tokens on stdin
//	emotool partitions <partitions.csv>      fail unless the table matches the stock layout
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, "usage: emotool summarize|motcheck|denylist-scan|denylist-gen|partitions ...\n")
		os.Exit(2)
	}
	var code int
	args := os.Args[2:]
	switch os.Args[1] {
	case "summarize":
		code = runSummarize(args)
	case "motcheck":
		code = runMotCheck(args)
	case "denylist-scan":
		code = runDenylistScan(args)
	case "denylist-gen":
		code = runDenylistGen(args)
	case "partitions":
		code = runPartitions(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		code = 2
	}
	os.Exit(code)
}
