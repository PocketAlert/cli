#!/bin/sh
# Pocket Alert CLI installer.
#
#   curl -fsSL https://raw.githubusercontent.com/PocketAlert/cli/main/install.sh | sh
#
# Environment:
#   POCKETALERT_VERSION      tag to install, e.g. v2.0.0 (default: latest)
#   POCKETALERT_INSTALL_DIR  target directory (default: /usr/local/bin, or
#                            ~/.local/bin when /usr/local/bin is not writable)
set -eu

REPO="PocketAlert/cli"
BINARY="pocketalert"

say() { printf '%s\n' "$*" >&2; }
fail() { say "error: $*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "'$1' is required"; }

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		fail "curl or wget is required"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "unsupported OS $(uname -s); download a release from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture $(uname -m)" ;;
esac

need tar
version="${POCKETALERT_VERSION:-latest}"
if [ "$version" = "latest" ]; then
	base="https://github.com/$REPO/releases/latest/download"
else
	base="https://github.com/$REPO/releases/download/$version"
fi
archive="${BINARY}_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

say "Downloading $archive ($version)…"
download "$base/$archive" "$tmp/$archive" || fail "download failed: $base/$archive"
download "$base/checksums.txt" "$tmp/checksums.txt" || fail "could not fetch checksums.txt"

expected="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$expected" ] || fail "$archive is missing from checksums.txt"
[ "$(sha256 "$tmp/$archive")" = "$expected" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY"

dir="${POCKETALERT_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
	if [ -w /usr/local/bin ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
fi
mkdir -p "$dir"
install -m 0755 "$tmp/$BINARY" "$dir/$BINARY" 2>/dev/null || {
	cp "$tmp/$BINARY" "$dir/$BINARY"
	chmod 0755 "$dir/$BINARY"
}

say "Installed $("$dir/$BINARY" version) to $dir/$BINARY"
case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "note: $dir is not in your PATH — add it to your shell profile" ;;
esac
say ""
say "Next: $BINARY auth"
