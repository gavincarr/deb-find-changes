// Package exclude compiles glob exclude patterns into regexps and matches
// paths against them.
package exclude

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Pattern is a single exclude glob. Source is "" and Line 0 for -x
// patterns and builtins.
type Pattern struct {
	Glob, Source string
	Line         int
}

// Builtin lists the always-applied exclude globs.
var Builtin = []string{"*.dpkg-dist", "*.dpkg-old", "*.dpkg-new", "*.dpkg-tmp", "*.dpkg-bak", "*.ucf-dist", "*.ucf-old", "*.ucf-new"}

type form int

const (
	formBase  form = iota // no "/": tested against basename
	formAbs               // leading "/"
	formDir               // trailing "/", not leading
	formOther             // contains "/" elsewhere
)

type rule struct {
	form form
	re   *regexp.Regexp
}

// Matcher matches paths against compiled patterns.
type Matcher struct {
	rules []rule
}

// Compile translates pats into a Matcher.
func Compile(pats []Pattern) (*Matcher, error) {
	m := &Matcher{}
	for _, p := range pats {
		r, err := compileOne(p.Glob)
		if err != nil {
			where := ""
			switch {
			case p.Source != "" && p.Line > 0:
				where = fmt.Sprintf(" (%s:%d)", p.Source, p.Line)
			case p.Source != "":
				where = fmt.Sprintf(" (%s)", p.Source)
			}
			return nil, fmt.Errorf("invalid exclude pattern %q%s: %w", p.Glob, where, err)
		}
		m.rules = append(m.rules, r)
	}
	return m, nil
}

func compileOne(glob string) (rule, error) {
	var f form
	var expr string
	switch {
	case strings.HasPrefix(glob, "/"):
		f = formAbs
		g, err := translate(strings.TrimSuffix(glob, "/"))
		if err != nil {
			return rule{}, err
		}
		expr = "^" + g + "(/|$)"
	case strings.HasSuffix(glob, "/"):
		f = formDir
		g, err := translate(strings.TrimSuffix(glob, "/"))
		if err != nil {
			return rule{}, err
		}
		expr = "(^|/)" + g + "/"
	case strings.Contains(glob, "/"):
		f = formOther
		g, err := translate(glob)
		if err != nil {
			return rule{}, err
		}
		expr = "(^|/)" + g + "$"
	default:
		f = formBase
		g, err := translate(glob)
		if err != nil {
			return rule{}, err
		}
		expr = "^" + g + "$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return rule{}, err
	}
	return rule{f, re}, nil
}

// translate converts a glob body to regexp syntax.
func translate(g string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			j := i + 1
			if j < len(g) && g[j] == '!' {
				j++
			}
			if j < len(g) && g[j] == ']' { // leading ] is literal
				j++
			}
			end := strings.IndexByte(g[j:], ']')
			if end < 0 {
				return "", fmt.Errorf("unclosed '['")
			}
			end += j
			class := g[i+1 : end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + strings.ReplaceAll(class, `\`, `\\`) + "]")
			i = end
		default:
			r, size := utf8.DecodeRuneInString(g[i:])
			b.WriteString(regexp.QuoteMeta(string(r)))
			i += size - 1
		}
	}
	return b.String(), nil
}

// Match reports whether path (a file) matches any pattern.
func (m *Matcher) Match(path string) bool {
	base := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		base = path[i+1:]
	}
	for _, r := range m.rules {
		if r.form == formBase {
			if r.re.MatchString(base) {
				return true
			}
		} else if r.re.MatchString(path) {
			return true
		}
	}
	return false
}

// MatchDir reports whether dir should be pruned. Only leading-"/" and
// trailing-"/" patterns apply; basename forms never prune directories.
func (m *Matcher) MatchDir(dir string) bool {
	for _, r := range m.rules {
		switch r.form {
		case formAbs:
			if r.re.MatchString(dir) {
				return true
			}
		case formDir:
			if r.re.MatchString(dir + "/") {
				return true
			}
		}
	}
	return false
}
