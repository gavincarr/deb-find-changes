// Command deb-find-changes lists orphaned and modified files for backup.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/gavincarr/deb-find-changes/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args and returns the process exit code. Kong exits are
// trapped: code 0 (e.g. --help) stays 0, anything else becomes 2.
func run(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			c, ok := r.(exitCode)
			if !ok {
				panic(r)
			}
			if c == 0 {
				code = 0
			} else {
				code = 2
			}
		}
	}()

	var cli CLI
	parser, err := newParser(&cli, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "deb-find-changes:", err)
		return 2
	}
	if _, err := parser.Parse(args); err != nil {
		parser.Stdout = stderr // usage goes to stderr on errors
		parser.FatalIfErrorf(err)
		return 2
	}
	if cli.Version {
		fmt.Fprintf(stdout, "deb-find-changes %s\n", version.Version)
		return 0
	}
	if len(cli.Paths) == 0 {
		parser.Stdout = stderr
		parser.FatalIfErrorf(fmt.Errorf("expected at least one <path>"))
		return 2
	}
	fmt.Fprintln(stderr, "deb-find-changes: not implemented")
	return 1
}
