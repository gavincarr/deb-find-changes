package exclude

import (
	"bufio"
	"io"
	"strings"
)

// ParseFile reads an exclude file. Blank lines and '#' comments are
// skipped, "@name" lines are returned as package names, and everything
// else as patterns tagged with source and line number.
func ParseFile(r io.Reader, source string) (pats []Pattern, pkgs []string, err error) {
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "@"):
			pkgs = append(pkgs, line[1:])
		default:
			pats = append(pats, Pattern{Glob: line, Source: source, Line: n})
		}
	}
	return pats, pkgs, sc.Err()
}
