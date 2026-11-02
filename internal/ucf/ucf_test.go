package ucf

import (
	"strings"
	"testing"
)

func TestParseHashfile(t *testing.T) {
	got, err := ParseHashfile(strings.NewReader("1B03ACC84964CF70B8A584870BCDF9B3  /etc/x.conf\n"))
	if err != nil || got["/etc/x.conf"] != "1b03acc84964cf70b8a584870bcdf9b3" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestParseRegistry(t *testing.T) {
	got, err := ParseRegistry(strings.NewReader("rsyslog \t /etc/rsyslog.d/50-default.conf\nfoo /etc/foo\nbar\t/etc/bar baz\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got["/etc/rsyslog.d/50-default.conf"] != "rsyslog" || got["/etc/foo"] != "foo" || got["/etc/bar baz"] != "bar" {
		t.Fatalf("got %v", got)
	}
}
