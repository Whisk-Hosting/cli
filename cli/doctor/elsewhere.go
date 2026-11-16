package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/whisk-run/cli/internal/api"
)

// OtherApp is one app the person can deploy, with the serious package findings of its newest
// scan (apitypes.PackageFinding.Serious): what the security update email names.
type OtherApp struct {
	Org      string               `json:"org"`
	App      string               `json:"app"`
	Findings []api.PackageFinding `json:"findings"`
}

// OtherApps is the summary doctor gives of the person's other apps that have packages to
// upgrade, so an agent working on one app hears about the rest. It is no finding of this
// repository and counts in neither errors nor warnings.
type OtherApps struct {
	Apps     int            `json:"apps"`
	Packages int            `json:"packages"`
	List     []OtherAppLine `json:"list"`
	Fix      string         `json:"fix"`
}

// OtherAppLine is one app in OtherApps: how many package versions want upgrading and how many
// of those are critical.
type OtherAppLine struct {
	Org      string `json:"org"`
	App      string `json:"app"`
	Packages int    `json:"packages"`
	Critical int    `json:"critical"`
}

// OtherAppsFix is what OtherApps tells the agent to do.
const OtherAppsFix = "Run whisk scan --all to list each package and the version that fixes it, then upgrade them in each app's own repository."

// SummariseOtherApps folds the other apps' serious findings into one line per app, leaving out
// the app doctor is checking and apps with nothing to upgrade, most critical packages first, then
// most packages, then by name. Nil when nothing is left. Pure.
func SummariseOtherApps(apps []OtherApp, org, app string) *OtherApps {
	out := OtherApps{List: []OtherAppLine{}, Fix: OtherAppsFix}
	for _, a := range apps {
		if a.Org == org && a.App == app {
			continue
		}
		groups := toFix(a.Findings)
		if len(groups) == 0 {
			continue
		}
		line := OtherAppLine{Org: a.Org, App: a.App, Packages: len(groups)}
		for _, g := range groups {
			if g.Severity == "critical" {
				line.Critical++
			}
		}
		out.List = append(out.List, line)
		out.Apps++
		out.Packages += line.Packages
	}
	if out.Apps == 0 {
		return nil
	}
	sort.SliceStable(out.List, func(i, j int) bool {
		a, b := out.List[i], out.List[j]
		if a.Critical != b.Critical {
			return a.Critical > b.Critical
		}
		if a.Packages != b.Packages {
			return a.Packages > b.Packages
		}
		return a.Org+"/"+a.App < b.Org+"/"+b.App
	})
	return &out
}

// otherAppsShown is how many apps the sentence names before it says how many more there are.
const otherAppsShown = 5

// Sentence is the summary in words: "2 other apps you work on have 5 packages to upgrade
// (1 critical): acme/shop 4, beta/site 1." Pure.
func (o OtherApps) Sentence() string {
	critical := 0
	names := []string{}
	for i, l := range o.List {
		critical += l.Critical
		if i < otherAppsShown {
			names = append(names, fmt.Sprintf("%s/%s %d", l.Org, l.App, l.Packages))
		}
	}
	if more := len(o.List) - otherAppsShown; more > 0 {
		names = append(names, fmt.Sprintf("%d more", more))
	}
	crit := ""
	if critical > 0 {
		crit = fmt.Sprintf(" (%d critical)", critical)
	}
	return fmt.Sprintf("%d other app%s you work on %s %d package%s to upgrade%s: %s.",
		o.Apps, plural(o.Apps, "", "s"), plural(o.Apps, "has", "have"), o.Packages, plural(o.Packages, "", "s"), crit, strings.Join(names, ", "))
}
