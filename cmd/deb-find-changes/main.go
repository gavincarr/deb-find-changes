// Command deb-find-changes lists orphaned and modified files for backup.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gavincarr/deb-find-changes/internal/exclude"
	"github.com/gavincarr/deb-find-changes/internal/index"
	"github.com/gavincarr/deb-find-changes/internal/output"
	"github.com/gavincarr/deb-find-changes/internal/scan"
	"github.com/gavincarr/deb-find-changes/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args and returns the process exit code. Kong exits are
// trapped: code 0 (e.g. --help) stays 0, anything else becomes 2.
func run(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			c, ok := r.(exitCode)
			if !ok {
				panic(r)
			}
			if c == 0 {
				code = 0
			} else {
				code = 2
			}
		}
	}()

	var cli CLI
	parser, err := newParser(&cli, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "deb-find-changes:", err)
		return 2
	}
	if _, err := parser.Parse(args); err != nil {
		parser.Stdout = stderr // usage goes to stderr on errors
		parser.FatalIfErrorf(err)
		return 2
	}
	if cli.Version {
		fmt.Fprintf(stdout, "deb-find-changes %s\n", version.Version)
		return 0
	}
	if len(cli.Paths) == 0 {
		fmt.Fprintln(stderr, "deb-find-changes: at least one <path> is required")
		fmt.Fprintln(stderr, "Usage: deb-find-changes [flags] <path> ...")
		return 2
	}
	return execute(&cli, stdout, stderr)
}

// execute runs the scan described by a parsed CLI.
func execute(cli *CLI, stdout, stderr io.Writer) int {
	log := newLogger(stderr, cli.Verbose, cli.Warnings)
	root := cli.Root
	if root != "" {
		root = filepath.Clean(root)
	}

	var trees []string
	for _, p := range cli.Paths {
		if root == "" {
			abs, err := filepath.Abs(p)
			if err != nil {
				fmt.Fprintf(stderr, "deb-find-changes: invalid path '%s'\n", p)
				return 2
			}
			p = abs
		} else if !filepath.IsAbs(p) {
			fmt.Fprintf(stderr, "deb-find-changes: invalid path '%s'\n", p)
			return 2
		}
		p = filepath.Clean(p)
		if st, err := os.Stat(root + p); err != nil || !st.IsDir() {
			fmt.Fprintf(stderr, "deb-find-changes: invalid path '%s'\n", p)
			return 2
		}
		// WalkDir does not descend a symlinked root, so resolve it.
		if st, err := os.Lstat(root + p); err == nil && st.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(root + p)
			if err != nil {
				fmt.Fprintf(stderr, "deb-find-changes: invalid path '%s'\n", p)
				return 2
			}
			logical := resolved
			if root != "" {
				realRoot, err := filepath.EvalSymlinks(root)
				if err != nil {
					realRoot = root
				}
				logical = strings.TrimPrefix(resolved, realRoot)
			}
			log.Warn("path is a symlink; scanning its target", "given", p, "resolved", logical)
			p = logical
		}
		trees = append(trees, p)
	}

	if !cli.ShowModified && !cli.ShowOrphans {
		cli.ShowModified, cli.ShowOrphans = true, true
	}

	var pats []exclude.Pattern
	for _, g := range exclude.Builtin {
		pats = append(pats, exclude.Pattern{Glob: g, Source: "builtin"})
	}
	pkgs := map[string]bool{}
	for _, p := range cli.ExcludePackage {
		pkgs[p] = true
	}
	for _, file := range cli.ExcludeFrom {
		f, err := os.Open(file)
		if err != nil {
			log.Warn("cannot read exclude file", "path", file, "err", err)
			continue
		}
		fp, fk, err := exclude.ParseFile(f, file)
		f.Close()
		if err != nil {
			log.Warn("cannot read exclude file", "path", file, "err", err)
			continue
		}
		pats = append(pats, fp...)
		for _, k := range fk {
			pkgs[k] = true
		}
	}
	for _, g := range cli.Exclude {
		pats = append(pats, exclude.Pattern{Glob: g, Source: "command line"})
	}
	matcher, err := exclude.Compile(pats)
	if err != nil {
		fmt.Fprintln(stderr, "deb-find-changes:", err)
		return 2
	}

	ix, warns, err := index.Load(cli.Admindir, cli.UcfDir)
	if err != nil {
		log.Error("cannot load package database", "err", err)
		return 1
	}
	for _, w := range warns {
		log.Warn(w)
	}

	log.Info("scanning", "trees", trees)
	entries, err := scan.Scan(scan.Options{
		Root:        root,
		Trees:       trees,
		Modified:    cli.ShowModified,
		Orphans:     cli.ShowOrphans,
		Exclude:     matcher,
		ExcludePkgs: pkgs,
		Index:       ix,
		Log:         log,
	})
	if err != nil {
		log.Error("scan failed", "err", err)
		return 1
	}
	var nm, no int
	for _, e := range entries {
		if e.Kind == scan.Modified {
			nm++
		} else {
			no++
		}
	}
	log.Info("scan complete", "modified", nm, "orphans", no)

	if cli.Output != "" {
		err = output.WriteFileAtomic(cli.Output, 0o600, func(w io.Writer) error {
			return output.Write(w, entries, cli.Type, log)
		})
	} else {
		err = output.Write(stdout, entries, cli.Type, log)
	}
	if err != nil {
		log.Error("cannot write output", "err", err)
		return 1
	}
	return 0
}
