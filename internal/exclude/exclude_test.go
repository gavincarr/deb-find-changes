package exclude

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"core*", "/etc/foo/core.123", true},
		{"core*", "/etc/corefile/x", false},
		{".*.sw?", "/etc/.passwd.swp", true},
		{".*.sw?", "/etc/.passwd.sw", false},
		{".*.sw?", "/etc/passwd.swp", false},
		{"*~", "/etc/a~", true},
		{"*-", "/etc/passwd-", true},
		{"*-", "/etc/passwd", false},
		{"*.bak", "/etc/x.bak", true},
		{"*.bak", "/etc/xbak", false},
		{"/etc/mtab", "/etc/mtab", true},
		{"/etc/mtab", "/etc/mtab/x", true},
		{"/etc/mtab", "/etc/mtab.foo", false},
		{"/etc/rc*.d", "/etc/rc2.d/S01foo", true},
		{"/etc/*/log/main", "/etc/svc/log/main", true},
		{".git/", "/etc/.git/config", true},
		{".git/", "/etc/x.git/config", false},
		{"ssl/private/*", "/etc/ssl/private/k.pem", true},
		{"[ab].conf", "/etc/a.conf", true},
		{"[ab].conf", "/etc/c.conf", false},
		{"[!a].conf", "/etc/b.conf", true},
		{"[!a].conf", "/etc/a.conf", false},
		{"/etc/a+b(1)", "/etc/a+b(1)", true},
		{"/etc/a+b(1)", "/etc/aab(1)", false},
		{"a]b", "/etc/a]b", true},
		{"café", "/etc/café", true},
		{"café", "/etc/cafe", false},
		{"/etc/café", "/etc/café", true},
		{"/etc/café", "/etc/café/x", true},
		{Builtin[0], "/etc/x.conf.dpkg-dist", true},
		{Builtin[6], "/etc/x.ucf-old", true},
	}
	for _, tc := range tests {
		m, err := Compile([]Pattern{{Glob: tc.pattern}})
		if err != nil {
			t.Fatalf("Compile(%q): %v", tc.pattern, err)
		}
		if got := m.Match(tc.path); got != tc.want {
			t.Errorf("pattern %q path %q: got %v want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestBuiltinsCompile(t *testing.T) {
	var ps []Pattern
	for _, g := range Builtin {
		ps = append(ps, Pattern{Glob: g})
	}
	m, err := Compile(ps)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match("/etc/x.conf.dpkg-dist") || !m.Match("/etc/x.ucf-old") || m.Match("/etc/x.conf") {
		t.Error("builtin matching wrong")
	}
}

func TestMatchDir(t *testing.T) {
	tests := []struct {
		pattern, dir string
		want         bool
	}{
		{"/etc/alternatives", "/etc/alternatives", true},
		{"/etc/alternatives", "/etc/other", false},
		{".git/", "/etc/.git", true},
		{"*-", "/etc/foo-", false},
		{"core*", "/etc/core", false},
		{"ssl/private", "/etc/ssl/private", false},
	}
	for _, tc := range tests {
		m, err := Compile([]Pattern{{Glob: tc.pattern}})
		if err != nil {
			t.Fatal(err)
		}
		if got := m.MatchDir(tc.dir); got != tc.want {
			t.Errorf("pattern %q dir %q: got %v want %v", tc.pattern, tc.dir, got, tc.want)
		}
	}
}

func TestCompileErrorNoLine(t *testing.T) {
	_, err := Compile([]Pattern{{Glob: "[", Source: "command line"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), ":0") || !strings.Contains(err.Error(), "(command line)") {
		t.Errorf("bad error text %q", err)
	}
}

func TestDefaultCoreExcludes(t *testing.T) {
	m, err := Compile([]Pattern{{Glob: "core"}, {Glob: "core.[0-9]*"}})
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{
		"/etc/x/core":                 true,
		"/etc/x/core.1234":            true,
		"/etc/systemd/coredump.conf":  false,
		"/etc/X11/cursors/core.theme": false,
	} {
		if got := m.Match(p); got != want {
			t.Errorf("Match(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestCompileError(t *testing.T) {
	_, err := Compile([]Pattern{{Glob: "[", Source: "exclude-etc", Line: 7}})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, s := range []string{"[", "exclude-etc:7"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q missing %q", err, s)
		}
	}
}
