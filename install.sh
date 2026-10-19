#!/bin/sh
# Install canvas-cli from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/bubunyo/canvas-lms-cli/main/install.sh | sh
#
# Environment:
#   CANVAS_CLI_VERSION  release tag to install (default: latest), e.g. v0.1.0
#   INSTALL_DIR         where to put the binary (default: ~/.local/bin)
#
# Run from a checkout with --source to build and install with Go instead.
set -eu

REPO="bubunyo/canvas-lms-cli"
BIN="canvas-cli"
VERSION="${CANVAS_CLI_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

if [ "${1:-}" = "--source" ]; then
	command -v go >/dev/null 2>&1 || die "Go is not installed (https://go.dev/dl/)"
	[ -f go.mod ] || die "--source must be run from the repository root"
	say "Building from source with $(go version)…"
	go install -trimpath ./cmd/canvas-cli
	say "Installed $(go env GOPATH)/bin/$BIN"
	exit 0
fi

case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	MINGW* | MSYS* | CYGWIN*) os=windows ;;
	*) die "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
esac

ext="tar.gz"
exe=""
if [ "$os" = windows ]; then
	ext="zip"
	exe=".exe"
fi
asset="${BIN}_${os}_${arch}.${ext}"

if [ -n "${CANVAS_CLI_BASE_URL:-}" ]; then
	base="$CANVAS_CLI_BASE_URL" # for testing against a local mirror
elif [ "$VERSION" = latest ]; then
	base="https://github.com/$REPO/releases/latest/download"
else
	base="https://github.com/$REPO/releases/download/$VERSION"
fi

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		die "need curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "need sha256sum or shasum to verify the download"
	fi
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/canvas-cli.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

say "Downloading $asset ($VERSION)…"
fetch "$base/$asset" "$tmp/$asset" || die "download failed: $base/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"

want="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$want" ] || die "$asset not listed in checksums.txt"
got="$(sha256 "$tmp/$asset")"
[ "$want" = "$got" ] || die "checksum mismatch for $asset (want $want, got $got)"

mkdir -p "$tmp/x"
if [ "$ext" = zip ]; then
	unzip -q "$tmp/$asset" -d "$tmp/x"
else
	tar -xzf "$tmp/$asset" -C "$tmp/x"
fi

mkdir -p "$INSTALL_DIR" || die "cannot create $INSTALL_DIR (set INSTALL_DIR or use sudo)"
[ -w "$INSTALL_DIR" ] || die "$INSTALL_DIR is not writable (set INSTALL_DIR or re-run with sudo)"
cp "$tmp/x/$BIN$exe" "$INSTALL_DIR/$BIN$exe"
chmod 755 "$INSTALL_DIR/$BIN$exe"
if [ "$os" = darwin ]; then
	xattr -d com.apple.quarantine "$INSTALL_DIR/$BIN" 2>/dev/null || true
fi

say "Installed $("$INSTALL_DIR/$BIN$exe" --version) to $INSTALL_DIR/$BIN$exe"
case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*) say "Note: $INSTALL_DIR is not on your PATH. Add it, e.g.:  export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
