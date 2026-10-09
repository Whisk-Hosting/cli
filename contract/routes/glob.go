// Package routes implements the route glob the manifest uses for routes.public,
// routes.challenge and routes.csrf_off, and the classification of a request path against a
// manifest. The semantics are the edge's: * matches within one path segment, ** matches across
// segments, anything else matches exactly.
package routes

import "strings"

// Match reports whether path matches pattern. Both are absolute paths. A trailing "/" on the
// path is ignored so "/reports" and "/reports/" classify the same way.
func Match(pattern, path string) bool {
	return matchSegments(segments(pattern), segments(path))
}

// MatchAny reports whether path matches any of the patterns.
func MatchAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if Match(p, path) {
			return true
		}
	}
	return false
}

func segments(p string) []string {
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(strings.TrimPrefix(p, "/"), "/")
}

// matchSegments fills a table from the last pattern segment back: rest[j] reports whether the
// pattern segments after i match path[j:]. Each cell is computed once, so a pattern of many **
// costs the product of the two lengths rather than growing exponentially with every **.
func matchSegments(pat, path []string) bool {
	rest := make([]bool, len(path)+1)
	rest[len(path)] = true
	for i := len(pat) - 1; i >= 0; i-- {
		cur := make([]bool, len(path)+1)
		for j := len(path); j >= 0; j-- {
			switch {
			case pat[i] == "**":
				// ** matches zero or more segments, including none.
				cur[j] = rest[j] || (j < len(path) && cur[j+1])
			case j < len(path):
				cur[j] = rest[j+1] && matchSegment(pat[i], path[j])
			}
		}
		rest = cur
	}
	return rest[0]
}

// matchSegment matches one segment where * matches any run of characters except "/".
func matchSegment(pat, s string) bool {
	if !strings.Contains(pat, "*") {
		return pat == s
	}
	parts := strings.Split(pat, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}
