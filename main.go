// Command diff-highlight is a Go port of git's contrib/diff-highlight:
// a streaming post-processor for unified diffs that highlights only the
// intra-line differences of removed/added line pairs.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// filter holds the streaming state, mirroring the oracle's package vars
// (@removed, @added, $in_hunk, $graph_indent).
type filter struct {
	out         *bufio.Writer
	removed     []string
	added       []string
	inHunk      bool
	graphIndent int
}

func newFilter(out *bufio.Writer) *filter {
	return &filter{out: out}
}

func (f *filter) run(r io.Reader) error {
	in := bufio.NewReader(r)
	for {
		line, err := in.ReadString('\n')
		if len(line) > 0 {
			f.handleLine(line)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	f.flush()
	return f.out.Flush()
}

func (f *filter) emit(lines ...string) {
	for _, l := range lines {
		f.out.WriteString(l) //nolint:errcheck
	}
}

func (f *filter) handleLine(orig string) {
	line := orig

	// match a graph line that begins a commit
	if m := graphStartRe.FindString(line); m != "" {
		// We must flush before setting graph indent, since the
		// new commit may be indented differently from what we
		// queued.
		f.flush()
		f.graphIndent = visibleWidth(m)
	} else if f.graphIndent != 0 {
		if len(line) < f.graphIndent {
			f.graphIndent = 0
		} else {
			line = visibleSubstr(line, f.graphIndent)
		}
	}

	if !f.inHunk {
		f.emit(orig)
		f.inHunk = hunkStartRe.MatchString(line)
	} else if minusRe.MatchString(line) {
		f.removed = append(f.removed, orig)
	} else if plusRe.MatchString(line) {
		f.added = append(f.added, orig)
	} else {
		f.flush()
		f.emit(orig)
		f.inHunk = hunkLineRe.MatchString(line)
	}

	// Most of the time there is enough output to keep things streaming,
	// but for something like "git log -Sfoo", you can get one early
	// commit and then many seconds of nothing. We want to show
	// that one commit as soon as possible.
	//
	// Since we can receive arbitrary input, there's no optimal
	// place to flush. Flushing on a blank line is a heuristic that
	// happens to match git-log output. (The oracle tests /^$/ on the
	// possibly graph-stripped line, which matches "" and "\n".)
	if line == "" || line == "\n" {
		f.out.Flush() //nolint:errcheck
	}
}

func (f *filter) flush() {
	// Flush any queued hunk (this can happen when there is no trailing
	// context in the final diff of the input).
	f.showHunk(f.removed, f.added)
	f.removed = nil
	f.added = nil
}

func (f *filter) showHunk(a, b []string) {
	// If one side is empty, then there is nothing to compare or highlight.
	if len(a) == 0 || len(b) == 0 {
		f.emit(a...)
		f.emit(b...)
		return
	}

	// If we have mismatched numbers of lines on each side, we could try to
	// be clever and match up similar lines. But for now we are simple and
	// stupid, and only handle multi-line hunks that remove and add the same
	// number of lines.
	if len(a) != len(b) {
		f.emit(a...)
		f.emit(b...)
		return
	}

	queue := make([]string, 0, len(b))
	for i := range a {
		rm, add := highlightPair(a[i], b[i], f.graphIndent)
		f.emit(rm)
		queue = append(queue, add)
	}
	f.emit(queue...)
}

func main() {
	enableVirtualTerminal()
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush() //nolint:errcheck
	if err := newFilter(out).run(os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "diff-highlight:", err)
		os.Exit(1)
	}
}
