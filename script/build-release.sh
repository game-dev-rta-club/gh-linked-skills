#!/usr/bin/env bash

set -euo pipefail

tag="${1:?release tag is required}"
mkdir -p dist

for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64; do
  os="${target%-*}"
  arch="${target#*-}"
  output="dist/gh-skill-linker_${tag}_${os}-${arch}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$output" .
done
