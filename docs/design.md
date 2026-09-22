# design

## Goal

A complete Go reproduction of git/contrib/diff-highlight (Perl): a
streaming filter that reads a unified diff, highlights only the
intra-line differences of `-`/`+` line pairs, and writes the result.
Input and output match the oracle (output is identical to input except
for the occasional highlight).

## Policy

- Minimal dependencies. Standard library first; minimal external deps
  (e.g. `golang.org/x/sys`) are allowed when needed, e.g. for Windows
  support.
- Must work on Windows (no Linux/macOS-only code).
- Go 1.27 baseline (the `go` directive in go.mod is also 1.27).
- `go fix` / `gofmt` / `go vet` must always pass.
- `package main` stays at the top level so that
  `go install github.com/podhmo/diff-highlight@latest` works (keep the
  location of the existing empty `main.go`). Internals may be split into
  `internal/` packages, but no public API is exposed.

## Structure

- `main.go`: reads stdin line by line, feeds the state machine, writes
  to stdout through a `bufio.Writer`.
- If core logic moves into separate files / `internal/`, keep function
  names and responsibilities aligned with the oracle's `DiffHighlight.pm`
  (`handle_line` / `flush` / `show_hunk` / `highlight_pair` /
  `highlight_line` / `color_config`) so the port stays easy to trace.
- Color config is read at runtime via
  `git config --type=color --get-regexp '^color\.diff-highlight\.'`.
  Defaults must still work when git is absent or fails.

## Testing

- Port the cases from the oracle's `t9400-diff-highlight.sh` into golden
  tests. Details in `docs/tasks.md`.
