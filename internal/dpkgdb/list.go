// Package dpkgdb parses dpkg's on-disk state under /var/lib/dpkg.
package dpkgdb

import (
	"bufio"
	"io"
	"strings"
)

// maxToken is the scanner's maximum line length; status files have long lines.
const maxToken = 1 << 20

func newScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), maxToken)
	return s
}

// ParseList parses a dpkg info/*.list file, returning non-empty lines as-is.
func ParseList(r io.Reader) ([]string, error) {
	var out []string
	s := newScanner(r)
	for s.Scan() {
		if line := s.Text(); line != "" {
			out = append(out, line)
		}
	}
	return out, s.Err()
}

// PackageFromInfoName extracts the package name from an info file name such
// as "bind9-libs:amd64.list" or "bash.md5sums".
func PackageFromInfoName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	return name
}
