---
title: Snapshot specification
updated: 2026-07-13
status: implemented
---

# Snapshots

Skillを比較・転送する最小単位を定義する。

## SNAP-001 Snapshot identity

完全一致対象:

- relative path集合
- raw byte
- 実行可否

YAML意味、改行変換、timestamp、directoryは比較しない。Any execute bitでexecutable。

## SNAP-002 Supported entries

Remoteは`100644/100755` blobのみ。Localはregular fileのみ。Symlink、submodule、特殊mode/file、truncated treeを拒否。

Root直下にregular `SKILL.md`必須。Remote readとpush前にfrontmatterを検証する。

- name: ASCII lower kebab-case、1〜64 byte
- description: trim後non-empty、元UTF-8で1024 byte以下
- LF/CRLF対応
- 他fieldは保持

Installed destinationはnameから決まるため、localではnameとparent directoryが一致する。Managed nameは同期後も固定し、status/pushはlocal、status/pullはsourceの不一致を拒否する。Source pathのbasename一致は検証しない。

Descriptionの上限はbyte数で判定するため、multibyte textでは[Agent Skills specification](https://agentskills.io/specification)より厳しい。

Statusはpush eligibilityのためlocal frontmatterを検証するが、pull eligibilityは維持する。Pull開始時は再検証しない。LFS pointer拒否は新規installだけ。

## SNAP-003 Write mode and umask boundary

Write modeは`0644/0755`。作成後chmodしないためumask依存。Execute bitを落とすumaskでは`100755`を再現できない。[Go `os.WriteFile`](https://pkg.go.dev/os#WriteFile)も作成modeはumask適用前と定義する。

Manifestだけ明示的に`chmod 0644`。
