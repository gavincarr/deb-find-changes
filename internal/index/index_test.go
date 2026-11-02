package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	h1 = "11111111111111111111111111111111"
	h2 = "22222222222222222222222222222222"
	h3 = "33333333333333333333333333333333"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture returns admin and ucf dirs with an empty status and info dir.
func fixture(t *testing.T) (admin, ucf string) {
	root := t.TempDir()
	admin = filepath.Join(root, "dpkg")
	ucf = filepath.Join(root, "ucf")
	writeFile(t, admin, "status", "")
	if err := os.MkdirAll(filepath.Join(admin, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

func load(t *testing.T, admin, ucf string) (*Index, []string) {
	t.Helper()
	ix, w, err := Load(admin, ucf)
	if err != nil {
		t.Fatal(err)
	}
	return ix, w
}

func TestLoadListAndMD5(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "info/a.list", "/etc\n/etc/a.conf\n/usr/bin/a\n")
	writeFile(t, admin, "info/a.md5sums", h1+"  etc/a.conf\n"+h2+"  usr/bin/a\n")
	ix, _ := load(t, admin, ucf)
	want := []Claim{{"a", h1}}
	if got := ix.Lookup("/etc/a.conf"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLoadMultiArchName(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "info/b:amd64.list", "/usr/lib/b\n")
	writeFile(t, admin, "info/b:amd64.md5sums", h1+"  usr/lib/b\n")
	ix, _ := load(t, admin, ucf)
	want := []Claim{{"b", h1}}
	if got := ix.Lookup("/usr/lib/b"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLoadConffiles(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "status", "Package: c\nStatus: deinstall ok config-files\nArchitecture: all\nVersion: 1\nConffiles:\n /etc/c.conf "+h1+"\n /etc/c2.conf newconffile\n\n")
	ix, _ := load(t, admin, ucf)
	if got := ix.Lookup("/etc/c.conf"); !reflect.DeepEqual(got, []Claim{{"c", h1}}) {
		t.Fatalf("got %v", got)
	}
	got := ix.Lookup("/etc/c2.conf")
	if len(got) != 1 || got[0].Package != "c" || got[0].HasHash() {
		t.Fatalf("got %v", got)
	}
}

func TestLoadConffileHashReplacesHashless(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "info/c.list", "/etc/c.conf\n")
	writeFile(t, admin, "status", "Package: c\nStatus: install ok installed\nArchitecture: all\nVersion: 1\nConffiles:\n /etc/c.conf "+h1+"\n\n")
	ix, _ := load(t, admin, ucf)
	if got := ix.Lookup("/etc/c.conf"); !reflect.DeepEqual(got, []Claim{{"c", h1}}) {
		t.Fatalf("got %v", got)
	}
}

func TestLoadDiversion(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "info/orig.list", "/etc/x\n")
	writeFile(t, admin, "info/orig.md5sums", h1+"  etc/x\n")
	writeFile(t, admin, "info/div.list", "/etc/x\n")
	writeFile(t, admin, "info/div.md5sums", h2+"  etc/x\n")
	writeFile(t, admin, "diversions", "/etc/x\n/etc/x.distrib\ndiv\n")
	ix, _ := load(t, admin, ucf)
	if got := ix.Lookup("/etc/x"); !reflect.DeepEqual(got, []Claim{{"div", h2}}) {
		t.Fatalf("From: %v", got)
	}
	if got := ix.Lookup("/etc/x.distrib"); !reflect.DeepEqual(got, []Claim{{"orig", h1}}) {
		t.Fatalf("To: %v", got)
	}
}

func TestLoadLocalDiversion(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "info/orig.list", "/etc/x\n")
	writeFile(t, admin, "info/orig.md5sums", h1+"  etc/x\n")
	writeFile(t, admin, "diversions", "/etc/x\n/etc/x.local\n:\n")
	ix, _ := load(t, admin, ucf)
	if got := ix.Lookup("/etc/x"); len(got) != 0 {
		t.Fatalf("From: %v", got)
	}
	if got := ix.Lookup("/etc/x.local"); !reflect.DeepEqual(got, []Claim{{"orig", h1}}) {
		t.Fatalf("To: %v", got)
	}
}

func TestLoadUCF(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, ucf, "registry", "rsyslog /etc/r.conf\nrsyslog /etc/nohash.conf\n")
	writeFile(t, ucf, "hashfile", h3+" /etc/r.conf\n")
	ix, _ := load(t, admin, ucf)
	if got := ix.Lookup("/etc/r.conf"); !reflect.DeepEqual(got, []Claim{{"rsyslog", h3}}) {
		t.Fatalf("got %v", got)
	}
	got := ix.Lookup("/etc/nohash.conf")
	if len(got) != 1 || got[0].HasHash() {
		t.Fatalf("got %v", got)
	}
}

func TestLoadUpdatesWarning(t *testing.T) {
	admin, ucf := fixture(t)
	writeFile(t, admin, "updates/tmp.i", "")
	if _, w := load(t, admin, ucf); len(w) != 0 {
		t.Fatalf("unexpected %v", w)
	}
	writeFile(t, admin, "updates/0001", "")
	if _, w := load(t, admin, ucf); len(w) != 1 {
		t.Fatalf("want one warning, got %v", w)
	}
}

func TestLoadMissingStatus(t *testing.T) {
	admin, ucf := fixture(t)
	os.Remove(filepath.Join(admin, "status"))
	if _, _, err := Load(admin, ucf); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadMissingInfo(t *testing.T) {
	admin, ucf := fixture(t)
	os.Remove(filepath.Join(admin, "info"))
	if _, _, err := Load(admin, ucf); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadMissingUCFDir(t *testing.T) {
	admin, ucf := fixture(t)
	_, w := load(t, admin, filepath.Join(ucf, "nope"))
	if len(w) != 0 {
		t.Fatalf("unexpected %v", w)
	}
}

func TestHasHash(t *testing.T) {
	if (Claim{Hash: "newconffile"}).HasHash() || (Claim{}).HasHash() || !(Claim{Hash: h1}).HasHash() {
		t.Fatal("HasHash wrong")
	}
}

func TestLoadRealSystem(t *testing.T) {
	if _, err := os.Stat("/var/lib/dpkg/status"); err != nil {
		t.Skip("no dpkg database")
	}
	ix, _, err := Load("/var/lib/dpkg", "/var/lib/ucf")
	if err != nil {
		t.Skip(err)
	}
	for _, c := range ix.Lookup("/etc/bash.bashrc") {
		if c.Package == "bash" && c.HasHash() {
			return
		}
	}
	t.Skip("bash claim for /etc/bash.bashrc not found (not Debian-like?)")
}
