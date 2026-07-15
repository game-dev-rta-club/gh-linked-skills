---
title: Transaction specification
updated: 2026-07-14
status: implemented
---

# Transactions

Local mutationは可能な範囲でrollbackする。Remote pushはrollbackしない。Process中断からの自動復旧は扱わない。

## TXN-001 Interruption and locking boundary

Process lock、journal、startup recovery、signal handlerはない。SIGINT等ではtemporary directoryが残り得る。

Pullは2回のrename間で終了するとtargetが消え、backupだけ残り得る。自動復旧しない。

| Operation | Manifest失敗時 |
| --- | --- |
| install | target削除 |
| install `--all` | 失敗したtargetを削除。成功済みskillは維持 |
| pull | original復元 |
| push | remoteを戻さない |
| publish | remoteを戻さない。同じcommandでmanifest登録を再試行 |
| manifest | fsync後、同一directoryでrename |

Rollback失敗時はbackup pathをerrorへ含め、transactionを残す。
