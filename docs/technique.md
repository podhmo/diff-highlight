# diff-highlight technical notes

Oracle: https://github.com/git/git/tree/master/contrib/diff-highlight
(A snapshot of the upstream sources is kept under `oracle/` in this repository.)

## Goal and overview

A streaming post-processing filter for unified diffs (`git log -p --color`,
etc.). It finds pairs of removed `-` and added `+` lines and highlights only
the intra-line differences with ANSI colors. To preserve the line-oriented
diff, the output must be identical to the input except for the occasional
highlight.

Upstream layout (git tree):

- `diff-highlight.perl`: an 8-line thin wrapper. Sets `SIG{PIPE} = 'DEFAULT'`
  and calls `DiffHighlight::highlight_stdin()`.
- `DiffHighlight.pm`: the actual implementation (~300 lines).
- `Makefile`: builds the single `diff-highlight` executable by concatenating
  `shebang + DiffHighlight.pm + diff-highlight.perl`.
- `t/t9400-diff-highlight.sh`: shell tests on git's test framework
  (test-lib.sh).

The Go port needs neither the cat-assembly nor a Makefile; a single
`package main` suffices.

## State and I/O model

Filter-internal state:

- `@removed` / `@added`: buffered `-` / `+` lines of the current hunk.
- `$in_hunk`: whether the previous line was inside a hunk.
- `$graph_indent`: visible width of the left-side graph drawing
  ("| * " etc.) from `git log --graph`. This many visible columns are
  stripped before diff detection.

Output goes through callbacks:

- `$line_cb`: emits lines (default: `print`). May be called with multiple
  lines at once.
- `$flush_cb`: flushes output when a "logical chunk" is done
  (default: flush stdout).

In Go these map to a `bufio.Writer` plus `Flush()`.

## Regex atoms

- `COLOR = /\x1b\[[0-9;]*m/`: one SGR sequence.
- `BORING = /$COLOR|\s/`: a color or whitespace.
- `RESET = "\x1b[m"`.

## handle_line (per-line state machine)

1. **Graph-line detection**: matching
   `/^(?:COLOR?\|COLOR? )* COLOR?\*COLOR? (?:COLOR?\|COLOR? )* * /x`
   marks the start of a `--graph` commit.
   - `flush()` first (so a queued hunk isn't mixed with a differently
     indented commit).
   - `$graph_indent = visible_width($&)` (visible width of the matched
     prefix).

2. **Graph indent handling**: when `$graph_indent > 0`:
   - If the line is shorter than the indent, reset `$graph_indent = 0`.
   - Otherwise replace `$_` with `visible_substr($_, $graph_indent)` for
     detection — note the original `$orig` is still used for output.

