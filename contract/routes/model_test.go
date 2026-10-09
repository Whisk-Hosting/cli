package routes

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// globModel is the route glob as a regular expression, the obvious reading of the edge's rule:
// a trailing "/" is ignored, "**" is any number of whole segments (none included), "*" is any
// run of characters within a segment, and everything else matches itself.
func globModel(pattern, path string) bool {
	var re strings.Builder
	re.WriteString("^")
	for _, seg := range modelSegments(pattern) {
		if seg == "**" {
			re.WriteString("(?:/[^/]*)*")
			continue
		}
		parts := strings.Split(seg, "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		re.WriteString("/" + strings.Join(parts, "[^/]*"))
	}
	re.WriteString("$")
	p := strings.Join(modelSegments(path), "/")
	if len(modelSegments(path)) > 0 {
		p = "/" + p
	}
	return regexp.MustCompile(re.String()).MatchString(p)
}

func modelSegments(p string) []string {
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(p, "/"), "/")
}

// TestGlobModel checks Match and MatchAny against the regular-expression model over patterns
// and paths built from a small alphabet, so stars, double stars, empty segments and trailing
// slashes meet often (HARNESS.md §8.5).
func TestGlobModel(t *testing.T) {
	patSeg := rapid.SampledFrom([]string{"a", "b", "ab", "", "*", "**", "a*", "*b", "a*b", "*a*", "**a", "a**"})
	pathSeg := rapid.SampledFrom([]string{"a", "b", "ab", "ba", "aab", "abb", "", "*"})
	build := func(segs []string, slash bool) string {
		s := "/" + strings.Join(segs, "/")
		if slash {
			s += "/"
		}
		return s
	}
	rapid.Check(t, func(t *rapid.T) {
		var patterns []string
		for range rapid.IntRange(1, 3).Draw(t, "patterns") {
			patterns = append(patterns, build(rapid.SliceOfN(patSeg, 0, 6).Draw(t, "pattern"), rapid.Bool().Draw(t, "pslash")))
		}
		path := build(rapid.SliceOfN(pathSeg, 0, 7).Draw(t, "path"), rapid.Bool().Draw(t, "slash"))
		anyWant := false
		for _, p := range patterns {
			want := globModel(p, path)
			anyWant = anyWant || want
			if got := Match(p, path); got != want {
				t.Fatalf("Match(%q, %q) = %v, the model says %v", p, path, got, want)
			}
		}
		if got := MatchAny(patterns, path); got != anyWant {
			t.Fatalf("MatchAny(%q, %q) = %v, the model says %v", patterns, path, got, anyWant)
		}
	})
}
