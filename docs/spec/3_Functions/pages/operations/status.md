---
title: Status reference
updated: 2026-07-14
status: implemented
---

# Status

Local、source、manifestのtree identityからstateを計算し、pull/push可能性を判定する。

## STAT-001 Managed inventory and states

Manifest `skills`だけを対象にする。base=manifest `treeSHA`、current=source refのskill tree SHA、local=destinationから計算したGit tree SHA。

1回のstatus内では、全refとpush permissionをまとめて確認する。Git inventoryもproject全体で各1回読む。永続cacheは持たない。Local判定ではbaseline snapshotをdownloadしない。

Tag sourceはpullを`fixed_source_ref`、pushを`source_ref_read_only`とする。Remote `refSHA`がbaselineと違う場合、pull reasonは`tag_moved`。Permission確認は行わない。

| Local | Remote | State |
| --- | --- | --- |
| same | same | `clean` |
| changed | same | `push` |
| same | changed | `pull` |
| changed | changed | `conflict` |

Markerがあればremoteを読まず`conflict`。Current commit SHAがbaselineと同じならrepository treeを読まない。異なる場合だけrepository treeをrepository/commit単位で読み、current tree SHAを比較する。内容の意味比較は行わない。

## STAT-002 Eligibility and reason precedence

Path/file/source不正とmarkerは早期終了する。その後、local frontmatterとGit inventoryを確認し、local/current tree SHAからstateを算出する。

Validation failureと`source_unavailable`はstate=`null`。markerはstate=`conflict`。Git/permission failureでは比較済みstateを保持する。

Frontmatter不正またはmanaged name不一致でもpull state/eligibilityは算出し、pushだけ`invalid_local_skill`にする。Changed currentのname不一致は`source_unavailable`。Push reasonはfrontmatter → permission → `remote_changed` → `ignored_files`。先のnon-eligible reasonを上書きしない。

`eligible`はserviceの事前条件を満たすことだけを示す。GitHubのbranch protectionやrulesetによる最終結果は含まない。
