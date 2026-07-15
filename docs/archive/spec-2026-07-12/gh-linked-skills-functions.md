---
title: Linked Skills 機能一覧
updated: 2026-07-12
status: implemented
---

# 機能一覧

## Install

```bash
gh linked-skills install OWNER/REPO --path PATH --branch BRANCH
```

source subtreeのrelative path、content byte、`100644` / `100755`を保って`.agents/skills/<name>`へ配置する。`SKILL.md`はCodex互換の`name`と`description`を必須とする。symlink、submodule、特殊mode、Git LFS pointerは拒否する。

destinationが存在しない場合だけinstallする。同じsource、baseline、destination、on-disk snapshotが一致する再実行はno-opとし、それ以外は上書きしない。commandは`git add`やcommitを行わない。

## Status

```bash
gh linked-skills status [--json]
```

`.gh-linked-skills.json`にあるskillだけを検査する。比較対象はrelative path、raw byte、実行bitであり、YAMLの意味比較や正規化はしない。

| State | 意味 |
| --- | --- |
| `clean` | localとremoteがbaselineと一致 |
| `pull` | remoteだけが変更済み |
| `push` | localだけが変更済み |
| `conflict` | localとremoteの両方が変更済み、またはmarkerが残る |

remote subtree SHAがbaselineと異なる場合、内容が同じでも`pull`とする。push権限がないrepositoryはpull可能だがpush不可とし、local変更があれば警告する。

## Pull

```bash
gh linked-skills pull <name|project-relative-path>
```

全skill fileが親projectのGitで追跡済みの場合だけ実行する。localがcleanならremote snapshotへatomic置換する。両側変更時はsystem Gitのthree-way mergeを使い、text conflictをfileへ書く。binary conflict、modify/delete、file/directory collisionは変更せず停止する。

適用後にmanifestのcommit SHAとsubtree SHAを進める。manifest更新が失敗した場合は元directoryへrollbackする。remoteとlocalのbyteがすでに一致する場合はcontentを触らずbaselineだけ進める。

## Push

```bash
gh linked-skills push <name|project-relative-path>
```

次を満たす場合だけpushする。

- manifest管理済みである
- generated conflict markerがない
- local `SKILL.md`がCodex互換である
- local fileが`.gitignore`で除外されていない
- source branchへのpush権限がある
- remote subtree SHAがbaselineと一致する

temporary shallow cloneで対象subtreeだけを置換し、通常commitと通常pushを行う。remote raceはnon-fast-forwardとして拒否する。成功後にmanifestを進める。remote push後のmanifest更新だけ失敗した場合は再pushせず`pull`でbaselineを回復する。

## Workflow skill

```bash
gh linked-skills skills install --agent <codex|claude-code> --scope project
```

binary内蔵bundleをCodexは`.agents/skills/gh-linked-skills`、Claude Codeは`.claude/skills/gh-linked-skills`へ置く。versionとSHA-256をmanifestへ保存し、未変更の管理済みbundleだけを再実行で更新する。所有不明のdestinationやlocal変更は上書きしない。

## Related

- [[gh-linked-skills|概要]]
- [[gh-linked-skills-conflict-resolution|手動競合解決]]
- [[gh-linked-skills-implementation|実装方針]]
