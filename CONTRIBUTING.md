# Contributing

Contributions that make synchronization safer, clearer, or easier to use are
welcome.

If you have found a security vulnerability, do not open a public issue. Follow
the private reporting process in [SECURITY.md](SECURITY.md).

## Propose a change

- For a typo, documentation fix, or small bug fix, open a pull request directly.
- For a new command, changed synchronization behavior, or file-format change,
  open an issue first so compatibility and failure recovery can be discussed.
- Keep refactoring separate from behavior changes.

## Local development

Requirements: Go 1.26.5 or later, GitHub CLI, and system Git.

```sh
git clone https://github.com/game-dev-rta-club/gh-skill-linker.git
cd gh-skill-linker
go mod verify
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

The scheduled Live E2E workflow writes only to a temporary owned branch and is
maintainer-operated. Contributors do not need to run it for ordinary pull
requests.

## Pull requests

Keep pull requests focused on one logical change. A pull request should:

- Explain the user-visible behavior and failure modes.
- Include tests for new behavior or a bug fix.
- Update README or detailed documentation when commands or formats change.
- Add a user-facing entry under `Unreleased` in `CHANGELOG.md` when applicable.
- Pass the test, race, vet, and build commands above.
- Avoid unrelated formatting or refactoring.

Conventional Commit prefixes such as `docs:`, `fix:`, `feat:`, and `test:` are
preferred for commit and pull-request titles.

Maintainers review contributions when available. A response, merge, or release
timeline is not guaranteed.

## Contribution license

No Contributor License Agreement or Developer Certificate of Origin is
required. Unless explicitly stated otherwise, contributions intentionally
submitted for inclusion in this repository are licensed under the repository's
[MIT License](LICENSE).
