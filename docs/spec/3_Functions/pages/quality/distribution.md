---
title: Distribution specification
updated: 2026-07-13
status: implemented
---

# Distribution

## DIST-001 Release artifacts

`v*` tagでtest/vet後、`cli/gh-extension-precompile@v2`を実行する。

- darwin-amd64
- darwin-arm64
- linux-amd64
- linux-arm64

CGO無効、`-trimpath -s -w`。Windows artifactなし。Release jobはUbuntu、`contents: write`。
