package main

import (
	"io"

	"github.com/alecthomas/kong"
	helpcolours "github.com/gavincarr/kong-help-colours"
)

// CLI is the deb-find-changes command-line grammar.
type CLI struct {
	ShowModified   bool     `name:"show-modified" short:"m" help:"Show modified files."`
	ShowOrphans    bool     `name:"show-orphans" short:"o" help:"Show orphaned files."`
	Type           bool     `name:"type" short:"t" aliases:"show-type" help:"Prefix each file with its type."`
	Exclude        []string `sep:"none" name:"exclude" short:"x" help:"Exclude files matching this glob (repeatable)."`
	ExcludeFrom    []string `sep:"none" name:"exclude-from" help:"Read exclude globs from this file (repeatable)."`
	ExcludePackage []string `sep:"none" name:"exclude-package" aliases:"exclude-pkg" help:"Exclude files owned by this package (repeatable)."`
	Output         string   `name:"output" short:"O" help:"Write output to this file instead of stdout."`
	Verbose        int      `name:"verbose" short:"v" type:"counter" help:"Increase verbosity (repeatable)."`
	Warnings       bool     `name:"warnings" short:"w" negatable:"" default:"true" help:"Emit warnings."`
	Admindir       string   `name:"admindir" default:"/var/lib/dpkg" help:"dpkg administrative directory."`
	UcfDir         string   `name:"ucf-dir" default:"/var/lib/ucf" help:"ucf state directory."`
	Root           string   `name:"root" hidden:"" help:"Re-root the scanned trees (testing only)."`
	Version        bool     `name:"version" help:"Print version and exit."`

	Paths []string `arg:"" optional:"" name:"path" help:"Directories to scan."`
}

// exitCode is panicked by the kong exit hook so run can recover it
// instead of terminating the process.
type exitCode int

func newParser(cli *CLI, stdout, stderr io.Writer) (*kong.Kong, error) {
	return kong.New(cli,
		kong.Name("deb-find-changes"),
		kong.Description("List orphaned and modified files under the given paths, for backup, on Debian systems."),
		kong.Writers(stdout, stderr),
		kong.UsageOnError(),
		kong.Help(helpcolours.Help),
		kong.Exit(func(code int) { panic(exitCode(code)) }),
	)
}
