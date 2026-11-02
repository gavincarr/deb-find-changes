# deb-find-changes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `deb-find-changes`, a Go CLI (shipped as a `.deb`) that lists orphaned and locally modified files under given trees on Debian/Ubuntu hosts, for `extract` to back up.

**Architecture:** Read dpkg's and ucf's on-disk state once into an in-memory ownership index (`path → []Claim{package, md5}`), with diversions applied. Walk each tree, apply excludes, classify each regular file (or symlink-to-regular-file) as orphan, modified or unchanged by md5. Write a sorted path list, atomically when `-O` is used.

**Tech Stack:** Go 1.27.2 (stdlib `crypto/md5`, `log/slog`, `io/fs`), `github.com/alecthomas/kong` v1.17.0, `github.com/gavincarr/kong-help-colours` v0.2.0, `github.com/lmittmann/tint` (latest). Debian packaging uses debhelper 13.

**Spec:** `docs/superpowers/specs/2026-11-02-deb-find-changes-design.md`. Read it before starting any task. Section references (§N) below point into it.

## Global Constraints

- Module `github.com/gavincarr/deb-find-changes`; binary `cmd/deb-find-changes`; `CGO_ENABLED=0` builds.
- Version single source: `internal/version/VERSION` = `0.1.0`, `//go:embed`'d. `--version` prints `deb-find-changes 0.1.0` and exits 0 before any other validation.
- Exit codes: `0` success (warnings don't change it); `1` runtime failure; `2` usage error.
- Output: one absolute path per line, byte-sorted, `\n`-terminated, no prefixes unless `-t` (then `M <path>` / `O <path>`).
- `-O FILE`: temp file in the same dir, mode `0600`, then rename. On any failure the previous FILE is untouched.
- **When in doubt, list the file.** Never silently drop a file the tool can't verify.
- Never list directories, dangling symlinks, symlinks to directories, sockets, FIFOs or devices. Never `open()` a non-regular file.
- Logging: `slog` only (no stdlib `log`), tint handler on stderr. Results never go through the logger.
- kong gotchas (skill `go-stack`): explicit `name:` on every field; `-v` is `type:"counter"`; usage-on-error goes to stderr; any non-zero kong exit maps to 2; a grammar test over `kong.New` + `Parse`.
- Default dpkg admin dir `/var/lib/dpkg`; default ucf dir `/var/lib/ucf`. A missing ucf dir is normal (ucf not installed). A missing/unreadable admin dir is exit 1.
- Commit messages: Conventional Commits, ending with the trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Don't push.
- Licence GPLv3 (copy `COPYING` from `~/work/rpm-find-changes/COPYING`).

## Review Focus

1. **Running as non-root against a real `/etc`** (the first thing anyone will try). Unreadable files are listed `M` with a warning. An unreadable directory is logged at **Error** level (so it still shows under `--no-warnings` and cron mails it) and the walk continues. Exit 0. → Task 5 tests `TestScanUnreadableFile` and `TestScanUnreadableDir`; Task 7 smoke run.
2. **Awkward filenames**: spaces, `%`, non-UTF-8 bytes and `[` round-trip verbatim. A newline in a name is skipped with a warning (rsync `--files-from` can't carry it). → Task 5 `TestScanAwkwardNames`, Task 6 `TestWriteSkipsNewline`.
3. **Non-regular entries in the tree**: a FIFO must not hang the run. Dangling symlinks, symlinks to directories and symlink loops are never listed. → Task 5 `TestScanSkipsNonRegular`.
4. **Overlapping or duplicate tree arguments** (`/etc /etc/ssh`, or `/etc` twice) produce each path once. → Task 5 `TestScanOverlappingTrees`.
5. **A file vanishes between walk and hash** (a package upgrade running concurrently with cron) is dropped quietly (debug log), not listed and not fatal. → Task 5 `TestClassifyVanished`.

---

## File structure

```
cmd/deb-find-changes/
  main.go          run(args, stdout, stderr) int; wiring; exit codes
  cli.go           CLI struct, newParser
  logging.go       newLogger(w, verbose, warnings) *slog.Logger
  cli_test.go      grammar tests
  e2e_test.go      builds the binary; runs it against a fixture root
internal/version/  VERSION, version.go
internal/exclude/  exclude.go (glob→regexp, Matcher), file.go (exclude-file parser) + tests
internal/dpkgdb/   list.go, md5sums.go, status.go, diversions.go + tests (pure parsers)
internal/ucf/      ucf.go (hashfile, registry parsers) + tests
internal/index/    index.go (Claim, Index, Load with diversions + ucf) + tests
internal/scan/     scan.go (walk + classify) + tests
internal/output/   output.go (Write, WriteFileAtomic) + tests
etc/exclude-etc    default exclude file (§6 verbatim)
debian/            control, rules, changelog, copyright, install, dirs,
                   deb-find-changes.cron.d, postinst, manpages
man/deb-find-changes.1   hand-written roff man page
scripts/           build_all.sh, deb_tarball.sh, release.sh
README.md, COPYING
```

Leave the existing `.env`, `.envrc`, `.env.local` and `.gitignore` alone. The tool reads no env config, so it doesn't use godotenv (YAGNI).

---

### Task 1: Scaffold, version and CLI grammar

**Files:**
- Create: `internal/version/VERSION`, `internal/version/version.go`, `cmd/deb-find-changes/{main.go,cli.go,cli_test.go}`, `scripts/build_all.sh`
- Modify: `go.mod` (add requires), add `go.sum`

**Interfaces:**
- Produces: `version.Version string`.
- Produces: `type CLI struct` with these fields (all with explicit `name:`):
  - `ShowModified bool` (`-m, --show-modified`), `ShowOrphans bool` (`-o, --show-orphans`)
  - `Type bool` (`-t, --type`, alias `show-type`)
  - `Exclude []string` (`-x, --exclude`), `ExcludeFrom []string` (`--exclude-from`), `ExcludePackage []string` (`--exclude-package`, alias `exclude-pkg`)
  - `Output string` (`-O, --output`)
  - `Verbose int` (`-v`, counter)
  - `Warnings bool` (`--warnings`/`--no-warnings`, `negatable:""`, `default:"true"`, short `w`)
  - `Admindir string` (default `/var/lib/dpkg`), `UcfDir string` (`--ucf-dir`, default `/var/lib/ucf`)
  - `Root string` (`--root`, `hidden:""`, test-only re-rooting; see Task 5)
  - `Version bool` (`--version`)
  - `Paths []string` (`arg:"" optional:"" name:"path"`: optional so that `--version` works with no args; `run` enforces ≥1)
- Produces: `func newParser(cli *CLI, stdout, stderr io.Writer) (*kong.Kong, error)` and `func run(args []string, stdout, stderr io.Writer) int`, following the panic/recover `exitCode` pattern in `~/work/extract/cmd/extract/main.go` and `cli.go` (kong exit 0 → 0, otherwise 2). In this task `run` handles only `--version`, `--help` and parse errors. Anything else prints `deb-find-changes: not implemented` and returns 1 (Task 7 replaces this).

- [ ] **Step 1: Write the failing grammar tests** in `cmd/deb-find-changes/cli_test.go`
  - `TestGrammar`: a table over `newParser(&cli, io.Discard, io.Discard)` + `Parse`, asserting the parsed struct:
    - `-vv /etc` → `Verbose==2`, `Paths==["/etc"]`
    - `-mt /etc` → `ShowModified && Type`
    - `--show-type /etc` → `Type`
    - `--exclude-pkg foo --exclude-package bar /etc` → `ExcludePackage==["foo","bar"]`
    - `-x '*.bak' -x core* /etc` → `Exclude==["*.bak","core*"]`
    - `--no-warnings /etc` → `!Warnings`; `/etc` → `Warnings`
    - `-O /tmp/x /etc` → `Output=="/tmp/x"`
    - no flags → `Admindir=="/var/lib/dpkg"`, `UcfDir=="/var/lib/ucf"`
  - `TestRunVersion`: `run([]string{"--version"}, &out, &errb)` returns 0, `out=="deb-find-changes 0.1.0\n"`, `errb==""`.
  - `TestRunBadFlag`: `run([]string{"--bogus"}, &out, &errb)` returns 2, `out==""`, and `errb` contains `--bogus`. Assert stdout and stderr separately.
  - `TestRunHelp`: `run([]string{"--help"}, …)` returns 0 and `out` contains `--exclude-from`.

- [ ] **Step 2: Run to verify failure.** Run: `go test ./cmd/...`. Expected: build failure (undefined `newParser`/`run`).

- [ ] **Step 3: Implement**
  - `internal/version`: `VERSION` contains `0.1.0`; `version.go` matches `~/work/src/dany/internal/version/version.go`.
  - `cli.go` and `main.go` per the Interfaces block. Use `kong.Name("deb-find-changes")` and a description line from spec §1. `go get github.com/alecthomas/kong@v1.17.0 github.com/gavincarr/kong-help-colours@v0.2.0`.
  - `scripts/build_all.sh`: copy dany's verbatim (it loops over `cmd/*/`).

- [ ] **Step 4: Verify.** Run: `go test ./... && scripts/build_all.sh && bin/deb-find-changes --version`. Expected: PASS; prints `deb-find-changes 0.1.0`.

- [ ] **Step 5: Commit** `feat: scaffold deb-find-changes CLI grammar and version`

---

### Task 2: Exclude patterns

**Files:**
- Create: `internal/exclude/exclude.go`, `internal/exclude/file.go`, `internal/exclude/exclude_test.go`, `internal/exclude/file_test.go`

**Interfaces:**
- Produces:
  ```go
  type Pattern struct { Glob, Source string; Line int } // Source "" / Line 0 for -x and builtins
  var Builtin = []string{"*.dpkg-dist", "*.dpkg-old", "*.dpkg-new", "*.dpkg-tmp", "*.dpkg-bak", "*.ucf-dist", "*.ucf-old", "*.ucf-new"}
  func Compile(pats []Pattern) (*Matcher, error)   // error names Glob, and Source:Line when set
  func (m *Matcher) Match(path string) bool        // for files: all pattern forms
  func (m *Matcher) MatchDir(dir string) bool      // for pruning dirs: only "/..." and trailing-"/" forms
  func ParseFile(r io.Reader, source string) (pats []Pattern, pkgs []string, err error)
  ```

Translation rules (spec §5). Glob → regexp: `*`→`.*`, `?`→`.`, `[...]` copied through (leading `!` becomes `^`), and every other character goes through `regexp.QuoteMeta`. Then wrap by form:

| Form | Regexp | Tested against |
|---|---|---|
| no `/` | `^G$` | basename |
| leading `/` | `^G(/|$)` | full path |
| trailing `/`, not leading | `(^|/)G/` (G = glob minus the trailing `/`) | full path |
| other, containing `/` | `(^|/)G$` | full path |

`MatchDir` applies only the leading-`/` and trailing-`/` forms, testing the dir path with a `/` appended for the trailing-`/` form. Basename forms like `*-` must never prune a directory.

`ParseFile`: trim each line. Skip blank lines and lines starting with `#`. `@name` adds to `pkgs`. Everything else becomes a `Pattern{Glob, source, lineNo}`.

- [ ] **Step 1: Write failing tests.** `TestMatch` is a table of `{pattern, path, want}` including at least:
  - `core*`: `/etc/foo/core.123` → true; `/etc/corefile/x` → false
  - `.*.sw?`: `/etc/.passwd.swp` → true; `/etc/.passwd.sw` → false; `/etc/passwd.swp` → false
  - `*~`: `/etc/a~` → true. `*-`: `/etc/passwd-` → true, `/etc/passwd` → false
  - `*.bak`: `/etc/x.bak` → true; `/etc/xbak` → false (literal `.`)
  - `/etc/mtab`: `/etc/mtab` → true; `/etc/mtab/x` → true; `/etc/mtab.foo` → false
  - `/etc/rc*.d`: `/etc/rc2.d/S01foo` → true
  - `/etc/*/log/main`: `/etc/svc/log/main` → true
  - `.git/`: `/etc/.git/config` → true; `/etc/x.git/config` → false
  - `ssl/private/*`: `/etc/ssl/private/k.pem` → true
  - `[ab].conf`: `/etc/a.conf` → true; `/etc/c.conf` → false. `[!a].conf`: `/etc/b.conf` → true
  - literal metachars: `/etc/a+b(1)`: `/etc/a+b(1)` → true; `/etc/aab(1)` → false
  - builtins: `/etc/x.conf.dpkg-dist`, `/etc/x.ucf-old` → true

  `TestMatchDir`: `/etc/alternatives` → true; `/etc/.git` (pattern `.git/`) → true; pattern `*-` with dir `/etc/foo-` → false. `TestCompileError`: pattern `[` returns an error whose text contains the glob, `exclude-etc:7` when given `Source:"exclude-etc", Line:7`. `TestParseFile`: input `"# c\n\n*~\n@kdebase\n /etc/x \n"` → pats `[*~ (line 3), /etc/x (line 5)]`, pkgs `[kdebase]`.

- [ ] **Step 2: Run to verify failure.** Run: `go test ./internal/exclude/`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** per the Interfaces block and table.
- [ ] **Step 4: Verify.** Run: `go test ./internal/exclude/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(exclude): glob exclude patterns with fixed Perl-era semantics`

---

### Task 3: dpkg and ucf state parsers

**Files:**
- Create: `internal/dpkgdb/{list.go,md5sums.go,status.go,diversions.go}` + `_test.go` for each; `internal/ucf/ucf.go`, `internal/ucf/ucf_test.go`

**Interfaces:**
- Produces:
  ```go
  // dpkgdb
  func ParseList(r io.Reader) ([]string, error)                 // non-empty lines as-is
  func ParseMD5Sums(r io.Reader) (map[string]string, error)     // key = "/" + path; value = lowercased hash
  type Conffile struct { Path, Hash string; Obsolete, RemoveOnUpgrade bool }
  type Package struct { Name, Arch, Status string; Conffiles []Conffile }
  func ParseStatus(r io.Reader) ([]Package, error)
  type Diversion struct { From, To, Package string }             // Package "" for local (":") diversions
  func ParseDiversions(r io.Reader) ([]Diversion, error)        // error on a trailing partial record
  func PackageFromInfoName(name string) string                  // "bind9-libs:amd64.list" → "bind9-libs"
  // ucf
  func ParseHashfile(r io.Reader) (map[string]string, error)    // path → lowercased md5
  func ParseRegistry(r io.Reader) (map[string]string, error)    // path → package
  ```
- Format notes:
  - md5sums: the hash, two spaces, then the path. The path may itself contain spaces, so split at the first run of spaces only.
  - status: deb822 stanzas separated by blank lines. `Conffiles:` continuation lines are ` <path> <hash> [obsolete] [remove-on-upgrade]`. The path may contain spaces, so parse flags from the right: trailing tokens in {`obsolete`,`remove-on-upgrade`}, then the hash, then the rest is the path. `Hash` may be the literal `newconffile`; keep it as-is. `Status` is the `Status:` value. `Arch` comes from `Architecture:`.
  - registry lines look like `rsyslog \t /etc/rsyslog.d/50-default.conf` (mixed whitespace): the first field is the package and the remainder, trimmed, is the path.
  - hashfile: `<md5>  <path>`.

- [ ] **Step 1: Write failing tests** with inline fixture strings. Cover:
  - a md5sums line with a space in its path
  - a status stanza with three conffiles: plain; `newconffile remove-on-upgrade`; `obsolete`
  - an `rc` package (`Status: deinstall ok config-files`) whose conffiles are kept
  - a stanza without `Conffiles`
  - continuation lines of *other* multi-line fields (`Description:`) not leaking into conffiles
  - diversions: two records, one with package `:` → `Package==""`; a 2-line trailing fragment → error
  - `PackageFromInfoName("libc6:amd64.md5sums")=="libc6"`, `PackageFromInfoName("bash.list")=="bash"`
  - registry lines with tabs and spaces; hashfile with uppercase hex → lowercased
- [ ] **Step 2: Run to verify failure.** Run: `go test ./internal/dpkgdb/ ./internal/ucf/`. Expected: FAIL.
- [ ] **Step 3: Implement** with `bufio.Scanner`, setting a 1 MiB max token size because status files have long lines.
- [ ] **Step 4: Verify.** Run the same command. Expected: PASS. Then sanity-check against this machine's real state with a throwaway `go test -run TestRealState` guarded by `if _, err := os.Stat("/var/lib/dpkg/status"); err != nil { t.Skip() }`. It parses `/var/lib/dpkg/status` and `/var/lib/dpkg/diversions` without error and finds >0 packages with conffiles. Keep the test, since it's skip-guarded.
- [ ] **Step 5: Commit** `feat(dpkgdb): parsers for dpkg list, md5sums, status, diversions and ucf state`

---

### Task 4: Ownership index

**Files:**
- Create: `internal/index/index.go`, `internal/index/index_test.go`

**Interfaces:**
- Consumes: Task 3 parsers.
- Produces:
  ```go
  type Claim struct { Package, Hash string }  // Hash "" when nothing recorded
  func (c Claim) HasHash() bool               // exactly 32 hex chars ("newconffile" → false)
  type Index struct{ /* map[string][]Claim */ }
  func New() *Index
  func (ix *Index) Add(path string, c Claim)  // dedupes identical Package for a path, preferring a claim with a hash
  func (ix *Index) Lookup(path string) []Claim
  func Load(admindir, ucfdir string) (*Index, []string, error) // []string = warnings for the caller to log
  ```

`Load` algorithm. The order matters:
1. For each `info/*.list`: `pkg := PackageFromInfoName`. For each path, `Add(path, Claim{pkg, md5s[path]})`, where `md5s` is that package's `info/<same stem>.md5sums` (missing file → empty map).
2. For each status package with conffiles: `Add(cf.Path, Claim{pkg.Name, cf.Hash})`. This covers `rc` packages, which have no `.list`. A conffile claim's hash replaces a hashless claim from step 1 for the same package.
3. Diversions: for each `{From, To, P}`, take every claim on `From` whose `Package != P` (for a local diversion, `P==""`, that's all claims). Remove them from `From` and `Add` them to `To` unchanged, keeping their hash for the original path. Claims by `P` itself stay on `From`.
4. ucf: for each path in the registry or the hashfile, `Add(path, Claim{registry[path], hashfile[path]})`. `Package` may be `""` if the path is only in the hashfile.
5. Warnings:
   - `admindir/updates/` contains any entry whose name is all digits → `"dpkg updates journal not empty (interrupted dpkg run?); ownership data may be stale"`.
   - A missing `admindir/diversions` is fine.
   - A missing `ucfdir` is fine and gives no warning.
   - A missing `admindir/status` or `admindir/info` → return error.

- [ ] **Step 1: Write failing tests.** Build fixture admin and ucf dirs under `t.TempDir()` with a helper `writeFile(t, root, rel, content)`. Cases:
  - `TestLoadListAndMD5`: pkg `a` lists `/etc/a.conf` (md5 present) and `/usr/bin/a` → `Lookup("/etc/a.conf")==[{a,<hash>}]`.
  - `TestLoadMultiArchName`: `info/b:amd64.list` → claims attributed to `b`.
  - `TestLoadConffiles`: status conffile for an `rc` package `c` with no `.list` → `Lookup` returns `{c,hash}`; conffile `newconffile` → `HasHash()==false`.
  - `TestLoadDiversion`: pkg `orig` lists `/etc/x`; pkg `div` lists `/etc/x`; diversion `/etc/x → /etc/x.distrib by div` → `Lookup("/etc/x")==[{div,…}]`, `Lookup("/etc/x.distrib")==[{orig,<orig's hash for /etc/x>}]`.
  - `TestLoadLocalDiversion`: diversion with `:` → `Lookup(From)` empty (orphan), `Lookup(To)` has the original owner.
  - `TestLoadUCF`: registry `rsyslog /etc/r.conf` + hashfile → `{rsyslog,hash}`; registry entry with no hashfile line → `HasHash()==false`.
  - `TestLoadUpdatesWarning`: `updates/0001` present → one warning; `updates/tmp.i` only → no warning.
  - `TestLoadMissingStatus` → error. `TestLoadMissingUCFDir` → no error, no warning.
- [ ] **Step 2: Run to verify failure.** Run: `go test ./internal/index/`. Expected: FAIL.
- [ ] **Step 3: Implement** per the algorithm.
- [ ] **Step 4: Verify.** Run: `go test ./internal/index/`. Expected: PASS. Add a skip-guarded `TestLoadRealSystem` that loads `/var/lib/dpkg` + `/var/lib/ucf` and asserts `Lookup("/etc/bash.bashrc")` has a hashed claim for `bash`.
- [ ] **Step 5: Commit** `feat(index): build file ownership index from dpkg and ucf state`

---

### Task 5: Scan and classify

**Files:**
- Create: `internal/scan/scan.go`, `internal/scan/scan_test.go`

**Interfaces:**
- Consumes: `index.Index`, `index.Claim`, `exclude.Matcher`.
- Produces:
  ```go
  type Kind byte
  const ( Modified Kind = 'M'; Orphan Kind = 'O' )
  type Entry struct { Path string; Kind Kind }
  type Options struct {
      Root        string            // "" normally; tests/e2e prefix every filesystem access with it
      Trees       []string          // absolute paths, as the user sees them (un-rooted)
      Modified, Orphans bool        // which kinds to report
      Exclude     *exclude.Matcher
      ExcludePkgs map[string]bool
      Index       *index.Index
      Log         *slog.Logger
  }
  func Scan(o Options) ([]Entry, error) // byte-sorted, deduped by Path
  ```

Behaviour (spec §4, plus the Review Focus items):
- Walk `o.Root+tree` with `filepath.WalkDir`. The logical path is the walked path with `o.Root` stripped; it's used for excludes, lookup and output.
- Directories: `Exclude.MatchDir(logical)` → `SkipDir`. A walk error on a directory → `Log.Error("cannot read directory", "path", logical, "err", err)` and skip it. The walk continues.
- Files: skip if `Exclude.Match(logical)`. `Lstat`.
  - Regular file → candidate.
  - Symlink → `os.Stat` (follows the link). Candidate only if the target is a regular file. Dangling links, links to directories and loops are skipped (debug log).
  - Any other type → skip.
- Claims: drop claims whose `Package` is in `ExcludePkgs`. If claims existed but all were dropped → skip the file.
- No claims → `Orphan`.
- If no claim has a hash:
  - symlink → skip (a packaged symlink)
  - otherwise → `Modified` + `Log.Warn("no checksum recorded", …)`
- Otherwise md5 the file. `os.Open` follows symlinks, and there is only one open per file.
  - Open error `fs.ErrNotExist` → skip (vanished; debug log).
  - Other error → `Modified` + `Log.Warn("cannot read file", …)`.
  - Mismatch with **any** hashed claim (case-insensitive) → `Modified` (debug log with got/expected). Else unchanged.
- Report only the kinds enabled in `Options`.
- Wrap the per-file decision in an unexported `classify(logical, real string, isSymlink bool, claims []index.Claim) (Kind, bool)`, where bool means "list it". `TestClassifyVanished` calls it directly.

- [ ] **Step 1: Write failing tests.** Each test builds a tree under `t.TempDir()` used as `Root`, and an `index.New()` populated with `Add`. Use a discard logger, or a buffer-backed `slog.NewTextHandler` where a test asserts a log line.
  - `TestScanBasic`: `/etc/orphan` (no claim) → `O`; `/etc/same` (claim with correct md5) → not listed; `/etc/changed` (claim, wrong md5) → `M`; `/etc/nohash` (claim, hash "") → `M` plus a "no checksum" warn record.
  - `TestScanKinds`: with `Modified:true, Orphans:false`, only `M` entries are returned, and vice versa.
  - `TestScanExcludes`: `-x '*.bak'` drops `/etc/x.bak`; `MatchDir` on `/etc/alternatives` prunes it. A file claimed only by excluded package `kde` → not listed. A file claimed by `kde` + `other` (other's hash matches) → not listed.
  - `TestScanSymlinks`: unowned symlink `/etc/systemd/system/multi-user.target.wants/x.service → ../x.service` (regular target) → `O`. An owned hashless symlink → not listed. A symlink at an owned path *with* a hash, pointing to a file with different content → `M`.
  - `TestScanSkipsNonRegular`: a FIFO (`syscall.Mkfifo`), a dangling symlink, a symlink to a directory and a two-link loop → nothing listed. Use a 5s `context`/timer guard in the test so a hang fails rather than blocks.
  - `TestScanUnreadableFile` (skip if `os.Geteuid()==0`): owned file with hash, mode 000 → `M` plus a "cannot read file" warn.
  - `TestScanUnreadableDir` (skip if root): `/etc/private` with mode 000 → Error-level log record mentioning it; `Scan` returns nil error; sibling files are still classified.
  - `TestScanAwkwardNames`: orphans named `a b`, `100%`, `x[1]` and the non-UTF-8 name `"\xff.conf"` → each listed with its exact path.
  - `TestScanOverlappingTrees`: `Trees: ["/etc", "/etc/ssh", "/etc"]` → each orphan listed once; output is sorted.
  - `TestClassifyVanished`: `classify` on a non-existent real path with a hashed claim → `(_, false)`.
- [ ] **Step 2: Run to verify failure.** Run: `go test ./internal/scan/`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Behaviour list.
- [ ] **Step 4: Verify.** Run: `go test ./internal/scan/` (as a normal user). Expected: PASS; the two unreadable-path tests skip only when run as root.
- [ ] **Step 5: Commit** `feat(scan): walk trees and classify orphaned and modified files`

---

### Task 6: Output writing

**Files:**
- Create: `internal/output/output.go`, `internal/output/output_test.go`

**Interfaces:**
- Consumes: `scan.Entry`.
- Produces:
  ```go
  func Write(w io.Writer, entries []scan.Entry, withType bool, log *slog.Logger) error // skips paths containing '\n' with a Warn
  func WriteFileAtomic(path string, mode os.FileMode, fill func(io.Writer) error) error
  ```
  `WriteFileAtomic` does the following, in order:
  1. `os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")`
  2. `Chmod(mode)`
  3. `fill` through a `bufio.Writer`, then flush, `Sync`, `Close`
  4. `os.Rename`

  On any error it removes the temp file and returns the error, leaving `path` untouched.

- [ ] **Step 1: Write failing tests**
  - `TestWritePlain` → `"/etc/a\n/etc/b\n"`.
  - `TestWriteTyped` → `"M /etc/a\nO /etc/b\n"`.
  - `TestWriteSkipsNewline`: an entry `"/etc/a\nb"` is omitted, with one warn record.
  - `TestWriteFileAtomicMode`: the result has mode `0600` and the expected content, and no temp files are left in the dir.
  - `TestWriteFileAtomicFailureKeepsOld`: with an existing file holding `old\n`, `fill` returns an error → the file still holds `old\n`, the error is returned, and no temp files remain.
  - `TestWriteFileAtomicMissingDir` → error.
- [ ] **Step 2: Run to verify failure.** Run: `go test ./internal/output/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Verify.** Run: `go test ./internal/output/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(output): plain/typed list writer and atomic file output`

---

### Task 7: Wire up the CLI, logging and end-to-end tests

**Files:**
- Create: `cmd/deb-find-changes/logging.go`, `cmd/deb-find-changes/e2e_test.go`
- Modify: `cmd/deb-find-changes/main.go` (replace the "not implemented" path)

**Interfaces:**
- Consumes: everything above.
- Produces: `func newLogger(w io.Writer, verbose int, warnings bool) *slog.Logger`. It uses a tint handler. Level:
  - `verbose>=2` → Debug
  - `verbose==1` → Info
  - otherwise Warn, or Error when `!warnings`

  Set `tint.Options.NoColor` when `w` isn't a terminal (`golang.org/x/term`, or tint's own guidance). Don't call `slog.SetDefault`; pass the logger explicitly.

`run` flow after a successful parse:
1. `--version` → print and return 0.
2. `len(Paths)==0` → usage error. Print `deb-find-changes: at least one <path> is required` plus the usage line to stderr; return 2.
3. Normalise paths. With `Root==""`, make each absolute with `filepath.Abs`. With `Root!=""`, each must already be absolute. Each `Root+path` must be an existing directory, else `deb-find-changes: invalid path '<p>'` on stderr and return 2.
4. Default kinds: if neither `-m` nor `-o` is given, both are on.
5. Excludes:
   - patterns = builtins + each `--exclude-from` file (a missing/unreadable file → `Log.Warn` and continue; its `@pkg` lines go to the package set) + `-x` values
   - package set = the `--exclude-package` values plus those `@pkg` lines
   - `exclude.Compile` error → stderr message and return 2
6. `index.Load(Admindir, UcfDir)`. On error, `Log.Error` and return 1. Log each returned warning with `Log.Warn`.
7. `scan.Scan`. On error, `Log.Error` and return 1.
8. Output: with `-O`, use `output.WriteFileAtomic(Output, 0600, …)`, and on error `Log.Error` and return 1. Otherwise `output.Write(stdout, …)`. Return 0.
9. `-v` progress lines at Info: trees being scanned, and counts of packages loaded, files checked, orphans and modified.

- [ ] **Step 1: Write the failing e2e test** `cmd/deb-find-changes/e2e_test.go`. `TestMain` builds the binary into a temp dir (`go build -o`). A helper `fixture(t)` lays out a root containing:
  - `var/lib/dpkg/status`: pkg `base` (installed) with conffile `/etc/base.conf <md5 of "base\n">`; pkg `gone` (`deinstall ok config-files`) with conffile `/etc/gone.conf <md5 of "gone\n">`
  - `var/lib/dpkg/info/base.list` (`/etc/base.conf`, `/etc/base.d/data`)
  - `var/lib/dpkg/info/base.md5sums` (md5 of `data\n` for `etc/base.d/data`)
  - empty `var/lib/dpkg/diversions` and `var/lib/dpkg/updates/`
  - `var/lib/ucf/hashfile` + `registry` for `/etc/gen.conf` (pkg `gen`, md5 of `gen\n`)
  - `etc/base.conf` = `base\n` (unchanged); `etc/base.d/data` = `edited\n` (modified); `etc/gone.conf` = `changed\n` (modified, rc package); `etc/gen.conf` = `gen\n` (unchanged, ucf); `etc/local.conf` (orphan); `etc/local.conf.dpkg-old` (builtin exclude); `etc/x~` (excluded via an exclude file)

  Tests:
  - `TestE2EDefault`: run `--root R --admindir R/var/lib/dpkg --ucf-dir R/var/lib/ucf /etc`. Expect exit 0, stdout `/etc/base.d/data\n/etc/gone.conf\n/etc/local.conf\n/etc/x~\n` and stderr empty.
  - `TestE2ETypedWithExcludeFile`: the same plus `-t --exclude-from F`, where F contains `*~`. Expect `M /etc/base.d/data\nM /etc/gone.conf\nO /etc/local.conf\n`.
  - `TestE2EOrphansOnly`: `-o` → `/etc/local.conf\n/etc/x~\n`.
  - `TestE2EOutputFile`: `-O R/out/etc.txt` → stdout empty, file mode 0600, same content as the default test.
  - `TestE2EOutputFailureKeepsOld`: `--admindir R/nonexistent -O F`, where F already holds `old\n` → exit 1, F still `old\n`, stderr non-empty.
  - `TestE2EMissingExcludeFile`: `--exclude-from R/nope` → exit 0 and stderr mentions `R/nope`. With `--no-warnings` → stderr empty.
  - `TestE2EBadPath`: `/does-not-exist` → exit 2, stdout empty.
  - `TestE2ENoPaths`: no path args → exit 2, stdout empty.
  - `TestE2ERelativePath`: run with `cmd.Dir = R` and no `--root`, passing the relative path `etc` plus the fixture admindir/ucfdir → output paths are absolute and start with `R/etc/`.
- [ ] **Step 2: Run to verify failure.** Run: `go test ./cmd/...`. Expected: FAIL (not implemented → exit 1).
- [ ] **Step 3: Implement** `logging.go` and the `run` flow. `go get github.com/lmittmann/tint`.
- [ ] **Step 4: Verify.** Run: `go test ./... && gofmt -l .`. Expected: PASS and no gofmt output.
- [ ] **Step 5: Smoke run on this machine** (non-root, real dpkg state):
  ```bash
  scripts/build_all.sh
  out="$(mktemp)"; bin/deb-find-changes -t -v /etc > "$out"; echo "exit=$?"; wc -l "$out"
  ```
  No exclude file yet (`etc/exclude-etc` arrives in Task 8). Expected:
  - `exit=0`
  - no panic
  - `/etc/hostname` appears as `O`
  - unreadable root-only files (e.g. `/etc/shadow`) appear as `O` or `M` without aborting the run
  - Error-level lines for any root-only directories

  Paste the counts and a sample of the output into the task report.
- [ ] **Step 6: Commit** `feat: wire up scan, excludes and output in the CLI`

---

### Task 8: Default excludes, Debian packaging, man page, release scripts

**Files:**
- Create:
  - `etc/exclude-etc`: spec §6 block verbatim, including the commented-out secrets block
  - `debian/{control,rules,changelog,copyright,install,dirs,deb-find-changes.cron.d,postinst,manpages}`
  - `man/deb-find-changes.1`
  - `scripts/deb_tarball.sh`, `scripts/release.sh`
  - `README.md`
  - `COPYING` (from rpm-find-changes)
- Modify: `.gitignore` only if `dist/` or `debian/` build outputs aren't already ignored. Add `debian/deb-find-changes/`, `debian/.debhelper/`, `debian/files`, `debian/*.substvars` and `debian/debhelper-build-stamp`.

**Packaging specifics:**
- `debian/control`: model on `~/work/src/dany/debian/control`.
  - Source/Package `deb-find-changes`, Section `admin`, `Build-Depends: debhelper-compat (= 13)`, `Architecture: amd64`, `Depends: ${misc:Depends}`, `Recommends: cron | cron-daemon`.
  - Description: one line from spec §1, plus a short body mentioning extract and `/var/cache/deb-find-changes/etc.txt`.
- `debian/rules`: dany's, plus:
  ```make
  override_dh_builddeb:
  	dh_builddeb -- -Zzstd
  ```
- `debian/install`: `bin/deb-find-changes usr/bin` and `etc/exclude-etc etc/deb-find-changes`. dh marks files under `/etc` as conffiles automatically.
- `debian/dirs`: `var/cache/deb-find-changes`.
- `debian/deb-find-changes.cron.d`: installed by `dh_installcron` as `/etc/cron.d/deb-find-changes`. Contents:
  - header comment: `# Find dpkg changes in the /etc tree`
  - `0 0 * * * root /usr/bin/deb-find-changes --no-warnings --exclude-from=/etc/deb-find-changes/exclude-etc -O /var/cache/deb-find-changes/etc.txt /etc`
- `debian/postinst` (`#!/bin/sh`, `set -e`). On `configure`, if `/var/cache/deb-find-changes/etc.txt` doesn't exist, run the exact cron command, with `|| echo "deb-find-changes: initial scan failed; the nightly cron job will retry" >&2` on the end. Then `#DEBHELPER#` and `exit 0`.
- `debian/changelog`: one stanza, `deb-find-changes (0.1.0) unstable; urgency=medium`, with `* Initial release.` Maintainer `Gavin Carr <gavin@profound.net>`, current date.
- `debian/copyright`: DEP-5, GPL-3+, Gavin Carr.
- `debian/manpages`: `man/deb-find-changes.1`.
- Man page: hand-written roff covering NAME, SYNOPSIS, DESCRIPTION (orphan/modified, what's verified), OPTIONS (every §7 flag except the hidden `--root`), EXCLUDES (the §5 pattern table, `@pkg`, builtins), FILES (cron, exclude-etc, cache path), EXIT STATUS, SEE ALSO `dpkg(1)`, `debsums(1)`, `extract`.
- `scripts/deb_tarball.sh`: dany's, with `PKG="deb-find-changes"` and the single binary. Also stage `etc/` and `man/`.
- `scripts/release.sh`: adapt dany's, but follow the `release-flow` skill where they differ:
  - gate 2 derives the branch from `git rev-parse --abbrev-ref '@{u}'` and **fails** when no upstream is set (this repo has no remote yet, which is expected)
  - gate 6 adds `test -z "$(gofmt -l .)"`
  - no push
- `README.md`: what it is, the relationship to rpm-find-changes and extract, usage, the secrets block note, build (`scripts/build_all.sh`), and packaging (`scripts/deb_tarball.sh`, then `dpkg-buildpackage -b -us -uc` on `repo`).

- [ ] **Step 1: Write the packaging files** listed above.
- [ ] **Step 2: Verify the exclude file against real /etc.** Run: `bin/deb-find-changes -t --exclude-from=etc/exclude-etc /etc | wc -l` vs the same without `--exclude-from`. Expected: the count drops. Eyeball the remaining list; if something is obviously generated noise, add it to the non-secrets section and note each addition in the commit body. Keep the secrets block commented out.
- [ ] **Step 3: Verify the man page.** Run: `man -l man/deb-find-changes.1 | head -40`. Expected: it renders with no warnings from `groff -ww -z -man man/deb-find-changes.1`.
- [ ] **Step 4: Build the package locally.** Run:
  ```bash
  scripts/deb_tarball.sh && cd dist/deb && tar xzf deb-find-changes_0.1.0.tar.gz && cd deb-find-changes-0.1.0 && dpkg-buildpackage -b -us -uc
  ```
  Then on the produced `.deb`:
  - `dpkg-deb -c` lists `/usr/bin/deb-find-changes`, `/etc/deb-find-changes/exclude-etc`, `/etc/cron.d/deb-find-changes`, `/var/cache/deb-find-changes/` and the man page
  - `dpkg-deb -I` shows `Recommends: cron | cron-daemon`, and `conffiles` lists both `/etc` files
  - `ar t` shows `data.tar.zst`
  - `sh -n debian/postinst` passes, and `shellcheck debian/postinst` passes if shellcheck is installed
- [ ] **Step 5: Verify the release script's gates fail safely.** Run: `scripts/release.sh 0.1.0`. Expected: it fails at the upstream gate ("no upstream"), having created no commit and no tag. Don't actually release.
- [ ] **Step 6: Commit** `build: add Debian packaging, default excludes, man page and release scripts`. Don't commit `dist/`.

---

## Spec coverage check

| Spec | Task |
|---|---|
| §3 output contract, atomic `-O`, newline paths | 6, 7 |
| §4 classification table, data sources, diversions, rc, ucf, md5 only | 3, 4, 5 |
| §5 excludes, builtins, pattern semantics, compile errors | 2, 7 |
| §6 default exclude file, secrets commented | 8 |
| §7 CLI flags, exit codes, relative paths, missing exclude file, updates journal, logging levels | 1, 4, 7 |
| §8 version, release.sh, packaging (zstd, Recommends, postinst initial scan, cron, dirs, man page, GPLv3) | 1, 8 |
| §9 testing (parsers, synthetic root, Perl-era cases, CLI stdout/stderr, atomic, smoke run) | 2–7 |
