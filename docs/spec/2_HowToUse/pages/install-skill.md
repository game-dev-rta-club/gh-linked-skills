---
title: Installing a managed skill
updated: 2026-07-13
status: implemented
---

# SkillのInstall

Repository内のskillを探し、現在のprojectへ登録する。Installしたskillは必ずsource repositoryと紐づく。

前提: [[docs/spec/2_HowToUse/pages/install-extension|gh-linked-skillsをinstall済み]]、GitHub token、Git repository。

GitHub.comのtokenには、`gh auth login`の保存済み認証または`GH_TOKEN`などを使う。

`BRANCH`または固定snapshotの`TAG`を指定する。

```bash
gh linked-skills install OWNER/REPO --branch BRANCH
gh linked-skills install OWNER/REPO SKILL --branch BRANCH
gh linked-skills install OWNER/REPO PATH --branch BRANCH
gh linked-skills install OWNER/REPO --all --branch BRANCH
gh linked-skills install OWNER/REPO SKILL --tag TAG
```

`OWNER/REPO`と`--branch` / `--tag`のどちらか1つは必須。Local directoryやrepository未指定のinstall経路はない。`./skills`、`../skills`、`~/skills`は拒否する。`skills/foo`のような2 segmentはGitHubの`OWNER/REPO`としてのみ解釈し、localからは読まない。HTTPS URL、`.git` suffix、default branch推測、commit指定はない。

- `OWNER/REPO`: GitHub repository
- `SKILL`: discoveryで見つかったskill名。重複時は`namespace/name`
- `PATH`: skill directory、またはその`SKILL.md`。直接取得する
- `--all`: discoveryで見つかった全skill
- `BRANCH`: pull/push先
- `TAG`: pull/pushしない固定snapshot
- 配置先: `.agents/skills/<name>`

Selectorを省略すると、skill名とpathを一覧表示して終了する。

`PATH`は[Agent Skills specification](https://agentskills.io/specification)形式のskill directoryを指す。直下に`SKILL.md`があり、`name`と`description`を持つ。

```bash
gh linked-skills install obra/superpowers skills/brainstorming --branch main
gh linked-skills install obra/superpowers skills/brainstorming/SKILL.md --branch main
```

上の2つは同じsourceを表す。`scripts/`、`references/`、`assets/`など、directory内のfileもそのままinstallする。

Discovery対象:

- `skills/<name>/SKILL.md`
- `skills/<namespace>/<name>/SKILL.md`
- `<prefix>/skills/<name>/SKILL.md`
- `<prefix>/skills/<namespace>/<name>/SKILL.md`
- `plugins/<namespace>/skills/<name>/SKILL.md`
- `<name>/SKILL.md`

`.`から始まるdirectoryとrepository rootの`SKILL.md`は対象外。Exact pathならdiscovery対象外のskill directoryも指定できる。

`--all`は同じref commitから全skillを取得する。同じnameが複数ある場合はinstallせず終了する。全件を事前検証した後、skill単位でinstallする。途中失敗時は成功済みskillを残し、成功と失敗を表示する。

既存destinationは上書きしない。同一source/snapshotの再実行はno-op。

Manifestにはrepository、path、source ref、最後の同期時点を記録する。Extensionは親projectをcommitしないため、install後に記録する。

```bash
git add -- .agents/skills .gh-linked-skills.json
git commit
gh linked-skills status
```

`clean`ならinstall完了。

成功時の表示:

- `installed <name> at <path>`
- `<name> is already installed at <path>`
- `re-pinned <name> tag: <old> -> <new> (<old-ref-sha> -> <new-ref-sha>)`

内部: [[docs/spec/3_Functions/pages/operations/install|Install spec]]

Tagの固定運用と再install: [[docs/spec/2_HowToUse/pages/install-by-tag|Tag指定のInstall]]
