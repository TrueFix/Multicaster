#!/bin/sh
# Released copies have their version and repository stamped by scripts/release.py.
set -eu
VERSION=${MULTICASTER_VERSION:-__VERSION__}
REPOSITORY=${MULTICASTER_REPOSITORY:-__REPOSITORY__}
INSTALL_DIR=${MULTICASTER_INSTALL_DIR:-"$HOME/.local/bin"}
case "$VERSION" in ''|*[!0-9A-Za-z.+-]*) printf '%s\n' 'Invalid version. Download install.sh from a published release.' >&2; exit 1;; esac
case "$REPOSITORY" in ''|*[!0-9A-Za-z._/-]*|*..*) printf '%s\n' 'Invalid release repository.' >&2; exit 1;; esac
case "$(uname -s)" in Linux) os=linux;; Darwin) os=macos;; *) printf '%s\n' 'Use install.ps1 or a Windows release binary.' >&2; exit 1;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) printf '%s\n' 'Supported architectures: amd64 and arm64.' >&2; exit 1;; esac
asset="multicaster-$os-$arch"
base="https://github.com/$REPOSITORY/releases/download/v$VERSION"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/multicaster-install.XXXXXX")
stage=''
trap 'rm -rf "$tmp"; if [ -n "$stage" ]; then rm -f "$stage"; fi' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
download() { if command -v curl >/dev/null 2>&1; then curl --fail --show-error --silent --location --proto '=https' --proto-redir '=https' "$1" -o "$2"; elif command -v wget >/dev/null 2>&1; then wget --https-only -q "$1" -O "$2"; else printf '%s\n' 'Install curl or wget first.' >&2; exit 1; fi; }
download "$base/$asset" "$tmp/$asset"
download "$base/SHA256SUMS" "$tmp/SHA256SUMS"
expected=''
while read -r sum name; do if [ "$name" = "$asset" ]; then expected=$sum; fi; done < "$tmp/SHA256SUMS"
case "$expected" in ''|*[!0-9a-f]*) printf '%s\n' 'Release checksum is missing or invalid.' >&2; exit 1;; esac
if [ "${#expected}" -ne 64 ]; then printf '%s\n' 'Invalid SHA-256 checksum.' >&2; exit 1; fi
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$asset"); elif command -v shasum >/dev/null 2>&1; then actual=$(shasum -a 256 "$tmp/$asset"); else printf '%s\n' 'Install sha256sum or shasum first.' >&2; exit 1; fi
actual=${actual%% *}
if [ "$actual" != "$expected" ]; then printf '%s\n' 'Checksum mismatch. Nothing was installed.' >&2; exit 1; fi
chmod 755 "$tmp/$asset"
if [ "$("$tmp/$asset" --version)" != "$VERSION" ]; then printf '%s\n' 'Binary version does not match the release.' >&2; exit 1; fi
mkdir -p "$INSTALL_DIR"
stage=$(mktemp "$INSTALL_DIR/.multicaster.XXXXXX")
cp "$tmp/$asset" "$stage"
chmod 755 "$stage"
mv -f "$stage" "$INSTALL_DIR/multicaster"
stage=''
printf 'Installed Multicaster %s to %s/multicaster\n' "$VERSION" "$INSTALL_DIR"
printf 'Run: "%s/multicaster"\n' "$INSTALL_DIR"
printf '%s\n' 'If Multicaster is already running, finish your stream before restarting it. Your settings are kept in the OS configuration directory.'
