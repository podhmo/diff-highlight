package main

import (
	"bufio"
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite .golden files from actual output")

// goldenCase mirrors a dh_test case from oracle/t/t9400-diff-highlight.sh
// (or an extra edge case). Input is "the hunk part from @@ onwards" (or any
// byte stream); the .golden file holds the expected output after running it
// through decodeColor, like the oracle suite's test_decode_color.
type goldenCase struct {
	name string
	// config, when non-nil, is injected as the git-config result
	// (key → ANSI sequence, as `git config --type=color` emits it).
	config map[string]string
}

var goldenCases = []goldenCase{
	// t9400 ports
	{name: "highlight-begin"},
	{name: "highlight-end"},
	{name: "highlight-middle"},
	{name: "whole-line-differs"},
	{name: "mismatched-hunk-size"}, // oracle: test_expect_failure → passthrough
	{name: "utf8-multibyte"},
	{name: "combining-codepoints"}, // oracle: test_expect_failure (known limitation)
	{name: "graph"},
	{name: "combined-diff"},
	{name: "removed-final-newline"},
	{name: "config-set-reset", config: map[string]string{
		"oldhighlight": "\x1b[1m", "oldreset": "\x1b[22m",
		"newhighlight": "\x1b[3m", "newreset": "\x1b[23m",
	}},
	{name: "config-normal-highlight", config: map[string]string{
		"oldnormal": "\x1b[31m", "oldhighlight": "\x1b[35m",
		"newnormal": "\x1b[32m", "newhighlight": "\x1b[33m",
	}},
	// edge cases beyond t9400
	{name: "graph-commits"},
	{name: "graph-nested"},
	{name: "not-a-diff"},
	{name: "empty-input"},
	{name: "blank-lines"},
	{name: "hunk-at-eof"},
	{name: "hunk-at-eof-no-newline"},
	{name: "consecutive-hunks"},
	{name: "colored-input"},
	{name: "colored-hunk-header"},
	{name: "crlf"},
	{name: "invalid-utf8"},
	{name: "tabs"},
	{name: "whitespace-lines"},
	{name: "truncated-ansi"},  // incomplete SGR is not a COLOR token
	{name: "color-only-line"}, // a hunk line carrying only a color
}

func runFilter(t *testing.T, input []byte) string {
	t.Helper()
	var buf bytes.Buffer
	f := newFilter(bufio.NewWriter(&buf))
	if err := f.run(bytes.NewReader(input)); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

func resetColorGlobals() {
	oldHighlight = nil
	newHighlight = nil
	cachedConfig = nil
}

func TestGolden(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			resetColorGlobals()
			if tc.config != nil {
				cachedConfig = tc.config
			}

			input, err := os.ReadFile(filepath.Join(dir, tc.name+".in"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got := decodeColor(runFilter(t, input))

			goldenPath := filepath.Join(dir, tc.name+".golden")
			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("update golden: %v", err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch (-update to regenerate)\ninput:\n%q\ngot:\n%s\nwant:\n%s", input, got, want)
			}
		})
	}
}

// decodeColor mirrors git's test_decode_color: each SGR sequence becomes
// a readable <NAME> tag; unknown codes become <COLOR:code>.
var decodeColorNames = map[string]string{
	"": "RESET", "0": "RESET",
	"1": "BOLD", "22": "NORMAL_INTENSITY",
	"3": "ITALIC", "23": "NOITALIC",
	"4": "UL", "24": "NOUL",
	"5": "BLINK", "25": "NOBLINK",
	"7": "REVERSE", "27": "NOREVERSE",
	"30": "BLACK", "31": "RED", "32": "GREEN", "33": "YELLOW",
	"34": "BLUE", "35": "MAGENTA", "36": "CYAN", "37": "WHITE",
}

var sgrRe = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

func decodeColor(s string) string {
	return sgrRe.ReplaceAllStringFunc(s, func(m string) string {
		code := sgrRe.FindStringSubmatch(m)[1]
		if name, ok := decodeColorNames[code]; ok {
			return "<" + name + ">"
		}
		return "<COLOR:" + code + ">"
	})
}
