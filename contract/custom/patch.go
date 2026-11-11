//whisk:pure

package custom

import (
	"strconv"
	"strings"
)

// PatchAdded is the lines a unified diff, as git diff prints it, adds to each file, by the file's
// path in the new code and each line by its number there. A file the diff deletes adds none.
func PatchAdded(patch string) map[string][]Line {
	out := map[string][]Line{}
	file, next := "", 0
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			file, next = "", 0
		case next == 0 && strings.HasPrefix(l, "+++ "):
			file = patchPath(strings.TrimPrefix(l, "+++ "))
		case strings.HasPrefix(l, "@@ "):
			next = hunkStart(l)
		case next > 0 && strings.HasPrefix(l, "+"):
			if file != "" {
				out[file] = append(out[file], Line{Number: next, Text: l[1:]})
			}
			next++
		case next > 0 && strings.HasPrefix(l, " "):
			next++
		}
	}
	return out
}

// patchPath is the path a diff's "+++" line names in the new code, "" for /dev/null: git writes
// b/ before it, and quotes a path with unusual characters.
func patchPath(s string) string {
	if strings.HasPrefix(s, `"`) {
		if unq, err := strconv.Unquote(s); err == nil {
			s = unq
		}
	}
	p, ok := strings.CutPrefix(s, "b/")
	if !ok {
		return ""
	}
	return p
}

// hunkStart is the first line of the new file a hunk header such as "@@ -3,0 +4,2 @@" covers, 0
// when the header cannot be read.
func hunkStart(header string) int {
	_, after, ok := strings.Cut(header, " +")
	if !ok {
		return 0
	}
	digits, _, _ := strings.Cut(after, ",")
	digits, _, _ = strings.Cut(digits, " ")
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return max(n, 1)
}
