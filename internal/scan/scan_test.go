package scan

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gavincarr/deb-find-changes/internal/exclude"
	"github.com/gavincarr/deb-find-changes/internal/index"
)

func sum(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

func write(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func bufLog() (*slog.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})), &b
}

func opts(root string, ix *index.Index, trees ...string) Options {
	m, _ := exclude.Compile(nil)
	return Options{Root: root, Trees: trees, Modified: true, Orphans: true, Exclude: m, Index: ix, Log: discard()}
}

func asMap(es []Entry) map[string]Kind {
	m := map[string]Kind{}
	for _, e := range es {
		m[e.Path] = e.Kind
	}
	return m
}

func TestScanBasic(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/orphan", "o")
	write(t, root, "etc/same", "s")
	write(t, root, "etc/changed", "c")
	write(t, root, "etc/nohash", "n")
	ix := index.New()
	ix.Add("/etc/same", index.Claim{Package: "p", Hash: sum("s")})
	ix.Add("/etc/changed", index.Claim{Package: "p", Hash: sum("other")})
	ix.Add("/etc/nohash", index.Claim{Package: "p"})
	lg, buf := bufLog()
	o := opts(root, ix, "/etc")
	o.Log = lg
	es, err := Scan(o)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{"/etc/changed", Modified}, {"/etc/nohash", Modified}, {"/etc/orphan", Orphan}}
	if !reflect.DeepEqual(es, want) {
		t.Fatalf("got %v want %v", es, want)
	}
	if !strings.Contains(buf.String(), "no checksum recorded") || !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("missing warn: %s", buf)
	}
}

func TestScanKinds(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/orphan", "o")
	write(t, root, "etc/changed", "c")
	ix := index.New()
	ix.Add("/etc/changed", index.Claim{Package: "p", Hash: sum("x")})
	o := opts(root, ix, "/etc")
	o.Orphans = false
	es, _ := Scan(o)
	if !reflect.DeepEqual(es, []Entry{{"/etc/changed", Modified}}) {
		t.Errorf("modified only: %v", es)
	}
	o.Orphans, o.Modified = true, false
	es, _ = Scan(o)
	if !reflect.DeepEqual(es, []Entry{{"/etc/orphan", Orphan}}) {
		t.Errorf("orphans only: %v", es)
	}
}

func TestScanExcludes(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/x.bak", "1")
	write(t, root, "etc/alternatives/a", "1")
	write(t, root, "etc/kdeonly", "1")
	write(t, root, "etc/both", "1")
	write(t, root, "etc/keep", "1")
	ix := index.New()
	ix.Add("/etc/kdeonly", index.Claim{Package: "kde", Hash: sum("zzz")})
	ix.Add("/etc/both", index.Claim{Package: "kde", Hash: sum("zzz")})
	ix.Add("/etc/both", index.Claim{Package: "other", Hash: sum("1")})
	o := opts(root, ix, "/etc")
	o.Exclude, _ = exclude.Compile([]exclude.Pattern{{Glob: "*.bak"}, {Glob: "/etc/alternatives/"}})
	o.ExcludePkgs = map[string]bool{"kde": true}
	es, _ := Scan(o)
	if !reflect.DeepEqual(es, []Entry{{"/etc/keep", Orphan}}) {
		t.Errorf("got %v", es)
	}
}

func TestScanSymlinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/x.service", "svc")
	write(t, root, "etc/other", "different")
	write(t, root, "etc/systemd/system/x.service", "unit")
	dir := filepath.Join(root, "etc/systemd/system/multi-user.target.wants")
	os.MkdirAll(dir, 0o755)
	os.Symlink("../x.service", filepath.Join(dir, "x.service"))
	os.Symlink("x.service", filepath.Join(root, "etc/owned"))
	os.Symlink("other", filepath.Join(root, "etc/hashed"))
	ix := index.New()
	ix.Add("/etc/x.service", index.Claim{Package: "p", Hash: sum("svc")})
	ix.Add("/etc/other", index.Claim{Package: "p", Hash: sum("different")})
	ix.Add("/etc/owned", index.Claim{Package: "p"})
	ix.Add("/etc/systemd/system/x.service", index.Claim{Package: "p", Hash: sum("unit")})
	ix.Add("/etc/hashed", index.Claim{Package: "p", Hash: sum("expected")})
	es, _ := Scan(opts(root, ix, "/etc"))
	want := map[string]Kind{
		"/etc/systemd/system/multi-user.target.wants/x.service": Orphan,
		"/etc/hashed": Modified,
	}
	if got := asMap(es); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestScanSkipsNonRegular(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "etc")
	os.MkdirAll(filepath.Join(d, "dir"), 0o755)
	if err := syscall.Mkfifo(filepath.Join(d, "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Symlink("nowhere", filepath.Join(d, "dangling"))
	os.Symlink("dir", filepath.Join(d, "todir"))
	os.Symlink("loop2", filepath.Join(d, "loop1"))
	os.Symlink("loop1", filepath.Join(d, "loop2"))
	done := make(chan []Entry, 1)
	go func() {
		es, _ := Scan(opts(root, index.New(), "/etc"))
		done <- es
	}()
	select {
	case es := <-done:
		if len(es) != 0 {
			t.Errorf("expected nothing, got %v", es)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Scan hung")
	}
}

func TestScanUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	root := t.TempDir()
	write(t, root, "etc/secret", "s")
	os.Chmod(filepath.Join(root, "etc/secret"), 0)
	ix := index.New()
	ix.Add("/etc/secret", index.Claim{Package: "p", Hash: sum("s")})
	lg, buf := bufLog()
	o := opts(root, ix, "/etc")
	o.Log = lg
	es, _ := Scan(o)
	if !reflect.DeepEqual(es, []Entry{{"/etc/secret", Modified}}) {
		t.Errorf("got %v", es)
	}
	if !strings.Contains(buf.String(), "cannot read file") || !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("missing warn: %s", buf)
	}
}

func TestScanUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores dir modes")
	}
	root := t.TempDir()
	write(t, root, "etc/private/f", "x")
	write(t, root, "etc/sibling", "x")
	os.Chmod(filepath.Join(root, "etc/private"), 0)
	t.Cleanup(func() { os.Chmod(filepath.Join(root, "etc/private"), 0o755) })
	lg, buf := bufLog()
	o := opts(root, index.New(), "/etc")
	o.Log = lg
	es, err := Scan(o)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(es, []Entry{{"/etc/sibling", Orphan}}) {
		t.Errorf("got %v", es)
	}
	out := buf.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "/etc/private") {
		t.Errorf("missing error record: %s", out)
	}
}

func TestScanAwkwardNames(t *testing.T) {
	root := t.TempDir()
	names := []string{"a b", "100%", "x[1]", "\xff.conf"}
	for _, n := range names {
		write(t, root, "etc/"+n, "x")
	}
	es, _ := Scan(opts(root, index.New(), "/etc"))
	got := asMap(es)
	if len(got) != len(names) {
		t.Fatalf("got %q", es)
	}
	for _, n := range names {
		if got["/etc/"+n] != Orphan {
			t.Errorf("missing %q in %q", n, es)
		}
	}
}

func TestScanOverlappingTrees(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/b", "x")
	write(t, root, "etc/a", "x")
	write(t, root, "etc/ssh/c", "x")
	es, _ := Scan(opts(root, index.New(), "/etc", "/etc/ssh", "/etc"))
	want := []Entry{{"/etc/a", Orphan}, {"/etc/b", Orphan}, {"/etc/ssh/c", Orphan}}
	if !reflect.DeepEqual(es, want) {
		t.Errorf("got %v", es)
	}
}

func TestClassifyVanished(t *testing.T) {
	lg, _ := bufLog()
	s := &scanner{log: lg}
	_, list := s.classify("/etc/gone", filepath.Join(t.TempDir(), "gone"), false,
		[]index.Claim{{Package: "p", Hash: sum("x")}})
	if list {
		t.Error("vanished file must not be listed")
	}
}
