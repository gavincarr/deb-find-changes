// Package index builds a path -> claims ownership index from dpkg and ucf state.
package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gavincarr/deb-find-changes/internal/dpkgdb"
	"github.com/gavincarr/deb-find-changes/internal/ucf"
)

// Claim records that Package owns a path; Hash is "" when nothing recorded.
type Claim struct{ Package, Hash string }

// HasHash reports whether the claim carries a real md5 (32 hex chars).
func (c Claim) HasHash() bool {
	if len(c.Hash) != 32 {
		return false
	}
	for _, r := range c.Hash {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// Index maps paths to the claims made on them.
type Index struct{ m map[string][]Claim }

// New returns an empty Index.
func New() *Index { return &Index{m: map[string][]Claim{}} }

// Add records a claim, deduping by Package and preferring a claim with a hash.
func (ix *Index) Add(path string, c Claim) {
	cs := ix.m[path]
	for i, e := range cs {
		if e.Package == c.Package {
			if !e.HasHash() && c.HasHash() {
				cs[i] = c
			}
			return
		}
	}
	ix.m[path] = append(cs, c)
}

// Paths returns the number of distinct paths with at least one claim.
func (ix *Index) Paths() int { return len(ix.m) }

// Packages returns the number of distinct non-empty package names.
func (ix *Index) Packages() int {
	pkgs := map[string]bool{}
	for _, cs := range ix.m {
		for _, c := range cs {
			if c.Package != "" {
				pkgs[c.Package] = true
			}
		}
	}
	return len(pkgs)
}

// Lookup returns the claims on path.
func (ix *Index) Lookup(path string) []Claim { return ix.m[path] }

func openOpt(path string) (*os.File, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return f, err
}

// Load builds the index from the dpkg admin dir and ucf dir. The returned
// strings are warnings for the caller to log.
func Load(admindir, ucfdir string) (*Index, []string, error) {
	ix := New()
	var warns []string

	entries, err := os.ReadDir(filepath.Join(admindir, "info"))
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".list") {
			continue
		}
		pkg := dpkgdb.PackageFromInfoName(name)
		paths, err := readList(filepath.Join(admindir, "info", name))
		if err != nil {
			return nil, nil, err
		}
		md5s, err := readMD5(filepath.Join(admindir, "info", strings.TrimSuffix(name, ".list")+".md5sums"))
		if err != nil {
			return nil, nil, err
		}
		for _, p := range paths {
			ix.Add(p, Claim{pkg, md5s[p]})
		}
	}

	sf, err := os.Open(filepath.Join(admindir, "status"))
	if err != nil {
		return nil, nil, err
	}
	pkgs, err := dpkgdb.ParseStatus(sf)
	sf.Close()
	if err != nil {
		return nil, nil, err
	}
	for _, p := range pkgs {
		for _, cf := range p.Conffiles {
			ix.Add(cf.Path, Claim{p.Name, cf.Hash})
		}
	}

	if df, err := openOpt(filepath.Join(admindir, "diversions")); err != nil {
		return nil, nil, err
	} else if df != nil {
		divs, err := dpkgdb.ParseDiversions(df)
		df.Close()
		if err != nil {
			return nil, nil, err
		}
		for _, d := range divs {
			var keep, move []Claim
			for _, c := range ix.m[d.From] {
				if c.Package != d.Package {
					move = append(move, c)
				} else {
					keep = append(keep, c)
				}
			}
			if len(keep) == 0 {
				delete(ix.m, d.From)
			} else {
				ix.m[d.From] = keep
			}
			for _, c := range move {
				ix.Add(d.To, c)
			}
		}
	}

	var reg, hashes map[string]string
	if f, err := openOpt(filepath.Join(ucfdir, "registry")); err != nil {
		return nil, nil, err
	} else if f != nil {
		reg, err = ucf.ParseRegistry(f)
		f.Close()
		if err != nil {
			return nil, nil, err
		}
	}
	if f, err := openOpt(filepath.Join(ucfdir, "hashfile")); err != nil {
		return nil, nil, err
	} else if f != nil {
		hashes, err = ucf.ParseHashfile(f)
		f.Close()
		if err != nil {
			return nil, nil, err
		}
	}
	for p, pkg := range reg {
		ix.Add(p, Claim{pkg, hashes[p]})
	}
	for p, h := range hashes {
		if _, ok := reg[p]; !ok {
			ix.Add(p, Claim{"", h})
		}
	}

	if ups, err := os.ReadDir(filepath.Join(admindir, "updates")); err == nil {
		for _, u := range ups {
			if allDigits(u.Name()) {
				warns = append(warns, "dpkg updates journal not empty (interrupted dpkg run?); ownership data may be stale")
				break
			}
		}
	}
	return ix, warns, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func readList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	return dpkgdb.ParseList(f)
}

func readMD5(path string) (map[string]string, error) {
	f, err := openOpt(path)
	if err != nil || f == nil {
		return nil, err
	}
	defer f.Close()
	return dpkgdb.ParseMD5Sums(f)
}
