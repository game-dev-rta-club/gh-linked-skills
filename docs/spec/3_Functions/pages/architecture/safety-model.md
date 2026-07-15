---
title: Safety model
updated: 2026-07-14
status: implemented
---

# Safety

不明な状態では上書きせず、利用者が確認できる情報を残す。保証はoperationごとに異なる。

- install: ownership不明のdestinationを上書きしない
- publish: 未管理skillだけを空remote pathへ追加する。一致済みpathは採用し、不一致は上書きしない
- pull: 新内容をstageし、localを再読してから入れ替える。manifest失敗時は元へ戻す
- push: source skillのtree SHAをclone後にも再確認し、normal pushだけを使う
- merge: text conflictはmarkerを残し、binary/構造conflictはworkspace変更前に停止する
- credential: repository configへ保存せず、logではredactする

Raw byteと実行可否を転送するが、新規fileのmodeはumaskの影響を受ける。Push/publish成功後のmanifest失敗はremoteを戻さない。

Process lock、journal、signal recoveryはない。詳細は[[docs/spec/3_Functions/pages/storage/transactions|Transactions]]。
