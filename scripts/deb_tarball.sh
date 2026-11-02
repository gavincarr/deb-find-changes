#!/usr/bin/env bash
# Package the current source tree as a Debian-build tarball:
# pre-built static binary + debian/ + etc/ + man/ + COPYING + README, ready
# for `dpkg-buildpackage -b` on a repo machine.
#
# Usage: scripts/deb_tarball.sh
#
# The version is read from debian/changelog (the single source of truth
# established by scripts/release.sh). Output:
# dist/deb/deb-find-changes_<version>.tar.gz, holding a top-level
# deb-find-changes-<version>/ directory.
#
# Workflow on the repo/build machine:
#   tar xzf deb-find-changes_<version>.tar.gz
#   cd deb-find-changes-<version>
#   dpkg-buildpackage -b -us -uc

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null && pwd)"
cd "$ROOT"

if ! command -v dpkg-parsechangelog >/dev/null; then
    echo "error: dpkg-parsechangelog not found (install dpkg-dev)" >&2
    exit 1
fi

VERSION="$(dpkg-parsechangelog -l debian/changelog -SVersion)"
PKG="deb-find-changes"
STAGE_NAME="${PKG}-${VERSION}"
STAGE_DIR="dist/deb/${STAGE_NAME}"
TARBALL="dist/deb/${PKG}_${VERSION}.tar.gz"

echo "+ building binaries (CGO_ENABLED=0 via scripts/build_all.sh)"
scripts/build_all.sh

echo "+ staging $STAGE_DIR"
rm -rf "$STAGE_DIR"
mkdir -p "$STAGE_DIR/bin"

# Copy the binary then strip the copy (leave the dev bin/ tree intact with
# its debug info -- the stripped artifact only lives inside the tarball).
cp "bin/${PKG}" "$STAGE_DIR/bin/${PKG}"
strip --strip-unneeded "$STAGE_DIR/bin/${PKG}"

# Packaging metadata, shipped config and man page, plus user-facing docs.
cp -r debian etc man "$STAGE_DIR/"
cp COPYING README.md "$STAGE_DIR/"

echo "+ creating $TARBALL"
tar -C dist/deb -czf "$TARBALL" "$STAGE_NAME"

echo
echo "Tarball: $TARBALL"
echo "Staged tree:"
ls -lh "$STAGE_DIR/bin"
echo
echo "On the build machine:"
echo "  tar xzf $(basename "$TARBALL")"
echo "  cd ${STAGE_NAME}"
echo "  dpkg-buildpackage -b -us -uc"
