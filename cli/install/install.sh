#!/bin/sh
# Installs the whisk CLI from its GitHub releases (CLI.md §2):
#   curl -fsSL https://github.com/Whisk-Hosting/cli/releases/latest/download/install.sh | sh
# Linux and macOS, amd64 and arm64; on Windows (Git Bash, MSYS, Cygwin) it hands over to
# install.ps1. WHISK_INSTALL_DIR sets where it goes (default ~/.local/bin).
set -eu
releases="https://github.com/Whisk-Hosting/cli/releases"
ps1="$releases/latest/download/install.ps1"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  mingw*|msys*|cygwin*)
    ps="powershell -NoProfile -ExecutionPolicy Bypass -Command \"irm $ps1 | iex\""
    if command -v powershell.exe >/dev/null 2>&1; then
      echo "whisk: this is Windows; installing with PowerShell: $ps"
      exec powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "irm $ps1 | iex"
    fi
    echo "whisk: this is Windows; install with PowerShell instead: $ps" >&2
    exit 1;;
  linux|darwin) ;;
  *) echo "whisk: unsupported OS $os" >&2; exit 1;;
esac
arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo "whisk: unsupported architecture $arch" >&2; exit 1;; esac
name="whisk_${os}_${arch}"
# One release's files throughout: the version the latest release names, then that release.
version=$(curl -fsSL --max-time 60 "$releases/latest/download/VERSION" | tr -d '[:space:]')
case "$version" in ""|*[!A-Za-z0-9._-]*) echo "whisk: could not read the latest version from $releases" >&2; exit 1;; esac
base="$releases/download/$version"
dir="${WHISK_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dir"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fsSL --max-time 300 "$base/$name" -o "$tmp"
want=$(curl -fsSL --max-time 60 "$base/SHA256SUMS" | grep " $name\$" | cut -d' ' -f1 || true)
[ -n "$want" ] || { echo "whisk: SHA256SUMS has no entry for $name; not installing" >&2; exit 1; }
got=$( (sha256sum "$tmp" 2>/dev/null || shasum -a 256 "$tmp") | cut -d' ' -f1)
[ "$got" = "$want" ] || { echo "whisk: checksum mismatch for $name" >&2; exit 1; }
install -m 0755 "$tmp" "$dir/whisk"
echo "whisk $version installed to $dir/whisk"
# A shell that has not got the folder on its PATH runs whisk by its full path, so the next step
# names it in full rather than failing with "command not found".
case ":$PATH:" in
  *":$dir:"*) echo "next: whisk login" ;;
  *) echo "$dir is not on your PATH: run $dir/whisk, or add it, for example: export PATH=\"$dir:\$PATH\""
     echo "next: $dir/whisk login" ;;
esac
