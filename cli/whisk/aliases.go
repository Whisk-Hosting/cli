package whisk

import (
	"slices"

	"github.com/spf13/cobra"
)

// lookupNames are the words agents use for showing one thing: whichever a group has, the others
// reach it too (whisk apps get is whisk apps info, whisk uploads info is whisk uploads show).
var lookupNames = []string{"info", "show", "get"}

// aliasLookups gives every group's info, show or get command the other two names, where no
// sibling already has the name.
func aliasLookups(root *cobra.Command) {
	for _, c := range root.Commands() {
		subs := c.Commands()
		taken := func(name string) bool {
			return slices.ContainsFunc(subs, func(s *cobra.Command) bool { return s.Name() == name || s.HasAlias(name) })
		}
		for _, sub := range subs {
			if !slices.Contains(lookupNames, sub.Name()) {
				continue
			}
			for _, n := range lookupNames {
				if !taken(n) {
					sub.Aliases = append(sub.Aliases, n)
				}
			}
		}
		aliasLookups(c)
	}
}
