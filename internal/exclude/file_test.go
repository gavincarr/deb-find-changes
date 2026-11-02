package exclude

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseFileBareAt(t *testing.T) {
	_, pkgs, err := ParseFile(strings.NewReader("@\n  @  \n@foo\n"), "f")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pkgs, []string{"foo"}) {
		t.Errorf("pkgs = %v", pkgs)
	}
}

func TestParseFile(t *testing.T) {
	pats, pkgs, err := ParseFile(strings.NewReader("# c\n\n*~\n@kdebase\n /etc/x \n"), "f")
	if err != nil {
		t.Fatal(err)
	}
	wantP := []Pattern{{"*~", "f", 3}, {"/etc/x", "f", 5}}
	if !reflect.DeepEqual(pats, wantP) {
		t.Errorf("pats = %v, want %v", pats, wantP)
	}
	if !reflect.DeepEqual(pkgs, []string{"kdebase"}) {
		t.Errorf("pkgs = %v", pkgs)
	}
}
