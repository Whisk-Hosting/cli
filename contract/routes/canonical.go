package routes

import (
	"net/url"
	"strings"
)

// Ambiguous reports whether a request path, as sent on the wire (still percent-encoded), could
// name a different route once something downstream normalises it. The edge classifies the path
// it is given, but an app's router or static file server may resolve "." and ".." segments, and
// some treat "\" as a separator. /public/../private matches /public/** while such an app serves
// /private, so the edge refuses these paths rather than guess which route the app will see. A
// browser never sends one: it resolves dot segments before the request leaves. Segments are
// checked after percent-decoding, so %2e%2e, %2f and %5c cannot hide one; a path that does not
// decode is ambiguous too.
func Ambiguous(escaped string) bool {
	if i := strings.IndexAny(escaped, "?#"); i >= 0 {
		escaped = escaped[:i]
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return true
	}
	return dotSegment(decoded)
}

// dotSegment reports whether any segment, split on "/" or "\", is "." or "..".
func dotSegment(p string) bool {
	for _, s := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if s == "." || s == ".." {
			return true
		}
	}
	return false
}
