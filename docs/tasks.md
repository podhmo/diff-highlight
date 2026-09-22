# tasks

Tasks for a complete Go reproduction of git/contrib/diff-highlight.
Detailed spec: `docs/technique.md`. Policy/design: `docs/design.md`.
Oracle: `oracle/`.

## Implementation tasks

- [x] ANSI utilities
  - `COLOR` equivalent: regexp `\x1b\[[0-9;]*m`.
  - `BORING` equivalent: a color or whitespace.
  - `visibleWidth(s)`, `visibleSubstr(s, n)` (color sequences count as
    zero width; note: `visible_substr` *drops* leading colors, not keeps).
- [x] Tokenizer `splitLine(line) []string`
  - Split on the COLOR regexp; color parts are single tokens, everything
    else is one token per rune.
  - Perl's `utf8::decode` → `split //` → `utf8::encode` is equivalent to
    rune-wise splitting in Go; invalid UTF-8 falls back to per-byte
    tokens (`utf8.ValidString` decides for the whole line, like the
    oracle's whole-line decode).
- [x] `handleLine` state machine
  - Port the `--graph` commit-start detection regexp (the oracle's
    5-line `/x` pattern) verbatim, plus `$graph_indent` management.
  - `$in_hunk` transitions: starts at `@@ `, buffers `-`/`+`, flushes on
    anything else. Verified `@@@` (combined diff) does not match `@@ `.
  - Flush output on blank lines (streaming heuristic); the oracle's
    `/^$/` matches `""` and `"\n"` (not `"\r\n"`).
- [x] `showHunk`/`flush`
  - Empty side or mismatched line counts → passthrough. Same count →
    pair i-th with i-th.
  - Emit removed lines immediately, queue added lines for afterwards
    (order preservation).
- [x] `highlightPair`
  - Common prefix scan (color skipping + the leading `-`/`+` marker
    `seen_plusminus` handling).
  - Common suffix scan.
  - `isPairInteresting`: identical lines, or prefix/suffix of only boring
    tokens → no highlight (whole-line-highlight suppression).
- [x] `highlightLine`
  - Two modes: normal/highlight (strip existing colors, recolor three
    spans) and highlight/reset (keep existing colors, splice only around
    mid).
- [x] Color config `colorConfig`/`loadColorConfig`
  - Run `git config --type=color --get-regexp '^color\.diff-highlight\.'`,
    cache the result. Works with defaults when git is absent or fails
    (tested via `PATH=""`).
  - Defaults: `oldhighlight=\x1b[7m`, `oldreset=\x1b[27m`; new* falls
    back to old*.
- [x] `main`
  - Read stdin line by line into `handleLine`; flush at EOF.
  - Use a `bufio.Writer`; `Flush()` on blank input lines.
  - SIGPIPE: Go dies on EPIPE writes to stdout (fds 1/2), matching the
    oracle's `$SIG{PIPE} = 'DEFAULT'`. Covered by `TestSIGPIPE`.
- [x] Cross-platform (Windows support)
  - `os.DevNull` used for the `git config` stderr redirect (never
    hardcode `/dev/null`).
  - Decided to stay stdlib-only: no `golang.org/x/sys` dependency for
    virtual-terminal processing (the task left this open).
  - `\r\n` input covered by golden test (`crlf.in`). Runtime verification
    on Windows itself is still pending.

## Test tasks (golden tests based on the oracle)

Golden tests run right after the implementation tasks.

Port the `dh_test` cases from `oracle/t/t9400-diff-highlight.sh` into Go
tests. Test inputs can be "the hunk part from `@@` onwards" plus expected
ANSI output in testdata, or real git diffs (the latter adds a git
dependency — check CI first). Cover at least:

- [x] Highlights at beginning / end / middle of a line
- [x] No highlight when the whole line differs
- [x] Mismatched hunk line counts → no highlight (oracle treats this as
      expected failure; golden captures passthrough)
- [x] Multibyte UTF-8 treated as a single character
- [x] Combining code points (oracle marks expected failure: documented as
      a known limitation in `docs/bugs/oracle-limitations.md`)
- [x] `--graph` (plain / nested / graph with leading `-`)
- [x] Combined diffs pass through
- [x] Removed final newline (`\ No newline at end of file`)
- [x] Color config: set/reset mode and normal/highlight mode (real
      `git config` via `GIT_CONFIG_*` env in tests, injected cache in
      goldens, and the fallback)

## Verification tasks (golden test elaboration and bug hunting)

Run these last, after the implementation is done.

- [x] Elaborate the golden tests: edge cases beyond t9400
  - Empty input / newline-only input / non-diff input
  - Hunk ending at EOF (with and without trailing newline)
  - `\r\n` newlines, tabs, whitespace-only changes
  - Broken/truncated ANSI sequences, color-only lines
  - Multiple consecutive hunks, invalid UTF-8 byte sequences
  - (huge hunks not covered: left as future work if it matters)
- [x] Oracle diff harness
  - `TestOracleComparison` feeds the same input to the Perl oracle and
    the Go binary and byte-compares the output (all golden inputs, the
    env-config cases, and 40 deterministic fuzz inputs) — all identical.
- [x] Bug report
  - No divergences found; inherited oracle limitations classified under
    `docs/bugs/oracle-limitations.md`.

## Finishing

- [x] `go fix` / `gofmt` / `go vet` / `go test ./...` all pass
- [x] Go usage notes added to the README
- [x] Keep `oracle/` in the repo: it powers `TestOracleComparison`, and
      git's GPLv2 is compatible with this repo's license.
