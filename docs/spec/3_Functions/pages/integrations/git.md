---
title: Git integration specification
updated: 2026-07-14
status: implemented
---

# Git

Parent projectのinventory取得と、source repositoryの一時操作にsystem Gitを使う。

## GIT-001 Project inventory

Rootは`git rev-parse --show-toplevel`。worktree外は失敗する。

- pull: `git ls-files --cached -- .agents/skills`
- push: `git ls-files --cached --others --exclude-standard -- .agents/skills`

Statusでは上記を各1回だけ実行し、全managed skillの判定に再利用する。

Tracked fileはignore規則にかかわらずpush対象。Untracked non-ignoredもpushできる。Untrackedかつignoredのfileだけpush不可。

Parentのclean/staged/manifest状態は検査しない。Parentでadd/commit/stashしない。

その他:

- install/pull/pushのlow-level ref fallback: `git ls-remote --exit-code`
- merge: `git merge-file --diff3`
- push commandのpermission: repository APIがpush不可を返した場合だけ、shallow clone + `git push --dry-run`
- push: shallow clone + scoped add + normal push
- publish: ref一覧を確認し、既存branchはshallow clone、refなしrepositoryは指定branchを初期化。Scoped add + normal push

Statusのref/permission確認はGitHub GraphQL APIだけを使い、cloneしない。

Branchはpush/publish直前に[`git check-ref-format --branch`](https://git-scm.com/docs/git-check-ref-format)でも検証する。
