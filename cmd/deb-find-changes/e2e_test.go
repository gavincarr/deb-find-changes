package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dfc-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "deb-find-changes")
	if out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture lays out a fake system root and returns its path.
func fixture(t *testing.T) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dpkg := filepath.Join(r, "var/lib/dpkg")
	put(t, dpkg+"/status",
		"Package: base\nStatus: install ok installed\nArchitecture: all\nVersion: 1\nConffiles:\n /etc/base.conf "+md5hex("base\n")+"\n\n"+
			"Package: gone\nStatus: deinstall ok config-files\nArchitecture: all\nVersion: 1\nConffiles:\n /etc/gone.conf "+md5hex("gone\n")+"\n\n")
	put(t, dpkg+"/info/base.list", "/etc/base.conf\n/etc/base.d/data\n")
	put(t, dpkg+"/info/base.md5sums", md5hex("data\n")+"  etc/base.d/data\n")
	put(t, dpkg+"/diversions", "")
	if err := os.MkdirAll(dpkg+"/updates", 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, r+"/var/lib/ucf/hashfile", md5hex("gen\n")+" /etc/gen.conf\n")
	put(t, r+"/var/lib/ucf/registry", "gen /etc/gen.conf\n")
	put(t, r+"/etc/base.conf", "base\n")
	put(t, r+"/etc/base.d/data", "edited\n")
	put(t, r+"/etc/gone.conf", "changed\n")
	put(t, r+"/etc/gen.conf", "gen\n")
	put(t, r+"/etc/local.conf", "local\n")
	put(t, r+"/etc/local.conf.dpkg-old", "old\n")
	put(t, r+"/etc/x~", "tilde\n")
	return r
}

func baseArgs(r string) []string {
	return []string{"--root", r, "--admindir", r + "/var/lib/dpkg", "--ucf-dir", r + "/var/lib/ucf"}
}

func runBin(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return o.String(), e.String(), code
}

const defaultOut = "/etc/base.d/data\n/etc/gone.conf\n/etc/local.conf\n/etc/x~\n"

func TestE2EDefault(t *testing.T) {
	r := fixture(t)
	out, errs, code := runBin(t, "", append(baseArgs(r), "/etc")...)
	if code != 0 || out != defaultOut || errs != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
}

func TestE2ERootTrailingSlash(t *testing.T) {
	r := fixture(t)
	args := baseArgs(r)
	args[1] = r + "/"
	out, errs, code := runBin(t, "", append(args, "/etc")...)
	if code != 0 || out != defaultOut || errs != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
}

func TestE2ETypedWithExcludeFile(t *testing.T) {
	r := fixture(t)
	f := filepath.Join(r, "excl")
	put(t, f, "*~\n")
	out, errs, code := runBin(t, "", append(baseArgs(r), "-t", "--exclude-from", f, "/etc")...)
	want := "M /etc/base.d/data\nM /etc/gone.conf\nO /etc/local.conf\n"
	if code != 0 || out != want || errs != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
}

func TestE2EOrphansOnly(t *testing.T) {
	r := fixture(t)
	out, _, code := runBin(t, "", append(baseArgs(r), "-o", "/etc")...)
	if code != 0 || out != "/etc/local.conf\n/etc/x~\n" {
		t.Errorf("code=%d stdout=%q", code, out)
	}
}

func TestE2EOutputFile(t *testing.T) {
	r := fixture(t)
	of := filepath.Join(r, "out/etc.txt")
	if err := os.MkdirAll(filepath.Dir(of), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runBin(t, "", append(baseArgs(r), "-O", of, "/etc")...)
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
	st, err := os.Stat(of)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
	got, _ := os.ReadFile(of)
	if string(got) != defaultOut {
		t.Errorf("content = %q", got)
	}
}

func TestE2EOutputFailureKeepsOld(t *testing.T) {
	r := fixture(t)
	f := filepath.Join(r, "keep.txt")
	put(t, f, "old\n")
	args := append(baseArgs(r), "--admindir", r+"/nonexistent", "-O", f, "/etc")
	_, errs, code := runBin(t, "", args...)
	if code != 1 || errs == "" {
		t.Errorf("code=%d stderr=%q", code, errs)
	}
	got, _ := os.ReadFile(f)
	if string(got) != "old\n" {
		t.Errorf("file = %q", got)
	}
}

func TestE2EMissingExcludeFile(t *testing.T) {
	r := fixture(t)
	nope := filepath.Join(r, "nope")
	out, errs, code := runBin(t, "", append(baseArgs(r), "--exclude-from", nope, "/etc")...)
	if code != 0 || out != defaultOut || !strings.Contains(errs, nope) {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
	_, errs, code = runBin(t, "", append(baseArgs(r), "--no-warnings", "--exclude-from", nope, "/etc")...)
	if code != 0 || errs != "" {
		t.Errorf("no-warnings: code=%d stderr=%q", code, errs)
	}
}

func TestE2EBadPath(t *testing.T) {
	r := fixture(t)
	out, errs, code := runBin(t, "", append(baseArgs(r), "/does-not-exist")...)
	if code != 2 || out != "" || !strings.Contains(errs, "invalid path") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
}

func TestE2ENoPaths(t *testing.T) {
	r := fixture(t)
	out, errs, code := runBin(t, "", baseArgs(r)...)
	if code != 2 || out != "" || !strings.Contains(errs, "at least one <path>") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errs)
	}
}

func TestE2ERelativePath(t *testing.T) {
	r := fixture(t)
	args := []string{"--admindir", r + "/var/lib/dpkg", "--ucf-dir", r + "/var/lib/ucf", "etc"}
	out, _, code := runBin(t, r, args...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no output")
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, r+"/etc/") {
			t.Errorf("line %q lacks prefix %q", l, r+"/etc/")
		}
	}
}

func TestE2ESymlinkedTree(t *testing.T) {
	r := fixture(t)
	if err := os.Symlink("etc", r+"/etcl"); err != nil {
		t.Fatal(err)
	}
	out, errs, code := runBin(t, "", append(baseArgs(r), "/etcl")...)
	if code != 0 || out != defaultOut {
		t.Errorf("code=%d stdout=%q", code, out)
	}
	if !strings.Contains(errs, "/etcl") || !strings.Contains(errs, "/etc") {
		t.Errorf("stderr = %q", errs)
	}
}
