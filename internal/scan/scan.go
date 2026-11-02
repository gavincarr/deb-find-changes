// Package scan walks filesystem trees and classifies files as orphaned
// (owned by no package) or modified (differing from the packaged checksum).
package scan

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gavincarr/deb-find-changes/internal/exclude"
	"github.com/gavincarr/deb-find-changes/internal/index"
)

// Kind is the classification of a listed file.
type Kind byte

const (
	Modified Kind = 'M'
	Orphan   Kind = 'O'
)

// Entry is one file to report; Path is the logical (un-rooted) path.
type Entry struct {
	Path string
	Kind Kind
}

// Options configures Scan.
type Options struct {
	Root        string // prefixes every filesystem access; "" normally
	Trees       []string
	Modified    bool
	Orphans     bool
	Exclude     *exclude.Matcher
	ExcludePkgs map[string]bool
	Index       *index.Index
	Log         *slog.Logger
}

type scanner struct {
	log *slog.Logger
}

// Scan walks o.Trees and returns the entries to report, byte-sorted by Path
// and deduplicated.
func Scan(o Options) ([]Entry, error) {
	s := &scanner{log: o.Log}
	if s.log == nil {
		s.log = slog.Default()
	}
	seen := map[string]bool{}
	var out []Entry
	checked := 0
	for _, tree := range o.Trees {
		root := o.Root + tree
		err := walk(root, func(real string, d fs.DirEntry, err error) error {
			logical := strings.TrimPrefix(real, o.Root)
			if err != nil {
				s.log.Error("cannot read directory", "path", logical, "err", err)
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if o.Exclude != nil && o.Exclude.MatchDir(logical) {
					return fs.SkipDir
				}
				return nil
			}
			if seen[logical] {
				return nil
			}
			if o.Exclude != nil && o.Exclude.Match(logical) {
				return nil
			}
			isLink := d.Type()&fs.ModeSymlink != 0
			if isLink {
				st, err := os.Stat(real)
				if err != nil || !st.Mode().IsRegular() {
					s.log.Debug("skipping non-regular symlink", "path", logical)
					return nil
				}
			} else if !d.Type().IsRegular() {
				return nil
			}
			checked++
			var claims []index.Claim
			all := o.Index.Lookup(logical)
			for _, c := range all {
				if !o.ExcludePkgs[c.Package] {
					claims = append(claims, c)
				}
			}
			if len(all) > 0 && len(claims) == 0 {
				return nil
			}
			kind, list := s.classify(logical, real, isLink, claims)
			if !list || (kind == Modified && !o.Modified) || (kind == Orphan && !o.Orphans) {
				return nil
			}
			seen[logical] = true
			out = append(out, Entry{Path: logical, Kind: kind})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var nm, no int
	for _, e := range out {
		if e.Kind == Modified {
			nm++
		} else {
			no++
		}
	}
	s.log.Info("scan complete", "files", checked, "modified", nm, "orphans", no)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func walk(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

// classify decides whether a candidate file is listed, and as what. claims
// are the owning claims after package exclusion.
func (s *scanner) classify(logical, real string, isSymlink bool, claims []index.Claim) (Kind, bool) {
	if len(claims) == 0 {
		return Orphan, true
	}
	var hashed []index.Claim
	for _, c := range claims {
		if c.HasHash() {
			hashed = append(hashed, c)
		}
	}
	if len(hashed) == 0 {
		if isSymlink {
			return 0, false
		}
		s.log.Warn("no checksum recorded", "path", logical)
		return Modified, true
	}
	f, err := os.Open(real)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.log.Debug("file vanished", "path", logical)
			return 0, false
		}
		s.log.Warn("cannot read file", "path", logical, "err", err)
		return Modified, true
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		s.log.Warn("cannot read file", "path", logical, "err", err)
		return Modified, true
	}
	got := hex.EncodeToString(h.Sum(nil))
	for _, c := range hashed {
		if !strings.EqualFold(got, c.Hash) {
			s.log.Debug("checksum mismatch", "path", logical, "got", got, "expected", c.Hash, "package", c.Package)
			return Modified, true
		}
	}
	return 0, false
}
