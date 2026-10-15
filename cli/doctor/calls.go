package doctor

import (
	"fmt"
	"slices"
	"strings"
)

// Call consent (doctor-rules.md W120, W121): the platform answers from the business's apps as
// they are now, so both rules need the directory bound and the platform asked.

// callsSkipped is why a call consent rule could not run, or "".
func callsSkipped(r Repo, c Context) string {
	switch {
	case c.Validation != nil:
		return ""
	case r.Binding == nil:
		return "the directory is not bound to an app, so the other apps are unknown"
	}
	return "the platform was not asked, so the other apps are unknown"
}

// w120: callers on the call bridge that this manifest does not list, while it declares no
// accepted_callers.
func w120(r Repo, c Context) outcome {
	if why := callsSkipped(r, c); why != "" {
		return skip("W120", why)
	}
	callers := c.Validation.Calls.Unlisted
	if _, declared := r.Manifest.AcceptsCallers(); declared || len(callers) == 0 {
		return outcome{}
	}
	line := "accepted_callers: [" + strings.Join(callers, ", ") + "]"
	verb := "calls"
	if len(callers) > 1 {
		verb = "call"
	}
	return one("W120", manifestFile, yamlLine(r.ManifestNode, "/name"),
		fmt.Sprintf("%s %s %s only because the call was made before apps had to accept their callers; add %s to keep it.", and(callers), verb, r.Manifest.Name, line))
}

// w121: apps in calls that do not accept this app, one finding each at its line.
func w121(r Repo, c Context) outcome {
	if why := callsSkipped(r, c); why != "" {
		return skip("W121", why)
	}
	var out outcome
	for _, target := range c.Validation.Calls.NotAccepted {
		i := slices.Index(r.Manifest.Calls, target)
		out = out.add("W121", manifestFile, yamlLine(r.ManifestNode, fmt.Sprintf("/calls/%d", max(i, 0))),
			fmt.Sprintf("%s does not accept calls from %s, so the call is refused: the whisk.yaml of %s needs accepted_callers: [%s].", target, r.Manifest.Name, target, r.Manifest.Name))
	}
	return out
}

// and joins names as a sentence does: "a", "a and b", "a, b and c".
func and(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
