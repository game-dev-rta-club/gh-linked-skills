---
title: Goals
updated: 2026-07-13
status: implemented
---

# Goal

Skillは、実際に使うprojectの中で改善される。

改善がlocalだけに残ると、別のprojectでは再利用できない。一方、sourceだけを編集する運用では、使用中に得た知見を戻しにくい。

Localで使いながら改善し、sourceへ戻し、次のprojectが受け取れる循環を作る。

利用者は状態を確認し、sourceの変更を取り込み、localで改善し、必要ならsourceへ戻す。この循環を短い操作で続けられる状態を目指す。

実現したい状態:

- projectごとにskillを使いながら改善できる
- localとsourceの状態を同期前に確認できる
- 変更が衝突したときは、内容を見て人が解決できる
- 書き込み可能なsourceへ改善を戻せる
- read-only sourceも安全に利用できる
