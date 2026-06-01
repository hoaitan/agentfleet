# Contributing to agentfleet

Thanks for your interest in improving agentfleet! This document explains how to
propose changes.

## Reporting issues

- **Bugs and feature requests:** open a GitHub issue using the provided
  templates.
- **Security vulnerabilities:** do **not** open a public issue. Follow
  [SECURITY.md](.github/SECURITY.md) and use private vulnerability reporting.

## Development setup

```bash
git clone https://github.com/hoaitan/agentfleet
cd agentfleet
go build ./...
go test ./...
```

## Before you open a pull request

Please make sure your change passes the same checks CI runs:

```bash
gofmt -l .            # should print nothing
go vet ./...
go test -race ./...
golangci-lint run     # if you have golangci-lint installed
govulncheck ./...     # checks for known vulnerabilities
```

## Pull request guidelines

- Keep PRs focused; one logical change per PR.
- Add or update tests for behavior changes.
- Update documentation (README, doc comments) where relevant.
- Write clear commit messages. Conventional prefixes (`feat:`, `fix:`,
  `deps:`, `ci:`, `docs:`) are appreciated.
- All PRs require review and passing status checks before merge.

## Coding conventions

- Standard Go style; code must be `gofmt`/`goimports` clean.
- Handle errors explicitly; avoid panics in library code.
- Be mindful of the security-sensitive areas (PTY handling, Unix sockets,
  task sources that execute commands). Validate and document any new surface.

By contributing, you agree that your contributions are licensed under the
project's [MIT License](LICENSE).
