#!/usr/bin/env bash
#
# bumblebee-install.sh — fetch a pinned, checksum-verified Bumblebee release
# and drop the binary into $BUMBLEBEE_PREFIX (default ~/.local/bin).
#
# Why pin + verify: per ADR-027, Bumblebee is a developer-endpoint scanner we
# want to run on a daily LaunchAgent. The whole tool's value proposition is
# "we'll tell you when something installed on your machine is bad," so the
# scanner itself must not be a supply-chain hole. Pinning to v0.1.1 with
# embedded SHA-256s captured at audit time means a re-uploaded release (or a
# MITM on the download) is detected, not laundered through.
#
# The four SHA-256s below were captured 2026-05-26 from two independent
# sources that matched:
#   1. github.com/perplexityai/bumblebee API release-asset digests
#   2. Upstream checksums.txt (sha256 27356f23…3394c5d2) attached to v0.1.1
#
# Usage:
#   scripts/bumblebee-install.sh             # install
#   scripts/bumblebee-install.sh --uninstall # remove the binary
#   scripts/bumblebee-install.sh --version   # print pin

set -euo pipefail

BUMBLEBEE_VERSION="v0.1.1"
BUMBLEBEE_SHA256_darwin_amd64="dd3b2573a974a2786f58215483420fa11cf62b39ff4032693f1440575940dc25"
BUMBLEBEE_SHA256_darwin_arm64="dc0a620e54e85f998c2280b0323763c342973a25eda475d8036d16b01820a2bf"
BUMBLEBEE_SHA256_linux_amd64="0ef1c56c85a67c10f7211883c0eb5fb902de705cc30bbca0bc6f4d60941547da"
BUMBLEBEE_SHA256_linux_arm64="41aad0296bb6c88e746b237ed32eaa3b9b93c48770a51cd66f736f8a4d07a7d1"

PREFIX="${BUMBLEBEE_PREFIX:-$HOME/.local/bin}"
ACTION="install"

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) ACTION="uninstall"; shift ;;
    --version)   echo "$BUMBLEBEE_VERSION"; exit 0 ;;
    --prefix)    PREFIX="$2"; shift 2 ;;
    -h|--help)   sed -n '3,21p' "$0"; exit 0 ;;
    *)           echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

target="$PREFIX/bumblebee"

if [ "$ACTION" = "uninstall" ]; then
  if [ -f "$target" ]; then
    rm -f "$target"
    echo "Removed $target"
  else
    echo "Not installed at $target (nothing to do)"
  fi
  exit 0
fi

# OS/arch resolution.
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch_raw" >&2; exit 1 ;;
esac
case "$os" in
  darwin|linux) ;;
  *) echo "unsupported OS: $os (Bumblebee supports macOS and Linux only)" >&2; exit 1 ;;
esac

# Look up the pinned digest for this os_arch.
sha_var="BUMBLEBEE_SHA256_${os}_${arch}"
expected_sha="${!sha_var:-}"
if [ -z "$expected_sha" ]; then
  echo "no pinned checksum for ${os}_${arch}" >&2
  exit 1
fi

asset="bumblebee_${BUMBLEBEE_VERSION#v}_${os}_${arch}.tar.gz"
url="https://github.com/perplexityai/bumblebee/releases/download/${BUMBLEBEE_VERSION}/${asset}"

mkdir -p "$PREFIX"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "→ Downloading $asset ($BUMBLEBEE_VERSION)..."
curl -fsSL -o "$tmpdir/$asset" "$url"

# Verify checksum BEFORE extracting. Untrusted tarballs are not opened until
# their digest matches the pin captured at audit time.
echo "→ Verifying SHA-256..."
actual_sha="$(shasum -a 256 "$tmpdir/$asset" | awk '{print $1}')"
if [ "$actual_sha" != "$expected_sha" ]; then
  echo "CHECKSUM MISMATCH for $asset" >&2
  echo "  expected: $expected_sha" >&2
  echo "  actual:   $actual_sha" >&2
  echo "Refusing to install. Investigate before retrying." >&2
  exit 1
fi
echo "  ok ($actual_sha)"

echo "→ Extracting..."
tar -xzf "$tmpdir/$asset" -C "$tmpdir"

# The release tarball contains a top-level `bumblebee` binary.
if [ ! -f "$tmpdir/bumblebee" ]; then
  echo "expected ./bumblebee in $asset, not found" >&2
  ls -la "$tmpdir" >&2
  exit 1
fi

install -m 0755 "$tmpdir/bumblebee" "$target"

echo "→ Installed:"
"$target" --version 2>/dev/null || "$target" version 2>/dev/null || echo "  (binary at $target)"
echo "Bumblebee $BUMBLEBEE_VERSION → $target"
echo
echo "Next: add $PREFIX to PATH if it isn't already, then run \`make bumblebee-scan\`."
