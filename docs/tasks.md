# tasks

Tasks for a complete Go reproduction of git/contrib/diff-highlight.
Detailed spec: `docs/technique.md`. Policy/design: `docs/design.md`.
Oracle: `oracle/`.

## Implementation tasks

- [ ] ANSI utilities
  - `COLOR` equivalent: regexp `\x1b\[[0-9;]*m`.
  - `BORING` equivalent: a color or whitespace.
  - `visibleWidth(s)`, `visibleSubstr(s, n)` (color sequences count as
    zero width).
- [ ] Tokenizer `splitLine(line) []string`
  - Split on the COLOR regexp; color parts are single tokens, everything
    else is one token per rune.
  - Perl's `utf8::decode` → `split //` → `utf8::encode` is equivalent to
    rune-wise splitting in Go.
- [ ] `handleLine` state machine
  - Port the `--graph` commit-start detection regexp (the oracle's
    5-line `/x` pattern) verbatim, plus `$graph_indent` management.
  - `$in_hunk` transitions: starts at `@@ `, buffers `-`/`+`, flushes on
    anything else. Verify `@@@` (combined diff) does not match `@@ `.
  - Flush output on blank lines (streaming heuristic).
- [ ] `showHunk`/`flush`
  - Empty side or mismatched line counts → passthrough. Same count →
    pair i-th with i-th.
  - Emit removed lines immediately, queue added lines for afterwards
    (order preservation).
- [ ] `highlightPair`
  - Common prefix scan (color skipping + the leading `-`/`+` marker
    `seen_plusminus` handling).
  - Common suffix scan.
  - `isPairInteresting`: identical lines, or prefix/suffix of only boring
    tokens → no highlight (whole-line-highlight suppression).
- [ ] `highlightLine`
  - Two modes: normal/highlight (strip existing colors, recolor three
    spans) and highlight/reset (keep existing colors, splice only around
    mid).
- [ ] Color config `colorConfig`/`loadColorConfig`
  - Run `git config --type=color --get-regexp '^color\.diff-highlight\.'`,
    cache the result. Must still work with defaults when git is absent
    or fails.
  - Defaults: `oldhighlight=\x1b[7m`, `oldreset=\x1b[27m`; new* falls
    back to old*.
- [ ] `main`
  - Read stdin line by line into `handleLine`; flush at EOF.
  - Use a `bufio.Writer`; `Flush()` on blank input lines.
  - SIGPIPE: Go exits on SIGPIPE writes to stdout by default, matching
    the oracle's `$SIG{PIPE} = 'DEFAULT'`. Verify broken-pipe behavior
    (no SIGPIPE on Windows; just check `EPIPE` handling).
- [ ] Cross-platform (Windows support)
  - Use `os.DevNull` for devnull (equivalent of `File::Spec->devnull()`;
    never hardcode `/dev/null`).
  - Check ANSI passes through when output is a terminal; enable virtual
    terminal processing on cmd.exe/PowerShell if needed
    (`golang.org/x/sys/windows` `SetConsoleMode`). Whether that alone
    justifies a dependency is open.
  - Verify paths, `\r\n` newlines, and the `git config` call on Windows.

## Test tasks (golden tests based on the oracle)

Golden tests run right after the implementation tasks.

Port the `dh_test` cases from `oracle/t/t9400-diff-highlight.sh` into Go
tests. Test inputs can be "the hunk part from `@@` onwards" plus expected
ANSI output in testdata, or real git diffs (the latter adds a git
dependency — check CI first). Cover at least:

- [ ] Highlights at beginning / end / middle of a line
- [ ] No highlight when the whole line differs
- [ ] Mismatched hunk line counts → no highlight (oracle treats this as
      expected failure)
- [ ] Multibyte UTF-8 treated as a single character
- [ ] Combining code points (oracle marks expected failure: document as a
      known limitation or mark as a known failure too)
- [ ] `--graph` (plain / colored / graph with leading `-`)
- [ ] Combined diffs pass through
- [ ] Removed final newline (`\ No newline at end of file`)
- [ ] Color config: set/reset mode and normal/highlight mode (both the
      real `git config` path and the fallback)

## Verification tasks (golden test elaboration and bug hunting)

Run these last, after the implementation is done.

- [ ] Elaborate the golden tests: edge cases beyond t9400
  - Empty input / newline-only input / non-diff input
  - Hunk ending at EOF (no trailing context)
  - `\r\n` newlines, tabs, long lines, huge hunks
  - Broken/truncated ANSI sequences, color-only lines
  - Multiple consecutive hunks, invalid UTF-8 byte sequences
- [ ] Oracle diff harness
  - A script/test that feeds the same input to the Perl oracle and the
    Go binary and byte-compares the output (the oracle is in-tree, so
    direct comparison is possible)
- [ ] Bug report
  - Classify any diverging cases by cause and write up a report under
    `docs/` (distinguish known limitations from real bugs;
    `docs/bugs/` per AGENTS.md)

## Finishing

- [ ] `go fix` / `gofmt` / `go vet` / `go test ./...` all pass
- [ ] Decide whether to add Go usage notes to the README
- [ ] Decide whether to keep `oracle/` in the repo (licensing check if
      kept: git is GPLv2 and so is this repo, so they are compatible)
