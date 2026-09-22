package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var testBinary string

// TestMain builds the diff-highlight binary once for tests that exec it
// (TestSIGPIPE, TestOracleComparison).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "diff-highlight-test")
	if err == nil {
		defer os.RemoveAll(dir)
		bin := filepath.Join(dir, "diff-highlight")
		if runtime.GOOS == "windows" { // go build -o appends .exe on Windows
			bin += ".exe"
		}
		if _, lookErr := exec.LookPath("go"); lookErr == nil {
			out, buildErr := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
			if buildErr == nil {
				testBinary = bin
			} else {
				fmt.Fprintf(os.Stderr, "go build: %v\n%s", buildErr, out)
			}
		}
	}
	os.Exit(m.Run())
}

// buildTestBinary returns the binary built by TestMain.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	if testBinary == "" {
		t.Skip("diff-highlight binary could not be built (go toolchain missing)")
	}
	return testBinary
}

// TestOracleComparison is the oracle-diff harness: feed identical input to
// the Perl oracle and the Go binary, and byte-compare the output.
func TestOracleComparison(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl not available")
	}
	bin := buildTestBinary(t)

	type oracleCase struct {
		name  string
		input []byte
		env   []string
	}
	var cases []oracleCase

	// every golden input
	matches, err := filepath.Glob(filepath.Join("testdata", "golden", "*.in"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, oracleCase{name: "golden/" + filepath.Base(m), input: data})
	}

	// color-config cases through real `git config` (env config)
	setResetEnv := []string{
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=color.diff-highlight.oldhighlight", "GIT_CONFIG_VALUE_0=bold",
		"GIT_CONFIG_KEY_1=color.diff-highlight.oldreset", "GIT_CONFIG_VALUE_1=nobold",
		"GIT_CONFIG_KEY_2=color.diff-highlight.newhighlight", "GIT_CONFIG_VALUE_2=italic",
		"GIT_CONFIG_KEY_3=color.diff-highlight.newreset", "GIT_CONFIG_VALUE_3=noitalic",
	}
	normalHlEnv := []string{
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=color.diff-highlight.oldnormal", "GIT_CONFIG_VALUE_0=red",
		"GIT_CONFIG_KEY_1=color.diff-highlight.oldhighlight", "GIT_CONFIG_VALUE_1=magenta",
		"GIT_CONFIG_KEY_2=color.diff-highlight.newnormal", "GIT_CONFIG_VALUE_2=green",
		"GIT_CONFIG_KEY_3=color.diff-highlight.newhighlight", "GIT_CONFIG_VALUE_3=yellow",
	}
	// On Windows the oracle's `git config` backtick call runs under
	// cmd.exe, where the single-quoted regexp is literal — it matches
	// nothing and the oracle silently falls back to defaults. An
	// upstream platform limitation; skip the git-config cases there.
	if runtime.GOOS != "windows" {
		simple := []byte("@@ -1 +1 @@\n-prefix a suffix\n+prefix b suffix\n")
		cases = append(cases,
			oracleCase{"config-set-reset", simple, setResetEnv},
			oracleCase{"config-normal-highlight", simple, normalHlEnv},
		)
	}

	// a large hunk: 500 changed lines (the "huge hunks" smoke check
	// tasks.md left as future work — cheap to include here)
	var big bytes.Buffer
	big.WriteString("@@ -1,500 +1,500 @@\n")
	for i := range 500 {
		fmt.Fprintf(&big, "-line %04d old\n", i)
	}
	for i := range 500 {
		fmt.Fprintf(&big, "+line %04d new\n", i)
	}
	cases = append(cases, oracleCase{name: "large-hunk", input: big.Bytes()})

	// deterministic fuzz: plausible-looking diff/graph/color lines
	rng := rand.New(rand.NewSource(1))
	atoms := []string{
		"foo", "bar", " ", "\t", "-", "+", "@", "*", "|", "\\", "/",
		"\x1b[31m", "\x1b[1m", "\x1b[7m", "\x1b[27m", "\x1b[m",
		"é", "\xff", "@@ -1 +1 @@", "diff --git", "index",
		"commit", " a", "\r",
	}
	for i := range 40 {
		var buf bytes.Buffer
		for l := 0; l < 1+rng.Intn(30); l++ {
			for k := 0; k < 1+rng.Intn(10); k++ {
				buf.WriteString(atoms[rng.Intn(len(atoms))])
			}
			buf.WriteByte('\n')
		}
		cases = append(cases, oracleCase{
			name:  fmt.Sprintf("fuzz-%02d", i),
			input: buf.Bytes(),
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// binmode disables perl's CRLF text layer on Windows so the
			// byte-compare checks the algorithm, not stdio translation.
			// No-op on Unix.
			cmd := exec.Command("perl",
				"-I", "oracle", "-MDiffHighlight",
				"-e", "binmode STDIN; binmode STDOUT; DiffHighlight::highlight_stdin()")
			cmd.Stdin = bytes.NewReader(tc.input)
			cmd.Env = append(os.Environ(), tc.env...)
			want, err := cmd.Output()
			if err != nil {
				t.Fatalf("oracle failed: %v", err)
			}

			cmd = exec.Command(bin)
			cmd.Stdin = bytes.NewReader(tc.input)
			cmd.Env = append(os.Environ(), tc.env...)
			got, err := cmd.Output()
			if err != nil {
				t.Fatalf("go binary failed: %v", err)
			}

			if !bytes.Equal(got, want) {
				t.Errorf("output differs from oracle\ninput:  %q\ngo:     %q\noracle: %q", tc.input, got, want)
			}
		})
	}
}
