---
title: Command reference
updated: 2026-07-14
status: implemented
---

# Commands

Command一覧と共通CLI規則を記載する。構文、動作、出力はリンク先を正本とする。

| Command | Purpose | Detail |
| --- | --- | --- |
| install | skillをprojectへ登録 | [[docs/spec/2_HowToUse/pages/install-skill\|SkillのInstall]] |
| publish | 未管理skillをsourceへ初回公開 | [[docs/spec/2_HowToUse/pages/publish\|Publish]] |
| status | 同期状態を確認 | [[docs/spec/2_HowToUse/pages/status\|Status]] |
| pull | sourceの変更を取得 | [[docs/spec/2_HowToUse/pages/pull\|Pull]] |
| push | localの変更を送信 | [[docs/spec/2_HowToUse/pages/push\|Push]] |

## Help

本体install後はCLIのhelpだけで基本操作を確認できる。

```bash
gh linked-skills --help
gh linked-skills help install
gh linked-skills install --help
gh linked-skills publish --help
```

Root helpは目的、Command一覧、基本例を表示する。Command helpは説明、構文、引数、flag、例を表示する。HelpはGitHub認証とGit projectを要求しない。

`-h`は`--help`と同じ。その他のshort flag、`--flag=value`、`--`、top-level `--version`はない。

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | success |
| `1` | operation failure |
| `2` | usage error |

競合を残したpullも`1`。表示と解決手順は[[docs/spec/2_HowToUse/pages/resolve-conflicts|Conflict解決]]を参照。

内部: [[docs/spec/3_Functions/pages/cli/runtime|CLI runtime]]
