---
title: Linked Skills
updated: 2026-07-12
status: implemented
tags:
  - gh-extension
  - linked-skills
---

# Linked Skills

> [!SUMMARY]
> 複数projectで育てるskillを、sourceのbyteを変えずにinstallし、安全にpull / pushできるGitHub CLI extension。

## Value

skillを「installして使う成果物」ではなく「複数projectから改善が戻る共有source」として扱う。利用者が覚える基本操作は`pull → 編集 → push`とし、remote更新、権限不足、競合をcommandが先に検出する。

## Product boundary

- extension本体は`gh extension`で配布する
- GitHub.com、macOS、Linux、project scopeを正式対応する
- managed skillは`.agents/skills/<name>`へ置く
- source情報はproject rootの`.gh-linked-skills.json`へ置く
- skill contentへtracking metadataを追加しない
- Git transport、認証、mergeは`gh`、GitHub API、system Gitを使う
- PR作成、force push、user scope、GHES、WindowsはMVP対象外とする

既存skillや利用者dataのmigrationは実装しない。未リリース段階で管理済みskillが存在しないため、旧prototypeを直接置き換える。

## User flow

```bash
gh linked-skills install OWNER/REPO --path skills/example --branch main
git add .agents/skills/example .gh-linked-skills.json
git commit
gh linked-skills status
gh linked-skills pull example
# edit the skill
gh linked-skills push example
```

操作と状態は[[gh-linked-skills-functions|機能一覧]]、競合時の手順は[[gh-linked-skills-conflict-resolution|手動競合解決]]を参照する。

## Pages

| Page | 内容 |
| --- | --- |
| [[gh-linked-skills-functions|機能一覧]] | command interface、状態、拒否条件 |
| [[gh-linked-skills-implementation|実装方針]] | component、data、transaction、test |
| [[gh-linked-skills-distribution|配布と対応範囲]] | extensionとworkflow skillのinstall |
| [[gh-linked-skills-conflict-resolution|手動競合解決]] | markerを使う解決flow |
