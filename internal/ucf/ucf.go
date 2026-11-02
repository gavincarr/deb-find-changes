// Package ucf parses ucf's state files under /var/lib/ucf.
package ucf

import (
	"bufio"
	"io"
	"strings"
)

func newScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return s
}

// ParseHashfile parses ucf's hashfile ("<md5>  <path>") into path -> lowercased md5.
func ParseHashfile(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	s := newScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		hash, path, ok := strings.Cut(line, " ")
		path = strings.TrimSpace(path)
		if !ok || hash == "" || path == "" {
			continue
		}
		out[path] = strings.ToLower(hash)
	}
	return out, s.Err()
}

// ParseRegistry parses ucf's registry ("<package> <ws> <path>") into path -> package.
func ParseRegistry(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	s := newScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		i := strings.IndexAny(line, " \t")
		if i <= 0 {
			continue
		}
		path := strings.TrimSpace(line[i:])
		if path == "" {
			continue
		}
		out[path] = line[:i]
	}
	return out, s.Err()
}
