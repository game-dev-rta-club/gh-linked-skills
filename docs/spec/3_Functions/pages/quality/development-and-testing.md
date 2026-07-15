---
title: Development and testing policy
updated: 2026-07-13
status: implemented
---

# Development

外部serviceをadapterへ分離し、通常testをlocalで完結させる。

Behavior変更はTDD:

1. Failing test
2. 最小実装
3. Simplify
4. HowTo/Spec更新

| Layer | Test |
| --- | --- |
| domain | pure unit |
| service | fake dependency |
| filesystem | `t.TempDir()` |
| Git | temporary repo/bare remote |
| GitHub API | test transport/server |
| CLI | injected dependency |

通常testはlive GitHubへ接続しない。

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Live E2Eはowned branch、canonical repo guard、cleanup verificationを必須とする。
