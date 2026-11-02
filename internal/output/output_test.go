package output

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gavincarr/deb-find-changes/internal/scan"
)

var entries = []scan.Entry{
	{Path: "/etc/a", Kind: scan.Modified},
	{Path: "/etc/b", Kind: scan.Orphan},
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)) }

func TestWritePlain(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, entries, false, nopLogger()); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), "/etc/a\n/etc/b\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteTyped(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, entries, true, nopLogger()); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), "M /etc/a\nO /etc/b\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteSkipsNewline(t *testing.T) {
	var logs bytes.Buffer
	l := slog.New(slog.NewTextHandler(&logs, nil))
	es := []scan.Entry{{Path: "/etc/a\nb", Kind: scan.Orphan}, {Path: "/etc/c", Kind: scan.Orphan}}
	var b bytes.Buffer
	if err := Write(&b, es, false, l); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "/etc/c\n" {
		t.Fatalf("got %q", got)
	}
	if n := strings.Count(logs.String(), "level=WARN"); n != 1 {
		t.Fatalf("want 1 warn, got %d: %s", n, logs.String())
	}
}

func tempLeftovers(t *testing.T, dir string, want int) {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(des) != want {
		t.Fatalf("dir has %d entries, want %d: %v", len(des), want, des)
	}
}

func TestWriteFileAtomicMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out")
	err := WriteFileAtomic(p, 0o600, func(w io.Writer) error {
		_, err := w.Write([]byte("hello\n"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if b, _ := os.ReadFile(p); string(b) != "hello\n" {
		t.Fatalf("content %q", b)
	}
	tempLeftovers(t, dir, 1)
}

func TestWriteFileAtomicFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out")
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err := WriteFileAtomic(p, 0o600, func(w io.Writer) error {
		w.Write([]byte("partial"))
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "old\n" {
		t.Fatalf("content %q", b)
	}
	tempLeftovers(t, dir, 1)
}

func TestWriteFileAtomicMissingDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "out")
	if err := WriteFileAtomic(p, 0o600, func(io.Writer) error { return nil }); err == nil {
		t.Fatal("want error")
	}
}
