package doctor

import (
	"fmt"
)

// The app's listing (doctor-rules.md W160, W161, CONTRACT.md §3.4): the business's home lists the
// app by its title, description and category. The platform knows the business's categories and
// what an owner set, so both rules need the directory bound and the platform asked.

// listingSkipped is why a listing rule could not run, or "".
func listingSkipped(r Repo, c Context) string {
	switch {
	case c.Validation != nil && c.Validation.Listing != nil:
		return ""
	case r.Binding == nil:
		return "the directory is not bound to an app, so the business's categories are unknown"
	case c.Validation != nil:
		return "the platform does not say the business's categories yet"
	}
	return "the platform was not asked, so the business's categories are unknown"
}

// w160: whisk.yaml leaves out the title, the description or the category. The message names the
// business's categories, so the agent reuses one.
func w160(r Repo, c Context) outcome {
	if why := listingSkipped(r, c); why != "" {
		return skip("W160", why)
	}
	var missing []string
	for _, f := range []struct{ name, value string }{{"title", r.Manifest.Title}, {"description", r.Manifest.Description}, {"category", r.Manifest.Category}} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) == 0 {
		return outcome{}
	}
	msg := fmt.Sprintf("whisk.yaml has no %s, which the business's home lists the app by.", and(missing))
	if r.Manifest.Category == "" {
		if cats := c.Validation.Listing.Categories; len(cats) > 0 {
			msg += " The business's categories are " + quoted(cats) + "; use one of them unless none fits."
		} else {
			msg += " The business has no categories yet, so name one for what the app is for, such as Sales."
		}
	}
	return one("W160", manifestFile, yamlLine(r.ManifestNode, "/name"), msg)
}

// w161: an owner changed the listing on the home, so whisk.yaml no longer says what people see.
func w161(r Repo, c Context) outcome {
	if why := listingSkipped(r, c); why != "" {
		return skip("W161", why)
	}
	shown := c.Validation.Listing.Shown
	if shown == nil {
		return outcome{}
	}
	var out outcome
	for _, f := range []struct{ name, yaml, shown string }{{"title", r.Manifest.Title, shown.Title}, {"description", r.Manifest.Description, shown.Description}, {"category", r.Manifest.Category, shown.Category}} {
		if !owned(c.Validation.Listing.Owned, f.name) || f.yaml == f.shown {
			continue
		}
		line := yamlLine(r.ManifestNode, "/"+f.name)
		if line == 0 {
			line = yamlLine(r.ManifestNode, "/name")
		}
		out = out.add("W161", manifestFile, line, fmt.Sprintf("An owner set the %s on the business's home to %q, which stays; set %s: %q in whisk.yaml to match.", f.name, f.shown, f.name, f.shown))
	}
	return out
}

func owned(list []string, name string) bool {
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}

// quoted joins names in quotes as a sentence does: "a", "a" and "b".
func quoted(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return and(q)
}
