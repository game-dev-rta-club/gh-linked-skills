---
title: Filesystem specification
updated: 2026-07-13
status: implemented
---

# Filesystem

書込みをproject内へ制限し、symlinkとpath collisionの境界を定義する。

## PATH-001 Filesystem containment

Root/targetをabsolute clean pathにする。Root外、symlink、途中のnon-directoryを拒否。不存在component以降は検査しない。

Remote pathはabsolute、backslash、`.`、`..`、non-canonicalを拒否。

## PATH-002 Existing-install containment gap

新規install、status、pull、pushはcontainmentを検査する。

既存managed installの再実行だけ、destinationを先に読む。親をsymlinkへ変更された場合、project外を読み得る。書込みはしない。

## PATH-003 Case and Unicode boundary

Case folding/Unicode normalization collisionを検査しない。該当filesystemでは衝突、順序依存、errorになり得る。Map順は未定義。
