---
title: Linked Skills 実装方針
updated: 2026-07-12
status: implemented
---

# 実装方針

## Components

| Component | 責務 |
| --- | --- |
| CLI | argument、exit code、表示 |
| manifest | schema検証、atomic JSON write、baseline更新 |
| GitHub API | private repositoryのref、tree、blob、permission読取 |
| workspace | raw snapshot読取、staging、rollback付き置換 |
| system Git | project inventory、diff3 merge、temporary clone、commit、push |

production codeは`gh skill` subprocessを実行しない。GitHub認証は`go-gh`と`gh auth`、push credentialは一時的なGit extra headerを使い、tokenをlogへ出さない。

## Manifest

project rootの`.gh-linked-skills.json`を唯一の管理情報とする。managed skillごとにrepository、source path、branch、destination、last synchronized commit SHA / subtree SHAを持つ。workflow skillはagent、destination、bundle version、SHA-256を持つ。

JSONはunknown fieldと不正path / branch / SHAを拒否し、temporary fileをsyncしてatomic renameする。skill contentとmanifestは親projectでcommitするが、extensionはstagingやcommitを暗黙に行わない。

## Mutation safety

installとpullは対象directoryを同じfilesystem内でstagingする。pullは現在directoryをrollback場所へ移し、expected snapshotとの一致を再確認してから新directoryをactivateする。manifest writeが失敗すれば元directoryを戻す。

pushはremote mutationが先になるためrollbackできない。manifest更新失敗を専用errorにし、再pushではなく`pull`によるreconcileを案内する。すべてのpathはproject root / skill rootからのescapeを拒否し、symlinkを辿らない。

## Tests

- pure test: state、manifest validation、raw byte/mode比較
- filesystem test: install、atomic replace、rollback、workflow bundle ownership
- local Git test: inventory、diff3、temporary bare repository push、race rejection
- HTTP test: GitHub tree/blob/ref/permission adapter
- private E2E: install、status、pull、push、conflict marker、手動解消

default testはnetwork、利用者credential、既存repositoryを使わない。変更ごとに`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...`を通す。

## Related

- [[gh-linked-skills|概要]]
- [[gh-linked-skills-functions|機能一覧]]
- [[gh-linked-skills-distribution|配布と対応範囲]]