3. **Hunk state machine**:
   - `!$in_hunk`: emit the line as-is; set `$in_hunk = 1` if it matches
     `/^COLOR*\@\@ /` (starts with `@@ `, possibly colored).
     - `@@@` (combined diff) does not match `@@ `, so combined diffs pass
       through untouched (test "ignores combined diffs").
   - `$in_hunk` and `/^COLOR*-/`: push onto `@removed`.
   - `$in_hunk` and `/^COLOR*\+/`: push onto `@added`.
   - Anything else (context lines, `\ No newline at end of file`, etc.):
     `flush()`, emit, then re-evaluate `$in_hunk = /^COLOR*[\@ ]/`.
     (A `\` line inside a hunk therefore terminates the hunk.)

4. **Blank line flush**: `/^$/` triggers `$flush_cb->()`. A heuristic that
   matches `git log`'s blank separator between commits, so output appears
   early even for slow-producing commands like `git log -S`.

## show_hunk / flush

`flush()` calls `show_hunk(\@removed, \@added)` and clears both buffers.
`highlight_stdin()` also calls `flush()` after EOF (trailing hunk).

`show_hunk($a, $b)`:

- Either side empty → nothing to compare → emit as-is.
- Different line counts → emit as-is (no clever alignment; "simple and
  stupid" policy. The "mismatched hunk size" test is currently a
  test_expect_failure upstream).
- Same line count → pair the i-th removed line with the i-th added line
  via `highlight_pair`. Removed lines are emitted immediately; added
  lines are queued and emitted afterwards (preserving the
  removed-block-then-added-block order).

## highlight_pair (core algorithm)

Both lines are tokenized by `split_line` first.

### split_line

1. Split on `/(COLOR+)/` (capturing, so delimiters are kept).
2. Each element that is a COLOR becomes one token; anything else is split
   with `split //` into **one token per character**.
3. `utf8::decode` beforehand makes multibyte UTF-8 count as one
   character (tokens are re-encoded with `utf8::encode` afterwards).

In Go the equivalent is: ANSI sequences as atomic tokens, everything else
split per rune. Combining code points are not handled as single
characters in Perl either (that test is test_expect_failure upstream) —
accepting the same limitation with plain rune splitting is the faithful
port.

### Common prefix scan

Advance `$pa`, `$pb` from the front. Each step:

- `a[$pa]` is COLOR → `$pa++` (skip colors).
- `b[$pb]` is COLOR → `$pb++`.
- Equal tokens → advance both.
- If `seen_plusminus` not yet set and `a[$pa] eq '-'` and
  `b[$pb] eq '+'`: treat as the leading diff marker pair, advance both,
  set `seen_plusminus = 1` (the leading `-`/`+` always differs).
- Otherwise stop.

### Common suffix scan

From `$sa = $#a`, `$sb = $#b` backwards: skip colors, decrement while
tokens are equal, bounded by the prefix positions (`$sa >= $pa` and
`$sb >= $pb`).

### is_pair_interesting (suppress whole-line highlight)

If the whole line would be highlighted, highlighting is noise — same as
the line-level diff — so suppress it.

- `$pa == @$a` or `$pb == @$b` (lines identical; prefix consumed
  everything) → not interesting. Happens with non-minimal diffs like
  `foo` → `foo` where only the trailing newline differs.
- prefix/suffix containing only "boring" tokens → not interesting:
  - `visible_substr($prefix_a, $graph_indent)` matching
    `/^COLOR*-$BORING*$/` (just `-` plus whitespace/colors) → boring.
    Same for `+` on the b side.
  - `suffix_a`/`suffix_b` matching `/^BORING*$/` → boring.
  - All four boring → not interesting (highlight would span the whole
    line).

### highlight_line (two coloring modes)

Join tokens into three spans `[0..prefix-1] | [prefix..suffix] |
[suffix+1..end]` → `start | mid | end`:

- **normal color mode** (`theme[0]` defined): strip all existing colors
  (`s/COLOR//g`), then emit
  `normal + start + RESET + highlight + mid + RESET + normal + end + RESET + \n`
  (the theme takes over the whole line).
- **highlight/reset mode** (`theme[0]` undefined): keep existing colors,
  splice `highlight`/`reset` around mid only:
  `start + highlight + mid + reset + end`.

## Color configuration (color_config / load_color_config)

- Themes: `@OLD_HIGHLIGHT` / `@NEW_HIGHLIGHT`, 3 elements each:
  `(normal, highlight, reset)`.
- If not set externally (module consumers may set them), lazily run
  `git config --type=color --get-regexp '^color\.diff-highlight\.'`
  (stderr → devnull) and cache. Deliberately has its own fallback so it
  **works even when git cannot be run** (see the code comment).
- Defaults: `oldhighlight = "\x1b[7m"` (reverse), `oldreset = "\x1b[27m"`.
  `new*` falls back to `old*` when unset.
- Config keys: `color.diff-highlight.{old,new}{Normal,Highlight,Reset}`.

The Go port can do the same with `exec.Command("git", "config", ...)`
plus a fallback. Whether to stay git-independent (env vars, own parser)
is an open consideration — see tasks.

## Utilities

- `visible_width($s)`: count visible characters, skipping COLOR tokens.
- `visible_substr($s, $n)`: return the string minus the first `n`
  visible characters; leading COLOR tokens don't count and are dropped
  by `s/^$COLOR//` (along with the removed characters).

## Known limitations (oracle README "Bugs")

1. Multiple changes on one line get highlighted as a single blob
   (`foo(buf, size)` → `foo(obj->buf, obj->size)` highlights the whole
   middle). Unavoidable without word-diff-style boundaries; a design
   decision.
2. Multi-line pairing is positional (by index) only, so removing a line
   at the top and adding at the bottom can produce misleading pairs.

## Oracle test coverage (t9400-diff-highlight.sh)

`dh_test a b`: commit a file with contents `a`, rewrite to `b`, generate
both `git diff` and `git show` output, run diff-highlight on each, strip
the header, decode colors, compare against expected.

- highlights beginning / end / middle of a line
- no highlight when the whole line differs
- mismatched hunk sizes (expected failure)
- multibyte UTF-8 / combining code points (latter expected failure)
- `--graph` (plain / colored / graph with leading `-`)
- combined diffs ignored
- removed final newline (`\ No newline at end of file`)
- color config: set/reset mode, normal/highlight mode
