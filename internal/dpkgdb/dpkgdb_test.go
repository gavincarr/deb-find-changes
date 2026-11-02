package dpkgdb

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseListAndInfoName(t *testing.T) {
	got, err := ParseList(strings.NewReader("/.\n/etc/a\n\n/etc/b c\n"))
	if err != nil || !reflect.DeepEqual(got, []string{"/.", "/etc/a", "/etc/b c"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if PackageFromInfoName("libc6:amd64.md5sums") != "libc6" || PackageFromInfoName("bash.list") != "bash" {
		t.Fatal("PackageFromInfoName")
	}
	if PackageFromInfoName("bind9-libs:amd64.list") != "bind9-libs" {
		t.Fatal("PackageFromInfoName arch")
	}
}

func TestParseMD5Sums(t *testing.T) {
	got, err := ParseMD5Sums(strings.NewReader("ABCDEF0123456789abcdef0123456789  etc/my file.conf\n0123456789abcdef0123456789abcdef  etc/x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got["/etc/my file.conf"] != "abcdef0123456789abcdef0123456789" || got["/etc/x"] == "" || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

const statusFixture = `Package: foo
Status: install ok installed
Architecture: amd64
Description: a thing
 Conffiles: not really
 /etc/not-a-conffile deadbeef
Conffiles:
 /etc/foo.conf 0123456789abcdef0123456789abcdef
 /etc/foo bar.conf newconffile remove-on-upgrade
 /etc/old.conf 0123456789abcdef0123456789abcdef obsolete

Package: gone
Status: deinstall ok config-files
Architecture: all
Conffiles:
 /etc/gone.conf 0123456789abcdef0123456789abcdef

Package: plain
Status: install ok installed
Architecture: all
Description: nothing
 more text
`

func TestParseStatus(t *testing.T) {
	pkgs, err := ParseStatus(strings.NewReader(statusFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("len %d: %+v", len(pkgs), pkgs)
	}
	want := []Conffile{
		{Path: "/etc/foo.conf", Hash: "0123456789abcdef0123456789abcdef"},
		{Path: "/etc/foo bar.conf", Hash: "newconffile", RemoveOnUpgrade: true},
		{Path: "/etc/old.conf", Hash: "0123456789abcdef0123456789abcdef", Obsolete: true},
	}
	if pkgs[0].Name != "foo" || pkgs[0].Arch != "amd64" || pkgs[0].Status != "install ok installed" ||
		!reflect.DeepEqual(pkgs[0].Conffiles, want) {
		t.Fatalf("foo: %+v", pkgs[0])
	}
	if pkgs[1].Status != "deinstall ok config-files" || len(pkgs[1].Conffiles) != 1 {
		t.Fatalf("rc: %+v", pkgs[1])
	}
	if pkgs[2].Name != "plain" || len(pkgs[2].Conffiles) != 0 {
		t.Fatalf("plain: %+v", pkgs[2])
	}
}

func TestParseDiversions(t *testing.T) {
	got, err := ParseDiversions(strings.NewReader("/a\n/a.d\npkg\n/b\n/b.d\n:\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Diversion{{"/a", "/a.d", "pkg"}, {"/b", "/b.d", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if _, err := ParseDiversions(strings.NewReader("/a\n/a.d\npkg\n/b\n/b.d\n")); err == nil {
		t.Fatal("expected error on partial record")
	}
}

func TestRealState(t *testing.T) {
	f, err := os.Open("/var/lib/dpkg/status")
	if err != nil {
		t.Skip()
	}
	defer f.Close()
	pkgs, err := ParseStatus(f)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range pkgs {
		if len(p.Conffiles) > 0 {
			n++
		}
	}
	if n == 0 {
		t.Fatal("no packages with conffiles")
	}
	if d, err := os.Open("/var/lib/dpkg/diversions"); err == nil {
		defer d.Close()
		if _, err := ParseDiversions(d); err != nil {
			t.Fatal(err)
		}
	}
}
