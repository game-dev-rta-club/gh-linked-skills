---
title: Architecture overview
updated: 2026-07-13
status: implemented
---

# Architecture

CLIがoperation serviceを呼び、domainと外部adapterを分離する。

## TECH-001 Runtime and build dependencies

- Go `1.25.0`
- `github.com/cli/go-gh/v2 v2.13.0`
- `gopkg.in/yaml.v3 v3.0.1`
- runtime: `gh`、Git、GitHub.com network

Node.js、Python、shell interpreterは不要。

```text
main / cli
  → install / status / pull / push
    → source / skill / syncstate / merge
    → manifest / workspace / githubapi / gitcli / command
```

Operationはinterface経由でadapterを使う。testではfakeへ置換する。

`status`、`pull`、`push`はmanifestを読み、source/baseline/localを比較する。`install`はsource snapshotを初期baselineとして登録する。

同期commandの変更対象は選択したskillだけ。

固定値:

- host: GitHub.com
- workspace: current Git worktree
- destination: `.agents/skills/<name>`
- source: repository + path + full Git ref
- parent commit: application外
