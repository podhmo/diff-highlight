# Known limitations inherited from the oracle

These behaviors are reproduced faithfully from `oracle/DiffHighlight.pm`
(the upstream t9400 suite marks the first two `test_expect_failure`).
They are **not** bugs in the Go port — the oracle-diff harness
(`TestOracleComparison`) byte-compares both implementations and they
agree on every case.

## Combining code points are not highlighted as a unit

Input: `unico<U+0301>de` → `unico<U+0302>de` (combining accents).

The oracle splits per *code point* (`utf8::decode` + `split //`), not
per grapheme cluster, so only the bare combining mark is highlighted
while the preceding `o` stays outside the span:

    -unico<REV>́<NO>de      (actual, both implementations)
    -unic<REV>ó<NO>de      (what t9400 expects → test_expect_failure)

The Go port intentionally uses plain rune splitting for the same
result. See `testdata/golden/combining-codepoints.*`.

## Mismatched hunk sizes are not paired at all

A hunk with different removed/added line counts (e.g. 1 `-` and 2 `+`)
gets no intra-line highlight at all; upstream calls the pairing "simple
and stupid" and marks the test an expected failure. Both sides are
emitted verbatim. See `testdata/golden/mismatched-hunk-size.*`.

## Positional pairing only

Removed line *i* is always compared with added line *i*; moving a line
from the top of a block to the bottom produces misleading pairs
(upstream README "Bugs", item 2). Likewise, multiple changes on one
line collapse into a single highlighted blob.

## Empty mid-span can still be highlighted

`-a   ` vs `+a   b` produces `-a   <REV><NO>`: a zero-length highlight
span on the removed side. Harmless but odd; inherited as-is. See
`testdata/golden/whitespace-lines.*`.
