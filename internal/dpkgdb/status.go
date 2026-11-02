package dpkgdb

import (
	"io"
	"strings"
)

// Conffile is one entry from a status file's Conffiles field.
type Conffile struct {
	Path, Hash      string
	Obsolete        bool
	RemoveOnUpgrade bool
}

// Package is the subset of a status stanza we care about.
type Package struct {
	Name, Arch, Status string
	Conffiles          []Conffile
}

// ParseStatus parses a dpkg status file.
func ParseStatus(r io.Reader) ([]Package, error) {
	var pkgs []Package
	var cur Package
	var have bool
	field := "" // current multi-line field name

	flush := func() {
		if have {
			pkgs = append(pkgs, cur)
		}
		cur, have, field = Package{}, false, ""
	}

	s := newScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if field == "Conffiles" {
				if cf, ok := parseConffile(line); ok {
					cur.Conffiles = append(cur.Conffiles, cf)
				}
			}
			continue
		}
		name, val, ok := strings.Cut(line, ":")
		if !ok {
			field = ""
			continue
		}
		field = name
		val = strings.TrimSpace(val)
		have = true
		switch name {
		case "Package":
			cur.Name = val
		case "Architecture":
			cur.Arch = val
		case "Status":
			cur.Status = val
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	flush()
	return pkgs, nil
}

// parseConffile parses " <path> <hash> [obsolete] [remove-on-upgrade]",
// working from the right because the path may contain spaces.
func parseConffile(line string) (Conffile, bool) {
	toks := strings.Fields(line)
	var cf Conffile
	for len(toks) > 0 {
		switch toks[len(toks)-1] {
		case "obsolete":
			cf.Obsolete = true
		case "remove-on-upgrade":
			cf.RemoveOnUpgrade = true
		default:
			goto flags
		}
		toks = toks[:len(toks)-1]
	}
flags:
	if len(toks) < 2 {
		return cf, false
	}
	cf.Hash = toks[len(toks)-1]
	// Recover the path from the original line to preserve internal spacing.
	t := strings.TrimSpace(line)
	idx := strings.LastIndex(t, cf.Hash)
	cf.Path = strings.TrimSpace(t[:idx])
	return cf, cf.Path != ""
}
