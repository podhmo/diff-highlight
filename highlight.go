package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

const resetSeq = "\x1b[m"

var (
	// COLOR = /\x1b\[[0-9;]*m/ : one SGR sequence.
	colorRe   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	colorReAt = regexp.MustCompile(`^\x1b\[[0-9;]*m`)

	// The oracle's /x pattern for the start of a `--graph` commit:
	//   /^(?:COLOR?\|COLOR? )* COLOR?\*COLOR? (?:COLOR?\|COLOR? )* * /x
	graphStartRe = regexp.MustCompile(`^(?:(?:\x1b\[[0-9;]*m)?\|(?:\x1b\[[0-9;]*m)? )*` +
		`(?:\x1b\[[0-9;]*m)?\*(?:\x1b\[[0-9;]*m)? ` +
		`(?:(?:\x1b\[[0-9;]*m)?\|(?:\x1b\[[0-9;]*m)? )* *`)

	hunkStartRe = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*@@ `)
	minusRe     = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*-`)
	plusRe      = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*\+`)
	hunkLineRe  = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)*[@ ]`)
)

// BORING = /$COLOR|\s/ on byte strings: a color sequence or ASCII
// whitespace (Perl's \s without utf8 flag matches [\t\n\f\r \x0b]).
func isBoringByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

func isColorToken(s string) bool {
	return colorReAt.MatchString(s)
}

func colorPrefixLen(s string) int {
	if loc := colorReAt.FindStringIndex(s); loc != nil {
		return loc[1]
	}
	return 0
}

// visibleWidth counts visible characters, excluding color sequences.
// Operates byte-wise like the oracle's `s/^.//` loop ("." is any byte
// except "\n"; a lone "\n" is skipped rather than counted).
func visibleWidth(s string) int {
	w := 0
	for len(s) > 0 {
		if n := colorPrefixLen(s); n > 0 {
			s = s[n:]
			continue
		}
		if s[0] == '\n' {
			s = s[1:]
			continue
		}
		s = s[1:]
		w++
	}
	return w
}

// visibleSubstr returns s minus its first n visible characters; color
// sequences do not count but are kept. Byte-wise, like the oracle.
func visibleSubstr(s string, n int) string {
	for n > 0 && len(s) > 0 {
		if l := colorPrefixLen(s); l > 0 {
			s = s[l:]
			continue
		}
		if s[0] != '\n' {
			s = s[1:]
		}
		n--
	}
	return s
}

// splitLine tokenizes a line into color sequences (one token each) and
// single characters. Valid UTF-8 input is split per rune (the oracle's
// utf8::decode + split // path); invalid UTF-8 falls back to per-byte
// tokens, again like the oracle.
func splitLine(s string) []string {
	runewise := utf8.ValidString(s)
	var out []string
	rest := s
	for len(rest) > 0 {
		loc := colorRe.FindStringIndex(rest)
		if loc == nil {
			out = appendChars(out, rest, runewise)
			break
		}
		if loc[0] > 0 {
			out = appendChars(out, rest[:loc[0]], runewise)
		}
		out = append(out, rest[loc[0]:loc[1]])
		rest = rest[loc[1]:]
	}
	return out
}

func appendChars(out []string, s string, runewise bool) []string {
	if runewise {
		for _, r := range s {
			out = append(out, string(r))
		}
		return out
	}
	for i := 0; i < len(s); i++ {
		out = append(out, s[i:i+1])
	}
	return out
}

// joinRange joins tokens[lo..hi] inclusive; empty range (hi < lo) gives "".
func joinRange(tokens []string, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi >= len(tokens) {
		hi = len(tokens) - 1
	}
	if hi < lo {
		return ""
	}
	return strings.Join(tokens[lo:hi+1], "")
}

func highlightPair(aLine, bLine string, graphIndent int) (string, string) {
	a := splitLine(aLine)
	b := splitLine(bLine)

	// Find common prefix, taking care to skip any ansi color codes.
	seenPlusMinus := false
	pa, pb := 0, 0
prefixScan:
	for pa < len(a) && pb < len(b) {
		switch {
		case isColorToken(a[pa]):
			pa++
		case isColorToken(b[pb]):
			pb++
		case a[pa] == b[pb]:
			pa++
			pb++
		case !seenPlusMinus && a[pa] == "-" && b[pb] == "+":
			// the leading "-"/"+" diff markers always differ
			seenPlusMinus = true
			pa++
			pb++
		default:
			break prefixScan
		}
	}

	// Find common suffix, ignoring colors.
	sa, sb := len(a)-1, len(b)-1
suffixScan:
	for sa >= pa && sb >= pb {
		switch {
		case isColorToken(a[sa]):
			sa--
		case isColorToken(b[sb]):
			sb--
		case a[sa] == b[sb]:
			sa--
			sb--
		default:
			break suffixScan
		}
	}

	if isPairInteresting(a, pa, sa, b, pb, sb, graphIndent) {
		loadColorConfig()
		return highlightLine(a, pa, sa, oldHighlight),
			highlightLine(b, pb, sb, newHighlight)
	}
	return aLine, bLine
}

