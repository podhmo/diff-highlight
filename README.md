# diff-highlight

go version of https://github.com/git/git/tree/master/contrib/diff-highlight

A streaming filter for unified diffs: it highlights only the intra-line
differences of `-`/`+` line pairs. Output is identical to the input
except for the added ANSI highlights.

## Usage

```console
$ go install github.com/podhmo/diff-highlight@latest
$ git log -p --color | diff-highlight
```

Colors can be tuned via `git config` under `color.diff-highlight.*`
(`oldNormal`, `oldHighlight`, `oldReset`, and the `new*` variants),
same as the Perl original. Defaults work even when `git` is absent.

## Development

```console
$ go test ./...   # unit tests, golden tests, oracle byte-diff harness
```

Layout: `main.go` (I/O + line state machine), `highlight.go` (pair
highlighting + color config). `oracle/` keeps the upstream Perl source
used by the oracle-diff test harness. See `docs/` for the spec.
