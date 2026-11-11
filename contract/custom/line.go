//whisk:pure

package custom

import (
	"fmt"
	"regexp"
	"strconv"
)

// The release line a fork follows (FORKS.md §2): one commit a release, made by Whisk, saying the
// product's name and the release's number. A checkout knows the line it follows by them, so the
// control plane makes each commit with these and the CLI reads them back.

// LineCommitter is who makes each commit of a release line.
const LineCommitter = "Whisk"

// LineSubject is what the commit of a product's release says.
func LineSubject(product string, number int) string {
	return fmt.Sprintf("%s release %d", product, number)
}

var lineSubject = regexp.MustCompile(`^(\S.*) release ([1-9][0-9]{0,8})$`)

// LineRelease is the product and the number of the release a commit's subject names when
// LineSubject made it, and whether it did.
func LineRelease(subject string) (product string, number int, ok bool) {
	m := lineSubject.FindStringSubmatch(subject)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	return m[1], n, err == nil
}

// IsLineSubject reports whether a commit's subject is one LineSubject makes.
func IsLineSubject(subject string) bool {
	_, _, ok := LineRelease(subject)
	return ok
}
