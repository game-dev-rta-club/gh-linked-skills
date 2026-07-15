---
title: Manifest specification
updated: 2026-07-14
status: implemented
---

# Manifest

Source identityと最後の同期点をproject rootへ永続化する。

## MAN-001 Location

Project rootの`.gh-linked-skills.json`が唯一の台帳。不存在時はschema v2の空document。Regular fileのみ。Symlink、unknown field、複数JSON、trailing data、不正schemaを拒否する。

## MAN-002 Schema version 2

```json
{
  "schemaVersion": 2,
  "skills": {
    "example": {
      "repository": "https://github.com/owner/repository.git",
      "sourcePath": "skills/example",
      "sourceRef": "refs/tags/v1.2.0",
      "refSHA": "<tag-object-or-commit-sha>",
      "commitSHA": "<peeled-commit-sha>",
      "treeSHA": "<skill-tree-sha>",
      "destination": ".agents/skills/example"
    }
  }
}
```

Schema v1は読取時にmemory上でv2へ変換する。Writeはv2だけを出力する。

## MAN-003 Validation

- name: lower kebab-case、1〜64文字
- repository: `https://github.com/<owner>/<repo>[.git]`
- path: relative canonical POSIX path
- sourceRef: `refs/heads/`または`refs/tags/`とvalid ref name
- SHA: 40/64 lowercase hex
- destination: `.agents/skills/<key>`

## MAN-004 Write and optimistic comparison

2-space JSON + newlineをtemporary fileへ書く。HTML escape無効、`chmod 0644`、file fsync、同一directory rename。Directory fsyncはしない。

Baseline更新は開始時entry=current entryの場合だけ。Publish追加は書込直前に開始時documentと再読documentを比較する。他entryは保持。不一致は`management file changed during operation`。

Process lockはないため、比較とrenameをまたぐinterprocess atomicityは保証しない。

Ref model: [[docs/spec/3_Functions/pages/architecture/source-reference|Source reference]]
