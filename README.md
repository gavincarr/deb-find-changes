# deb-find-changes

Lists the files under given directory trees (default `/etc`) that dpkg does
not fully account for:

- **orphans**: files not owned by any installed package;
- **modified** files: owned files whose MD5 checksum differs from the one
  dpkg (or ucf) recorded, or that can't be verified.

It is the Debian counterpart of
[rpm-find-changes](../rpm-find-changes) and is meant to feed `extract`, the
backup tool that saves just the locally-changed files. It reads the dpkg
and ucf databases directly rather than running `dpkg` per file.

## Usage

```
deb-find-changes [flags] <path> [<path> ...]

deb-find-changes -t /etc                  # M/O-prefixed list
deb-find-changes -o /etc                  # orphans only
deb-find-changes -m /etc                  # modified only
deb-find-changes --exclude-from=/etc/deb-find-changes/exclude-etc \
    -O /var/cache/deb-find-changes/etc.txt /etc
```

See `deb-find-changes --help` and `man deb-find-changes` for all flags.
Exit status is 0 on success, 1 on runtime failure, 2 on usage error.

The package installs a nightly cron job (`/etc/cron.d/deb-find-changes`)
that writes the list to `/var/cache/deb-find-changes/etc.txt`, and runs an
initial scan on first install. Removing the package (with or without
purge) deletes the list, so a consumer never reads a stale one.

## Excludes

`/etc/deb-find-changes/exclude-etc` is the default exclude file (a dpkg
conffile, so local edits survive upgrades). It excludes editor backups and
generated or cache state. It ends with a **commented-out secrets block**
(SSH host keys, `/etc/ssl/private`, `/etc/shadow`, ...): by default these
files are backed up, for restore fidelity and parity with rpm-find-changes.
Uncomment lines to keep them out of the backup (and out of extract's git
history). This can't catch secrets embedded in ordinary config files.

## Build

```
scripts/build_all.sh        # static binary in bin/deb-find-changes
go test ./...
```

## Packaging

```
scripts/deb_tarball.sh      # dist/deb/deb-find-changes_<version>.tar.gz
```

Then, on the `repo` build machine:

```
tar xzf deb-find-changes_<version>.tar.gz
cd deb-find-changes-<version>
dpkg-buildpackage -b -us -uc
```

The `.deb` is amd64 only and zstd-compressed (Debian 12 / Ubuntu 22.04 or
newer). Releases are cut with `scripts/release.sh <version>` after adding a
`debian/changelog` stanza; it tags locally and never pushes.

## Licence

GPLv3; see `COPYING`.
