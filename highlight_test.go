package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestSplitLine(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"plain", "ab\n", []string{"a", "b", "\n"}},
		{"color atomic", "\x1b[31m-x\x1b[m", []string{"\x1b[31m", "-", "x", "\x1b[m"}},
		{"multibyte utf8", "aéz\n", []string{"a", "é", "z", "\n"}},
		// invalid UTF-8: the oracle's utf8::decode fails → per-byte tokens
		{"invalid utf8", "a\xff\xfec\n", []string{"a", "\xff", "\xfe", "c", "\n"}},
		{"color between utf8", "é\x1b[7mó", []string{"é", "\x1b[7m", "ó"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitLine(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("token %d: got %q, want %q (all: %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestVisibleWidth(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"abc", 3},
		{"\x1b[31mabc", 3},
		{"\x1b[1m\x1b[31mab", 2},
		{"", 0},
	}
	for _, tc := range cases {
		if got := visibleWidth(tc.input); got != tc.want {
			t.Errorf("visibleWidth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestVisibleSubstr(t *testing.T) {
	cases := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 2, "llo"},
		{"hello", 0, "hello"},
		// colors don't count as visible; the oracle's s/^$COLOR// drops them
		{"\x1b[31mhello", 2, "llo"},
		{"\x1b[31mh\x1b[32mello", 2, "llo"},
		// n beyond the visible length stops at what it can remove
		{"ab", 5, ""},
	}
	for _, tc := range cases {
		if got := visibleSubstr(tc.input, tc.n); got != tc.want {
			t.Errorf("visibleSubstr(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
		}
	}
}

func TestIsAllBoring(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"", true},
		{" \t\r\n", true},
		{"\x1b[31m \x1b[m", true},
		{"x ", false},
		{" x", false},
		{"\xc2\xa0", false}, // NBSP is not \s on a byte string
	}
	for _, tc := range cases {
		if got := isAllBoring(tc.input); got != tc.want {
			t.Errorf("isAllBoring(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestOnlyMarkerAndBoring(t *testing.T) {
	cases := []struct {
		input  string
		marker byte
		want   bool
	}{
		{"-", '-', true},
		{"-  ", '-', true},
		{"- \t", '-', true},
		{"\x1b[31m-\x1b[m ", '-', true},
		{"-x", '-', false},
		{"- x", '-', false},
		{"+", '-', false},
		{"+", '+', true},
		{"", '-', false},
		{"x-", '-', false},
	}
	for _, tc := range cases {
		if got := onlyMarkerAndBoring(tc.input, tc.marker); got != tc.want {
			t.Errorf("onlyMarkerAndBoring(%q, %q) = %v, want %v", tc.input, tc.marker, got, tc.want)
		}
	}
}

func TestHighlightPair(t *testing.T) {
	resetColorGlobals()
	cachedConfig = map[string]string{} // defaults: \x1b[7m / \x1b[27m

	cases := []struct {
		name     string
		a, b     string
		wantA    string
		wantB    string
		graphInd int
	}{
		{
			name: "middle diff",
			a:    "-prefix a suffix\n", b: "+prefix b suffix\n",
			wantA: "-prefix \x1b[7ma\x1b[27m suffix\n",
			wantB: "+prefix \x1b[7mb\x1b[27m suffix\n",
		},
		{
			name: "identical lines are not interesting",
			a:    "-content\n", b: "+content\n",
			wantA: "-content\n", wantB: "+content\n",
		},
		{
			name: "whole line differs is not interesting",
			a:    "-aaa\n", b: "+bbb\n",
			wantA: "-aaa\n", wantB: "+bbb\n",
		},
		{
			name: "existing colors are skipped in scans and kept",
			a:    "\x1b[31m-foo\x1b[m\n", b: "\x1b[32m+f0o\x1b[m\n",
			wantA: "\x1b[31m-f\x1b[7mo\x1b[27mo\x1b[m\n",
			wantB: "\x1b[32m+f\x1b[7m0\x1b[27mo\x1b[m\n",
		},
		{
			name: "graph cruft stays, boring check sees past indent",
			a:    "| -foo\n", b: "| +f0o\n",
			graphInd: 2,
			wantA:    "| -f\x1b[7mo\x1b[27mo\n",
			wantB:    "| +f\x1b[7m0\x1b[27mo\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotA, gotB := highlightPair(tc.a, tc.b, tc.graphInd)
			if gotA != tc.wantA || gotB != tc.wantB {
				t.Errorf("got (%q, %q), want (%q, %q)", gotA, gotB, tc.wantA, tc.wantB)
			}
		})
	}
}

// newTestFilter returns a filter writing through a bufio.Writer into rec.
func newTestFilter(rec *bytes.Buffer) *filter {
	return newFilter(bufio.NewWriter(rec))
}

func feedAll(t *testing.T, f *filter, input string) {
	t.Helper()
	if err := f.run(strings.NewReader(input)); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestHandleLineHunkBuffering(t *testing.T) {
	resetColorGlobals()
	cachedConfig = map[string]string{}

	// removed lines are emitted first, then the queued added lines
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	feedAll(t, f, "@@ -1,2 +1,2 @@\n-a1\n-a2\n+b1\n+b2\n ctx\n")
	want := "@@ -1,2 +1,2 @@\n" +
		"-\x1b[7ma\x1b[27m1\n-\x1b[7ma\x1b[27m2\n" +
		"+\x1b[7mb\x1b[27m1\n+\x1b[7mb\x1b[27m2\n ctx\n"
	if got := rec.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandleLineHunkStart(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)

	f.handleLine("@@ -1 +1 @@\n")
	if !f.inHunk {
		t.Error("@@ line did not enter hunk")
	}

	var rec2 bytes.Buffer
	f2 := newTestFilter(&rec2)
	f2.handleLine("@@@ -1,1 -1,0 +1,1 @@@\n")
	if f2.inHunk {
		t.Error("@@@ (combined diff) must not enter hunk")
	}
}

func TestHandleLineMarkersOutsideHunk(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	feedAll(t, f, "-not in hunk\n+also not\n")
	if got, want := rec.String(), "-not in hunk\n+also not\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandleLineContextKeepsHunk(t *testing.T) {
	resetColorGlobals()
	cachedConfig = map[string]string{}

	var rec bytes.Buffer
	f := newTestFilter(&rec)
	feedAll(t, f, "@@ -1,3 +1,3 @@\n ctx\n-foo\n+f0o\n ctx\n")
	want := "@@ -1,3 +1,3 @@\n ctx\n" +
		"-f\x1b[7mo\x1b[27mo\n+f\x1b[7m0\x1b[27mo\n ctx\n"
	if got := rec.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandleLineBackslashEndsHunk(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	feedAll(t, f, "@@ -1 +1 @@\n-foo\n+f0o\n\\ No newline at end of file\n-foo\n")
	// the pair is highlighted, then after the backslash line the hunk is
	// over: trailing -foo passes through
	want := "@@ -1 +1 @@\n-f\x1b[7mo\x1b[27mo\n+f\x1b[7m0\x1b[27mo\n" +
		"\\ No newline at end of file\n-foo\n"
	if got := rec.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandleLineMismatchedHunkPassthrough(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	feedAll(t, f, "@@ -1 +1 @@\n-foo\n-end\n+bar\n ctx\n")
	want := "@@ -1 +1 @@\n-foo\n-end\n+bar\n ctx\n"
	if got := rec.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBlankLineFlushesWriter(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	f.handleLine("commit header\n")
	if rec.Len() != 0 {
		t.Fatal("output should still be buffered before a blank line")
	}
	f.handleLine("\n")
	// the blank line itself is emitted too
	if got, want := rec.String(), "commit header\n\n"; got != want {
		t.Errorf("blank line did not flush: got %q, want %q", got, want)
	}
}

func TestGraphIndent(t *testing.T) {
	resetColorGlobals()
	cachedConfig = map[string]string{}

	var rec bytes.Buffer
	f := newTestFilter(&rec)
	f.handleLine("* commit\n")
	if f.graphIndent != 2 {
		t.Fatalf("graphIndent = %d, want 2", f.graphIndent)
	}
	f.handleLine("| @@ -1 +1 @@\n") // detected via stripped line
	if !f.inHunk {
		t.Fatal("graph-stripped @@ line did not enter hunk")
	}
	f.handleLine("| -foo\n")
	f.handleLine("| +f0o\n")
	f.handleLine("|  ctx\n")
	f.out.Flush() //nolint:errcheck
	want := "* commit\n| @@ -1 +1 @@\n" +
		"| -f\x1b[7mo\x1b[27mo\n| +f\x1b[7m0\x1b[27mo\n|  ctx\n"
	if got := rec.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGraphIndentResetsOnShortLine(t *testing.T) {
	var rec bytes.Buffer
	f := newTestFilter(&rec)
	f.handleLine("| * commit\n")
	if f.graphIndent != 4 {
		t.Fatalf("graphIndent = %d, want 4", f.graphIndent)
	}
	f.handleLine("x\n")
	if f.graphIndent != 0 {
		t.Errorf("graphIndent should reset to 0 on a short line, got %d", f.graphIndent)
	}
}

func TestColorConfigFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	resetColorGlobals()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "nonexistent"))
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "color.diff-highlight.oldhighlight")
	t.Setenv("GIT_CONFIG_VALUE_0", "bold")
	t.Setenv("GIT_CONFIG_KEY_1", "color.diff-highlight.newhighlight")
	t.Setenv("GIT_CONFIG_VALUE_1", "italic")

	loadColorConfig()
	if got, want := oldHighlight.highlight, "\x1b[1m"; got != want {
		t.Errorf("oldhighlight = %q, want %q (bold)", got, want)
	}
	if got, want := newHighlight.highlight, "\x1b[3m"; got != want {
		t.Errorf("newhighlight = %q, want %q (italic)", got, want)
	}
	if got, want := oldHighlight.reset, "\x1b[27m"; got != want {
		t.Errorf("oldreset = %q, want default %q", got, want)
	}
}

func TestColorConfigDefaultsWithoutGit(t *testing.T) {
	resetColorGlobals()
	t.Setenv("PATH", "") // git cannot be found

	loadColorConfig()
	if got, want := oldHighlight.highlight, "\x1b[7m"; got != want {
		t.Errorf("oldhighlight = %q, want default %q", got, want)
	}
	if got, want := oldHighlight.reset, "\x1b[27m"; got != want {
		t.Errorf("oldreset = %q, want default %q", got, want)
	}
	// new* falls back to old*
	if got, want := newHighlight.highlight, "\x1b[7m"; got != want {
		t.Errorf("newhighlight = %q, want fallback %q", got, want)
	}
	if got, want := newHighlight.reset, "\x1b[27m"; got != want {
		t.Errorf("newreset = %q, want fallback %q", got, want)
	}
	if newHighlight.hasNormal {
		t.Error("newnormal should be unset")
	}
}

func TestColorConfigNewFallsBackToOld(t *testing.T) {
	resetColorGlobals()
	cachedConfig = map[string]string{"oldhighlight": "\x1b[1m"}

	loadColorConfig()
	if got, want := newHighlight.highlight, "\x1b[1m"; got != want {
		t.Errorf("newhighlight = %q, want oldhighlight %q", got, want)
	}
}

// Go dies by SIGPIPE when writing to a broken stdout pipe (the runtime
// raises it for fds 1 and 2), matching the oracle's $SIG{PIPE}='DEFAULT'.
func TestSIGPIPE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGPIPE on Windows")
	}
	bin := buildTestBinary(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close() // reads will never happen → writes get EPIPE

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader("plain line\n")
	cmd.Stdout = w
	runErr := cmd.Run()
	w.Close()

	exitErr, ok := runErr.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected exit error, got %v", runErr)
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Fatalf("expected death by SIGPIPE, got %#v", exitErr.Sys())
	}
}
