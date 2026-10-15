package whisk

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/whisk-run/cli/internal/output"
)

// maxDeleteApps is the most apps one confirm link names, so the link stays a usable length.
const maxDeleteApps = 50

// appRef is an app as the person named it: its org and its slug or ID.
type appRef struct{ org, app string }

// deleteTarget is an app the platform found for a name, kept by its ID.
type deleteTarget struct{ org, id, slug, name string }

// parseAppRefs reads the names given to whisk apps delete (pure but for org, which is asked only
// when a name has no org of its own): <org>/<app> names its org, a bare <app> is in the org the
// command acts on.
func parseAppRefs(args []string, org func() (string, error)) ([]appRef, error) {
	out := make([]appRef, 0, len(args))
	var current string
	for _, a := range args {
		o, app, scoped := strings.Cut(strings.TrimSpace(a), "/")
		if !scoped {
			o, app = "", o
		}
		if app == "" || (scoped && o == "") || strings.Contains(app, "/") {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%q is not an app.", a), "Name each app as <app> or <org>/<app>.", map[string]any{"app": a})
		}
		if !scoped {
			if current == "" {
				got, err := org()
				if err != nil {
					return nil, err
				}
				current = got
			}
			o = current
		}
		out = append(out, appRef{org: o, app: app})
	}
	return out, nil
}

// uniqueTargets drops an app named twice (by slug and by ID, say), keeping the first.
func uniqueTargets(ts []deleteTarget) []deleteTarget {
	seen := map[string]bool{}
	out := make([]deleteTarget, 0, len(ts))
	for _, t := range ts {
		if seen[t.id] {
			continue
		}
		seen[t.id] = true
		out = append(out, t)
	}
	return out
}

// deleteURL is the dashboard page where a person confirms the deletion of exactly these apps,
// named by org and ID (DASHBOARD.md §4.23).
func deleteURL(dash string, ts []deleteTarget) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = url.QueryEscape(t.org) + ":" + url.QueryEscape(t.id)
	}
	return dash + "/delete-apps?apps=" + strings.Join(parts, ",")
}

// needsHumanDelete is the block whisk apps delete prints: the apps it found and the one link
// where a person deletes them all.
func needsHumanDelete(dash string, ts []deleteTarget) *output.Error {
	link := deleteURL(dash, ts)
	names := make([]string, len(ts))
	apps := make([]map[string]any, len(ts))
	for i, t := range ts {
		names[i] = t.org + "/" + t.slug
		apps[i] = map[string]any{"org": t.org, "app": t.slug, "id": t.id, "name": t.name}
	}
	what := "1 app"
	if len(ts) != 1 {
		what = fmt.Sprintf("%d apps", len(ts))
	}
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: fmt.Sprintf("Deleting %s (%s) needs a person to confirm. Deleted apps stop at once and can be restored for 7 days.", what, strings.Join(names, ", ")),
		Fix:     "Ask an owner or admin to open " + link + " and confirm. whisk apps deleted lists them afterwards, and whisk apps restore <id> brings one back.",
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": link, "reason": "delete_apps", "apps": apps},
	}
}
