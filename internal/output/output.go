// Package output writes the sorted file list, optionally with M/O type
// prefixes, and provides atomic file output.
package output

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gavincarr/deb-find-changes/internal/scan"
)

// Write writes one path per line (prefixed "M " or "O " when withType).
// Entries must already be sorted. Paths containing a newline cannot be
// represented in a line-based list and are skipped with a warning.
func Write(w io.Writer, entries []scan.Entry, withType bool, log *slog.Logger) error {
	bw := bufio.NewWriter(w)
	for _, e := range entries {
		if strings.ContainsAny(e.Path, "\n\r") {
			log.Warn("skipping path containing newline or carriage return", "path", e.Path)
			continue
		}
		if withType {
			bw.WriteByte(byte(e.Kind))
			bw.WriteByte(' ')
		}
		bw.WriteString(e.Path)
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// WriteFileAtomic writes path via a temp file in the same directory and an
// atomic rename. On any error the temp file is removed and path is untouched.
func WriteFileAtomic(path string, mode os.FileMode, fill func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	if err = fill(bw); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
