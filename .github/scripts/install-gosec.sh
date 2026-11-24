#!/usr/bin/env bash
set -euo pipefail

# Both scan jobs run on GitHub's linux/amd64 runner.
version=2.29.0
archive="gosec_${version}_linux_amd64.tar.gz"
sha256=6431b119741c1f4a50fdfcf94e782e16b9e642afc8c7fa9b5d39d48bf3003095
work="$(mktemp -d)"
trap 'rm -f -- "$work/$archive" "$work/gosec"; rmdir -- "$work"' EXIT
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
  --output "$work/$archive" "https://github.com/securego/gosec/releases/download/v$version/$archive"
(
  cd "$work"
  printf '%s  %s\n' "$sha256" "$archive" | sha256sum --check -
  tar -xzf "$archive" gosec
)
mkdir -p "$(go env GOPATH)/bin"
install -m 0755 "$work/gosec" "$(go env GOPATH)/bin/gosec"
