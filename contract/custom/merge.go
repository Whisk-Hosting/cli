//whisk:pure

package custom

import "slices"

// Taking a release into code with changes of the business's own (FORKS.md §4). What the release
// changed is joined with what each change touches before anyone merges: a change the release does
// not meet carries over with nobody looking, and one it meets is named with what it asked for and
// the checks that prove it, which is all the person or agent settling the merge needs to keep it.

// Meets is each of released, the paths that differ between the release a fork holds and the one it
// takes, that c names, in order.
func Meets(c Change, released []string) []string {
	var out []string
	for _, p := range released {
		if names(c, p) {
			out = append(out, p)
		}
	}
	return out
}

// Naming is each of changes whose record names the file at p, in order.
func Naming(changes []Change, p string) []Change {
	return slices.DeleteFunc(slices.Clone(changes), func(c Change) bool { return !names(c, p) })
}
