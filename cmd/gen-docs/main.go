// Command gen-docs embeds the example scripts into docs/README.md between
// gen-docs markers, so the documented code blocks stay in sync with the real,
// type-checked files under docs/examples. It is a build-time tool, not part of
// the server.
//
// Usage:
//
//	gen-docs [-check] [file...]
//
// With no files it defaults to docs/README.md (relative to the working
// directory, so run it from the repo root). With -check it writes nothing and
// exits non-zero when a file is out of date, for CI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/homeland-live/homeland/cmd/gen-docs/snippets"
)

func main() {
	check := flag.Bool("check", false, "verify files are up to date without writing; exit 1 if not")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		files = []string{"docs/README.md"}
	}

	stale := false
	for _, f := range files {
		run := snippets.Generate
		if *check {
			run = snippets.Check
		}
		changed, err := run(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		switch {
		case *check && changed:
			fmt.Fprintf(os.Stderr, "%s is out of date; run `make docs`\n", f)
			stale = true
		case changed:
			fmt.Printf("wrote %s\n", f)
		}
	}
	if stale {
		os.Exit(1)
	}
}
