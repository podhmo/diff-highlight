# AGENTS.md

Go port of git/contrib/diff-highlight (Perl). Oracle: `oracle/`.
Detailed spec: `docs/technique.md`. Design policy: `docs/design.md`.
Task list: `docs/tasks.md`.

## Policy

- Go 1.27, minimal dependencies (standard library first), Windows is a
  supported target.
- `package main` stays at the top level
  (`go install github.com/podhmo/diff-highlight@latest`).
- Keep `go fix` / `gofmt` / `go vet` / `go test ./...` passing at all
  times.

## Recording bugs

- Record any bug or suspicious behavior you notice under `docs/bugs/`,
  **regardless of the task at hand**: one file per bug, include a
  reproducing input when available.
- Distinguish known limitations inherited from the oracle from real bugs
  found in the Go port.
