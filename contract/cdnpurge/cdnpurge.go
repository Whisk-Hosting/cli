// Package cdnpurge is the rule for what a CDN purge may name, which the platform and whisk dev
// both keep (CONTROL-PLANE.md §6.29).
package cdnpurge

import "strings"

// MaxPaths is how many paths one purge may name.
const MaxPaths = 20

// PathProblem says what is wrong with a purge path, "" when nothing is: it starts with
// /, holds no space, # or ?, has a * only at its end, and is at most 1024 characters. The
// platform and `whisk dev` both check with it. Pure.
func PathProblem(p string) string {
	switch {
	case !strings.HasPrefix(p, "/"):
		return "does not start with /"
	case strings.ContainsAny(p, " \t\r\n#?"):
		return "holds a space, # or ?"
	case strings.Contains(strings.TrimSuffix(p, "*"), "*"):
		return "has a * before its end"
	case len(p) > 1024:
		return "is longer than 1024 characters"
	}
	return ""
}
