#!/bin/sh
set -eu

repository="dapi/docmost-openviking-sync"
binary="docmost-openviking-sync"
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
version="${VERSION:-latest}"

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

base="https://github.com/${repository}/releases"
if [ "$version" = "latest" ]; then
  asset_url="${base}/latest/download/${binary}_${os}_${arch}.tar.gz"
  checksums_url="${base}/latest/download/checksums.txt"
else
  asset_url="${base}/download/${version}/${binary}_${os}_${arch}.tar.gz"
  checksums_url="${base}/download/${version}/checksums.txt"
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM
archive="${binary}_${os}_${arch}.tar.gz"
curl -fsSL "$asset_url" -o "$tmp_dir/$archive"
curl -fsSL "$checksums_url" -o "$tmp_dir/checksums.txt"
expected=$(awk -v file="$archive" '$2 == file {print $1}' "$tmp_dir/checksums.txt")
if [ -z "$expected" ]; then echo "Checksum is missing for $archive" >&2; exit 1; fi
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp_dir/$archive" | awk '{print $1}'); else actual=$(shasum -a 256 "$tmp_dir/$archive" | awk '{print $1}'); fi
if [ "$actual" != "$expected" ]; then echo "Checksum mismatch for $archive" >&2; exit 1; fi
mkdir -p "$install_dir"
tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
install -m 0755 "$tmp_dir/$binary" "$install_dir/$binary"
echo "Installed $binary to $install_dir/$binary"
