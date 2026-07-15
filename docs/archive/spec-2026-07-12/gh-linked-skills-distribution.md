---
title: Linked Skills 配布と対応範囲
updated: 2026-07-12
status: implemented
---

# 配布と対応範囲

## Extension

repository名、extension名、project名は`gh-linked-skills`に統一する。Go製precompiled GitHub CLI extensionとしてreleaseし、利用者側へGo runtimeを要求しない。

```bash
gh extension install game-dev-rta-club/gh-linked-skills
gh extension upgrade game-dev-rta-club/gh-linked-skills
```

GitHub CLI 2.96.0未満は処理前にupdateを要求する。古いversion向けcompatibility layerは持たない。

## Workflow skill

workflow skillはextension binaryへ埋め込み、追加downloadなしでinstallする。

```bash
gh linked-skills skills install --agent codex
gh linked-skills skills install --agent claude-code
```

project scopeだけを提供する。extension upgrade後に同じcommandを再実行すると、local変更のない管理済みbundleだけを更新する。

## Support

| 対象 | MVP |
| --- | --- |
| Host | GitHub.com |
| OS | macOS、Linux |
| Git | system Gitが必要 |
| Managed skill | Codex互換、project scope |
| Workflow adapter | Codex、Claude Code |

GHES、Windows、user scope、custom destinationは正式対応しない。

## Related

- [[gh-linked-skills|概要]]
- [[gh-linked-skills-implementation|実装方針]]
