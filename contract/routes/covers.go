package routes

import "strings"

// CoversAll reports whether the patterns together match every path, so that a request to any
// address at all matches one of them: routes.public ["/**"] is the plain case, and "/**/**",
// or "/" with "/*" and "/*/**", cover every path too. A segment with a literal in it ("/api",
// "/a*") matches only some names, so only patterns made wholly of wildcard segments can take
// part, and those match a path by its depth alone. Each depth up to one past the deepest of
// them is tried with a path of that many segments; deeper paths are matched exactly as that
// one is, since only a ** reaches them. Pure.
func CoversAll(patterns []string) bool {
	var open []string
	deepest := 0
	for _, p := range patterns {
		segs := segments(p)
		if !allWildcards(segs) {
			continue
		}
		open = append(open, p)
		deepest = max(deepest, len(segs))
	}
	if len(open) == 0 {
		return false
	}
	for depth := 0; depth <= deepest+1; depth++ {
		if !MatchAny(open, "/"+strings.Repeat("x/", depth)) {
			return false
		}
	}
	return true
}

// allWildcards reports whether every segment is made of * alone, so it matches a segment of
// any name (or, as **, any run of them).
func allWildcards(segs []string) bool {
	for _, s := range segs {
		if s == "" || strings.Trim(s, "*") != "" {
			return false
		}
	}
	return true
}
