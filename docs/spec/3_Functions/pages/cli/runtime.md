---
title: CLI runtime specification
updated: 2026-07-14
status: implemented
---

# CLI runtime

公開構文と出力は[[docs/spec/2_HowToUse/pages/command-reference|Command reference]]、ここではdispatchとpreflightを扱う。

## CLI-001 Dispatch and parsing

Commands: `install`、`publish`、`status`、`pull`、`push`。

- install: `OWNER/REPO [SKILL|PATH | --all] (--branch BRANCH | --tag TAG) [--accept-moved-tag]`
- publish: `OWNER/REPO SKILL --branch BRANCH`
- status: flagなし、または`--json`だけ
- pull/push: selector 1件

Install/publishのrepositoryは`OWNER/REPO`形式をCLI境界で検証する。Repositoryなし、`.git` suffix、3 segment以上はusage error。2 segmentの値は常にGitHub repositoryとして扱う。Installのselectorなしはdiscoveryだけを行う。

Helpは`--help`、`-h`、`help [command]`。Command引数内のhelp flagは位置を問わずhelpを優先する。Helpとmalformed argumentはdependencyを作らない。公開表示は[[docs/spec/2_HowToUse/pages/command-reference|Command reference]]を正本とする。

その他のshort flag、`--flag=value`、`--`は非対応。

## CLI-002 Exit mapping

success=`0`、operation failure=`1`、usage=`2`。marker適用済みpullも`1`。

## CLI-003 Preflight implementation

1. `auth.TokenForHost("github.com")`
2. `gh --version`の終了成功。出力はparseしない
3. `git rev-parse --show-toplevel`

Argument検証後にpreflightを行う。Usage errorとhelpはtoken不足より先に返す。

## CLI-004 Status rendering

path順にsortし、table/JSONへ出す。JSONはHTML escape無効。state/reasonはstatus serviceが算出する。

## CLI-005 Mutation rendering

Resultをsuccess/no-op messageへ変換する。Render failureはexit codeへ反映しない。

Conflict pullはstdoutを空にし、project-relative conflict pathを順に`CONFLICT (content)`としてstderrへ出す。続けて`status`と、`STATE=push`の場合だけ実行する`push`を案内し、exit `1`。
