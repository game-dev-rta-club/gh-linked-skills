---
title: Push reference
updated: 2026-07-13
status: implemented
---

# Push

Tag sourceはlocal検証、permission確認、remote操作前に`source_ref_read_only`で拒否する。

Local snapshotを同じsource branchへ通常commitとして戻す。

## PUSH-001 Eligibility enforcement

再検査:

- selectorが一意
- pathがproject内、symlinkなし
- regular local files
- markerなし
- validでmanaged nameと一致する`SKILL.md`
- trackedまたはuntracked non-ignored
- push権限あり
- current source skill tree=baseline

Tracked fileとuntracked non-ignoredはpush可。Tracked fileはignore規則に一致していても対象になる。Git indexへの新規登録は不要。

Permission判定はrepository levelの事前確認。Branch protectionやrulesetによる実際のpush拒否はPUSH-003でerrorになる。

## PUSH-002 No-op

Local=currentならcommitしない。Tree同一でcommit SHAだけ進んだ場合はmanifestを更新する。

## PUSH-003 Remote mutation

`--branch --single-branch --no-tags --depth 1`でclone。`HEAD:<sourcePath>`がexpected tree SHAか再確認し、対象subtreeだけ置換する。

`0644/0755`で書き、`git add -A -- <sourcePath>`。差分なしならcommitしない。

- author: `gh-linked-skills <gh-linked-skills@users.noreply.github.com>`
- message: `chore(skill): sync <skill-name>`
- refspec: `HEAD:refs/heads/<branch>`

Forceしない。Non-fast-forwardは`remote changed`。Push後のmanifest失敗はremoteを戻さず、pullでreconcileさせる。
