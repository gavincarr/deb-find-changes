package dpkgdb

import (
	"fmt"
	"io"
)

// Diversion is one dpkg-divert record. Package is "" for local diversions.
type Diversion struct{ From, To, Package string }

// ParseDiversions parses the three-lines-per-record diversions file.
func ParseDiversions(r io.Reader) ([]Diversion, error) {
	var out []Diversion
	var buf []string
	s := newScanner(r)
	for s.Scan() {
		buf = append(buf, s.Text())
		if len(buf) == 3 {
			d := Diversion{From: buf[0], To: buf[1], Package: buf[2]}
			if d.Package == ":" {
				d.Package = ""
			}
			out = append(out, d)
			buf = buf[:0]
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(buf) != 0 {
		return nil, fmt.Errorf("diversions: truncated trailing record (%d of 3 lines)", len(buf))
	}
	return out, nil
}
