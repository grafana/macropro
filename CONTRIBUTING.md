# Contributing to macropro

Thank you for your interest in contributing to `macropro`. Pull requests, bug reports, and suggestions are all welcome.

## Reporting bugs

Before opening a new issue, please search [existing issues](https://github.com/grafana/macropro/issues) to see if the problem has already been reported.

When filing a bug report, include:

- The version of `macropro` in use.
- A minimal reproduction — ideally a failing test case.
- The Go version and OS.

## Reporting security vulnerabilities

Do **not** open a public issue for security vulnerabilities. Follow the process in [SECURITY.md](SECURITY.md) instead.

## Development

`macropro` has no external dependencies. After cloning:

```sh
go build ./...
go test -race ./...
go vet ./...
```

Benchmarks live in `benchmark_test.go`:

```sh
go test -bench=. -benchmem ./...
```

## Commit messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org/). PR titles are validated by CI, and [release-please](https://github.com/googleapis/release-please) uses commit prefixes to generate the changelog and pick the next version:

- `feat:` — new feature (minor bump pre-1.0, otherwise minor)
- `fix:` — bug fix (patch bump)
- `perf:` / `refactor:` / `docs:` / `test:` / `ci:` / `chore:` — no version bump on their own

A trailing `!` or a `BREAKING CHANGE:` footer triggers a major bump.

Please do **not** add AI co-author trailers (`Co-Authored-By: Claude`, `Assisted-by:`, `Co-developed-by:`, etc.) to commits or PRs.

## Pull requests

1. Open a PR against `main`.
2. Ensure `go test -race ./...`, `go vet ./...`, and `golangci-lint run` all pass.
3. The PR title must follow Conventional Commits (CI enforces this).
4. Keep changes focused — unrelated cleanups should go in separate PRs.

## Releases

Releases are cut automatically by [release-please](https://github.com/googleapis/release-please). Merging a release PR tags the new version and publishes a GitHub release; no manual tagging is required.
