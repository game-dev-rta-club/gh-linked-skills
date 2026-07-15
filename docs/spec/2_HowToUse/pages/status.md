---
title: Checking synchronization health
updated: 2026-07-13
status: implemented
---

# Status

Stateは差分の方向、PULL/PUSH eligibilityは操作可能かを表す。

Manifestへ登録されたskillだけを表示する。

```bash
gh linked-skills status
```

出力例:

```text
SKILL          PATH                                STATE     PULL                              PUSH
shared-rules   .agents/skills/shared-rules         clean     eligible                          eligible
review-helper  .agents/skills/review-helper        pull      eligible                          ineligible (remote_changed)
local-notes    .agents/skills/local-notes          push      eligible                          eligible
idea-refine    .agents/skills/idea-refine          conflict  ineligible (unresolved_conflict)  ineligible (unresolved_conflict)
release-skill  .agents/skills/release-skill        clean     ineligible (fixed_source_ref)      ineligible (source_ref_read_only)
```

- `STATE`: localとsourceの差分方向
- `PULL` / `PUSH`: commandを実行できるか
- 括弧内: 実行できない理由

| State | 意味 | 操作 |
| --- | --- | --- |
| `clean` | 差分なし | なし |
| `pull` | source変更 | pull |
| `push` | local変更 | push |
| `conflict` | localとsourceの両方が変更 / 未解決箇所あり | 未解決箇所がfileにあれば編集。それ以外はpull |

Sourceのtree SHAがbaselineと異なれば、contentが同じでも`pull`を優先する。

| Reason | 対応 |
| --- | --- |
| `invalid_local_skill` | `SKILL.md`またはfrontmatterを修復 |
| `unsafe_local_path` | pathを修復 |
| `unsupported_local_file` | regular fileへ変更 |
| `unsupported_host` / `invalid_source_repository` | sourceを確認 |
| `unresolved_conflict` | file内の競合表示を解消 |
| `source_unavailable` | network/認証/sourceを確認 |
| `git_inventory_unknown` | Git状態を確認 |
| `untracked_files` | Git indexへ登録 |
| `ignored_files` | untrackedかつignoredのfileをtrackするかignore対象外へ変更 |
| `permission_unknown` | network/認証/repositoryを確認 |
| `repository_read_only` | pull専用で使用 |
| `remote_changed` | pullを先に実行 |
| `fixed_source_ref` | tag固定。変更時は別tagで再install |
| `source_ref_read_only` | tag固定。sourceへpushしない |
| `tag_moved` | 自動適用しない。同じtagを受け入れる場合だけ`--accept-moved-tag`で再install |

`ineligible`は実行不可、`unknown`は判定失敗。

`eligible`は事前条件を満たすという意味で、GitHubが実際のpushを受理する保証ではない。

JSON: `gh linked-skills status --json`

競合表示の編集: [[docs/spec/2_HowToUse/pages/resolve-conflicts|Conflict解決]]

内部: [[docs/spec/3_Functions/pages/operations/status|Status spec]]
