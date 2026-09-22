package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGitDiffEndToEnd ports the oracle suite's dh_test faithfully: it
// commits a real file change in a scratch repo, feeds both `git diff`
// and `git show` output through the binary, and compares the hunk part
// (like test_strip_patch_header) after decodeColor. Everything before
// the first "@@ " line must pass through byte-identically.
func TestGitDiffEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Isolate git config: no system/global config (so the filter's own
	// `git config` call sees no color.diff-highlight.* overrides), fixed
	// identity, and no autocrlf translation on Windows.
	env := append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "missing-global"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.autocrlf", "GIT_CONFIG_VALUE_0=false",
	)

	git := func(args ...string) []byte {
		cmd := exec.Command("git", append([]string{"-C", dir, "--no-pager"}, args...)...)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return out
	}
	run := func(input []byte) []byte {
		cmd := exec.Command(bin)
		cmd.Stdin = bytes.NewReader(input)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("diff-highlight: %v", err)
		}
		return out
	}

	git("init", "-q")
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("aaa\nbbb\nccc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "file")
	git("commit", "-qm", "Add a file")
	if err := os.WriteFile(file, []byte("aaa\nb0b\nccc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := git("diff", "file")
	git("commit", "-qam", "Update a file")
	show := git("show")

	want := "@@ -1,3 +1,3 @@\n" +
		" aaa\n" +
		"-b<REVERSE>b<NOREVERSE>b\n" +
		"+b<REVERSE>0<NOREVERSE>b\n" +
		" ccc\n"

	for _, tc := range []struct {
		name  string
		input []byte
	}{
		{"git diff", diff},
		{"git show", show},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := run(tc.input)
			inHead, _ := splitAtHunk(tc.input)
			gotHead, gotHunk := splitAtHunk(out)
			if !bytes.Equal(gotHead, inHead) {
				t.Errorf("header modified\ngot:  %q\nwant: %q", gotHead, inHead)
			}
			if got := decodeColor(string(gotHunk)); got != want {
				t.Errorf("hunk mismatch\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// splitAtHunk splits a diff at the first "@@ " line, mirroring the
// oracle suite's test_strip_patch_header (sed -n '/^@@/,$p').
func splitAtHunk(data []byte) (head, hunk []byte) {
	if bytes.HasPrefix(data, []byte("@@ ")) {
		return nil, data
	}
	if i := bytes.Index(data, []byte("\n@@ ")); i >= 0 {
		return data[:i+1], data[i+1:]
	}
	return data, nil
}
