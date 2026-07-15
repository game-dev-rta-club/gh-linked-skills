---
title: Pull reference
updated: 2026-07-13
status: implemented
---

# Pull

Tag sourceはlocal/remote読取前に`fixed_source_ref`で拒否する。

Current sourceをlocalへ適用し、必要ならthree-way mergeする。

## PULL-001 Selection and prerequisites

Nameまたはproject-relative destinationで一意に選ぶ。0件/複数件はerror。

全local fileをtracked必須とする。Manifestのtracked状態は見ない。Marker、unsafe path、invalid source、managed nameと異なるsource、remote failureでは変更しない。

## PULL-002 No-op and baseline-only update

Local=currentならcontentを変えない。SHAも同じならno-op。SHAだけ進んだ場合はbaselineを更新し`Changed=true`。

## PULL-003 Clean and merged apply

Local=baseならcurrentへ置換。それ以外はthree-way mergeする。

同じparentへstageし、targetをbackupへrenameする。Target再読が開始時snapshotと違えばrollbackし、`workspace changed during pull`。

Activate後にbaselineを更新。Manifest失敗はoriginalへ戻す。rollback失敗はbackup pathを返し、transactionを残す。

Backup削除だけ失敗した場合もerror。ただしnew content/baselineは有効。Marker生成時もbaselineを進める。

Text conflictはproject-relative pathをsort済み`ConflictPaths`として返す。CLIはfileを列挙してexit `1`。