// isPairInteresting reports whether highlighting would span only a
// subset of the line; whole-line highlights are useless noise.
func isPairInteresting(a []string, pa, sa int, b []string, pb, sb, graphIndent int) bool {
	// The prefix consumed the entire line: the two lines are identical.
	// Can happen when only the trailing newline differs.
	if pa == len(a) || pb == len(b) {
		return false
	}

	prefixA := joinRange(a, 0, pa-1)
	prefixB := joinRange(b, 0, pb-1)
	suffixA := joinRange(a, sa+1, len(a)-1)
	suffixB := joinRange(b, sb+1, len(b)-1)

	return !onlyMarkerAndBoring(visibleSubstr(prefixA, graphIndent), '-') ||
		!onlyMarkerAndBoring(visibleSubstr(prefixB, graphIndent), '+') ||
		!isAllBoring(suffixA) ||
		!isAllBoring(suffixB)
}

// onlyMarkerAndBoring matches /^COLOR*<marker>BORING*$/.
func onlyMarkerAndBoring(s string, marker byte) bool {
	for len(s) > 0 {
		if n := colorPrefixLen(s); n > 0 {
			s = s[n:]
			continue
		}
		break
	}
	if len(s) == 0 || s[0] != marker {
		return false
	}
	return isAllBoring(s[1:])
}

// isAllBoring matches /^BORING*$/ on a byte string.
func isAllBoring(s string) bool {
	for len(s) > 0 {
		if n := colorPrefixLen(s); n > 0 {
			s = s[n:]
			continue
		}
		if !isBoringByte(s[0]) {
			return false
		}
		s = s[1:]
	}
	return true
}

// colorTheme mirrors one of the oracle's @OLD_HIGHLIGHT/@NEW_HIGHLIGHT
// triples (normal, highlight, reset). hasNormal records whether the
// "normal" element is defined, which selects the coloring mode.
type colorTheme struct {
	normal    string
	hasNormal bool
	highlight string
	reset     string
}

var (
	oldHighlight *colorTheme
	newHighlight *colorTheme
	cachedConfig map[string]string
)

func loadColorConfig() {
	if oldHighlight == nil {
		normal, hasNormal := colorConfigGet("oldnormal")
		oldHighlight = &colorTheme{
			normal:    normal,
			hasNormal: hasNormal,
			highlight: colorConfig("oldhighlight", "\x1b[7m"),
			reset:     colorConfig("oldreset", "\x1b[27m"),
		}
	}
	if newHighlight == nil {
		normal, hasNormal := colorConfigGet("newnormal")
		if !hasNormal {
			normal, hasNormal = oldHighlight.normal, oldHighlight.hasNormal
		}
		newHighlight = &colorTheme{
			normal:    normal,
			hasNormal: hasNormal,
			highlight: colorConfig("newhighlight", oldHighlight.highlight),
			reset:     colorConfig("newreset", oldHighlight.reset),
		}
	}
}

func colorConfig(key, fallback string) string {
	if s, ok := colorConfigGet(key); ok {
		return s
	}
	return fallback
}

func colorConfigGet(key string) (string, bool) {
	ensureConfigCache()
	s, ok := cachedConfig[key]
	return s, ok
}

// Ideally we would feed the default as a human-readable color to
// git-config as the fallback value. But diff-highlight does not
// otherwise depend on git at all, and there are reports of it being
// used in other settings. Like the oracle, we handle our own fallback,
// which means we work even if git can't be run.
func ensureConfigCache() {
	if cachedConfig != nil {
		return
	}
	cachedConfig = map[string]string{}

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		defer devnull.Close()
	}
	cmd := exec.Command("git", "config", "--type=color",
		"--get-regexp", `^color\.diff-highlight\.`)
	if devnull != nil {
		cmd.Stderr = devnull
	}
	data, err := cmd.Output()
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := splitAwkField(line)
		if !ok {
			continue
		}
		stripped, found := strings.CutPrefix(key, "color.diff-highlight.")
		if !found {
			continue
		}
		cachedConfig[stripped] = value
	}
}

// splitAwkField emulates Perl's split ' ', $line, 2: leading whitespace
// is dropped and the first run of whitespace separates key from value.
func splitAwkField(line string) (key, value string, ok bool) {
	line = strings.TrimLeft(line, " \t")
	i := strings.IndexAny(line, " \t")
	if i < 0 {
		return "", "", false
	}
	return line[:i], strings.TrimLeft(line[i:], " \t"), true
}

func highlightLine(tokens []string, prefix, suffix int, th *colorTheme) string {
	start := joinRange(tokens, 0, prefix-1)
	mid := joinRange(tokens, prefix, suffix)
	end := joinRange(tokens, suffix+1, len(tokens)-1)

	// If we have a "normal" color specified, then take over the whole
	// line. Otherwise, we try to just manipulate the highlighted bits.
	if th.hasNormal {
		start = colorRe.ReplaceAllString(start, "")
		mid = colorRe.ReplaceAllString(mid, "")
		end = strings.TrimSuffix(colorRe.ReplaceAllString(end, ""), "\n")
		return th.normal + start + resetSeq +
			th.highlight + mid + resetSeq +
			th.normal + end + resetSeq + "\n"
	}
	return start + th.highlight + mid + th.reset + end
}
