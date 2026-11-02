package exclude

import (
	"reflect"
	"strings"
	"testing"
)

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
