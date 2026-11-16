package doctor

import (
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
)

func TestSummariseOtherApps(t *testing.T) {
	f := func(pkg, version, fixed, severity string, reach api.PackageReach) api.PackageFinding {
		return api.PackageFinding{ID: "GHSA-" + pkg + "-" + severity, Package: pkg, Version: version, Fixed: fixed, Severity: apitypes.PackageSeverity(severity),
			Where: "source", Path: "package-lock.json", Reach: reach}
	}
	apps := []OtherApp{
		{Org: "acme", App: "crm", Findings: []api.PackageFinding{f("lodash", "4.17.15", "4.17.21", "high", api.ReachUnknown)}},
		{Org: "acme", App: "shop", Findings: []api.PackageFinding{
			f("next", "14.0.0", "14.2.25", "critical", api.ReachUnknown),
			f("next", "14.0.0", "14.1.1", "high", api.ReachUnknown), // the same version: one package to upgrade
			f("axios", "1.6.0", "1.7.4", "high", api.ReachCalled),
		}},
		{Org: "beta", App: "site", Findings: []api.PackageFinding{
			f("x/text", "0.3.0", "0.3.8", "high", api.ReachNotCalled), // set aside
			f("debug", "2.6.0", "", "critical", api.ReachUnknown),     // no fix yet
			f("ms", "0.7.0", "2.0.0", "medium", api.ReachUnknown),     // not serious
		}},
		{Org: "beta", App: "blog", Findings: []api.PackageFinding{f("marked", "0.3.0", "4.0.10", "high", api.ReachUnknown)}},
	}
	for _, c := range []struct {
		name     string
		org, app string
		want     string // the sentence, or "" for no summary
	}{
		{"leaves out the app being checked", "acme", "crm", "2 other apps you work on have 3 packages to upgrade (1 critical): acme/shop 2, beta/blog 1."},
		{"an unbound directory counts every app", "", "", "3 other apps you work on have 4 packages to upgrade (1 critical): acme/shop 2, acme/crm 1, beta/blog 1."},
	} {
		got := SummariseOtherApps(apps, c.org, c.app)
		if got == nil || got.Sentence() != c.want {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
	if got := SummariseOtherApps(apps[2:3], "", ""); got != nil {
		t.Errorf("nothing serious still summarised: %+v", got)
	}
	if got := SummariseOtherApps(apps[:1], "acme", "crm"); got != nil {
		t.Errorf("only the checked app still summarised: %+v", got)
	}
}

func TestOtherAppsSentenceNamesFive(t *testing.T) {
	o := OtherApps{Apps: 7, Packages: 7}
	for _, a := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		o.List = append(o.List, OtherAppLine{Org: "acme", App: a, Packages: 1})
	}
	want := "7 other apps you work on have 7 packages to upgrade: acme/a 1, acme/b 1, acme/c 1, acme/d 1, acme/e 1, 2 more."
	if got := o.Sentence(); got != want {
		t.Errorf("sentence = %q", got)
	}
}
