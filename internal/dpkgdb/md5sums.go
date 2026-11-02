package dpkgdb

import (
	"io"
	"strings"
)

// ParseMD5Sums parses an info/*.md5sums file into a map keyed by "/" + path,
// with lowercased hash values.
func ParseMD5Sums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	s := newScanner(r)
	for s.Scan() {
		line := s.Text()
		i := strings.IndexByte(line, ' ')
		if i <= 0 {
			continue
		}
		path := strings.TrimLeft(line[i:], " ")
		if path == "" {
			continue
		}
		out["/"+path] = strings.ToLower(line[:i])
	}
	return out, s.Err()
}
