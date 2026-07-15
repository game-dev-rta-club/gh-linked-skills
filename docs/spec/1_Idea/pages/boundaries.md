---
title: Product boundaries
updated: 2026-07-13
status: implemented
---

# Boundaries

Project内のlocal skillと、GitHub repository内のsourceの同期を扱う。管理単位はprojectだけとする。

扱わないこと:

- 汎用Git clientやpackage registry
- user全体やsystem全体のskill管理
- 全projectへの一括配布
- 競合の自動判断
- GitHub権限の管理や昇格
- 強制的な履歴変更
