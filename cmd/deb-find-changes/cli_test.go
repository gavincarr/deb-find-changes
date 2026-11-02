package main

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestGrammar(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, c *CLI)
	}{
		{"counter", []string{"-vv", "/etc"}, func(t *testing.T, c *CLI) {
			if c.Verbose != 2 || !reflect.DeepEqual(c.Paths, []string{"/etc"}) {
				t.Errorf("got Verbose=%d Paths=%v", c.Verbose, c.Paths)
			}
		}},
		{"short combo", []string{"-mt", "/etc"}, func(t *testing.T, c *CLI) {
			if !c.ShowModified || !c.Type {
				t.Errorf("got %+v", c)
			}
		}},
		{"show-type alias", []string{"--show-type", "/etc"}, func(t *testing.T, c *CLI) {
			if !c.Type {
				t.Error("Type not set")
			}
		}},
		{"exclude-pkg alias", []string{"--exclude-pkg", "foo", "--exclude-package", "bar", "/etc"}, func(t *testing.T, c *CLI) {
			if !reflect.DeepEqual(c.ExcludePackage, []string{"foo", "bar"}) {
				t.Errorf("got %v", c.ExcludePackage)
			}
		}},
		{"exclude repeat", []string{"-x", "*.bak", "-x", "core*", "/etc"}, func(t *testing.T, c *CLI) {
			if !reflect.DeepEqual(c.Exclude, []string{"*.bak", "core*"}) {
				t.Errorf("got %v", c.Exclude)
			}
		}},
		{"no-warnings", []string{"--no-warnings", "/etc"}, func(t *testing.T, c *CLI) {
			if c.Warnings {
				t.Error("Warnings should be false")
			}
		}},
		{"warnings default", []string{"/etc"}, func(t *testing.T, c *CLI) {
			if !c.Warnings {
				t.Error("Warnings should default true")
			}
		}},
		{"output", []string{"-O", "/tmp/x", "/etc"}, func(t *testing.T, c *CLI) {
			if c.Output != "/tmp/x" {
				t.Errorf("got %q", c.Output)
			}
		}},
		{"defaults", []string{"/etc"}, func(t *testing.T, c *CLI) {
			if c.Admindir != "/var/lib/dpkg" || c.UcfDir != "/var/lib/ucf" {
				t.Errorf("got %q %q", c.Admindir, c.UcfDir)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cli CLI
			p, err := newParser(&cli, io.Discard, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			tt.check(t, &cli)
		})
	}
}

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Errorf("code = %d", code)
	}
	if out.String() != "deb-find-changes 0.1.0\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errb.String() != "" {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--bogus"}, &out, &errb); code != 2 {
		t.Errorf("code = %d", code)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(errb.String(), "--bogus") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--help"}, &out, &errb); code != 0 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(out.String(), "--exclude-from") {
		t.Errorf("stdout = %q", out.String())
	}
}
