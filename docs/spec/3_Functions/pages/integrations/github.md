---
title: GitHub integration specification
updated: 2026-07-14
status: implemented
---

# GitHub

Source snapshot取得、publish、push権限判定をGitHub.comへ限定する。Repository作成は扱わない。

## GITHUB-001 Authentication and host

Hostは`github.com`固定。`go-gh`でtokenを取得する。保存済み認証に加え、`GH_TOKEN`など`gh`互換の環境変数を利用できる。APIへ`Cache-Control: no-cache`を付ける。

Git pushはtokenをBasic auth extra headerで渡す。traceを無効化し、token/Authorizationを`[REDACTED]`へ置換する。configへ保存しない。

User environmentとglobal/system Git configは継承する。専用HOME、config隔離、timeout、retryはない。

## GITHUB-002 Ref, discovery, tree, blob reads

Install/pull/pushのsource refはGit refs APIで解決する。Annotated tagはGit tags APIでcommitまでpeelする。Tag object SHAとcommit SHAを分けて返し、commit以外を指すrefは拒否する。

Trees API `recursive=1`とBlobs APIでsubtreeを読む。

Discoveryはsource refを1回だけcommit SHAへ解決し、repository treeを1回読む。候補path、namespace、skill tree SHAを返す。`--all`とname選択は同じ結果を使う。

拒否: path/SKILL.md不存在、不正frontmatter、truncated tree、unsupported mode/type/encoding。

Install/pull/pushのsnapshotはtreeとblobを読む。Statusは全source refとrepository permissionをGraphQLでまとめて読む。1 requestは最大32 refとし、超過分だけ分割する。Owner、repository、refはGraphQL variableで渡す。

Statusはcommit SHAがbaselineと異なるrepositoryだけtreeを読む。Tree SHAが変わったskillだけsnapshotを読み、nameを検証する。

GraphQLの一部だけ失敗した場合、error pathに対応するrefまたはpermissionだけをunknownにする。HTTP、decodeなどrequest全体の失敗は、そのrequestに含まれる項目をunknownにする。

Statusのtree responseは1 process内だけ共有する。Discovery treeがtruncatedの場合は一覧/name/`--all`を拒否する。Statusもtruncated treeを判定不能として拒否する。Exact path installは全体discoveryを使わない。

永続cache、pagination、size limit、timeout、retryはない。

GitHub APIの上限は適用される。Recursive treeは100,000 entryまたは7 MBまでで、`truncated=true`を拒否する。Blobは1件100 MBまで。詳細: [Trees API](https://docs.github.com/en/rest/git/trees) / [Blobs API](https://docs.github.com/en/rest/git/blobs)

## GITHUB-003 Push permission

StatusはGraphQLの`viewerPermission`をrepository単位で読む。`ADMIN`、`MAINTAIN`、`WRITE`は許可、`READ`、`TRIAGE`はread-only、取得失敗はpermission unknown。Statusではcloneしない。

Push実行前はfalseの場合にshallow clone + `git push --dry-run`で再確認する。

Known denialはread-only。network/clone/unknown failureはpermission unknown。

この判定はbranch protectionやrulesetを完全には表さない。最終的なpushはGitHubに拒否され得る。
