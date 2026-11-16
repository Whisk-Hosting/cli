package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
	"github.com/whisk-run/contract/run"
	"github.com/whisk-run/contract/status"
)

// orgPackages is one business's answer to GET /orgs/:org/packages.
type orgPackages struct {
	Org      string                 `json:"org"`
	Included bool                   `json:"included"`
	Apps     []apitypes.AppPackages `json:"apps"`
}

// deployRoles are the roles whose apps doctor and whisk scan --all report on: the people who can
// ship a fix.
var deployRoles = map[apitypes.Role]bool{apitypes.RoleOwner: true, apitypes.RoleAdmin: true, apitypes.RoleDeveloper: true}

// fixingOrgs is the businesses whose apps the person can deploy, by slug, in the order given. Pure.
func fixingOrgs(orgs []apitypes.Org) []string {
	out := []string{}
	for _, o := range orgs {
		if deployRoles[o.Role] && o.Status == status.OrgActive {
			out = append(out, o.Slug)
		}
	}
	return out
}

// elsewhereLimit bounds how long asking every business may take, so doctor never hangs on it.
const elsewhereLimit = 15 * time.Second

// elsewhereAtOnce is how many businesses are asked at the same time.
const elsewhereAtOnce = 4

// packagesEverywhere asks each business the person can deploy in for its apps' newest scans, a
// few at a time, within elsewhereLimit. The businesses that answered are returned in order even
// when another did not, with the errors joined.
func packagesEverywhere(ctx context.Context, client *api.Client) ([]orgPackages, error) {
	ctx, cancel := context.WithTimeout(ctx, elsewhereLimit)
	defer cancel()
	page, err := api.Call(ctx, client, endpoints.ListOrgs)
	if err != nil {
		return nil, err
	}
	slugs := fixingOrgs(page.Items)
	got := make([]orgPackages, len(slugs))
	errs := make([]error, len(slugs))
	idx := make([]int, len(slugs))
	for i := range idx {
		idx[i] = i
	}
	// Each business's error is kept for it, so one that fails leaves the others answered.
	_ = run.Each(ctx, elsewhereAtOnce, idx, func(ctx context.Context, i int) error {
		p, err := api.Call(ctx, client, endpoints.GetOrgPackages, slugs[i])
		if err != nil {
			errs[i] = fmt.Errorf("%s: %w", slugs[i], err)
			return nil
		}
		got[i] = orgPackages{Org: slugs[i], Included: p.Included, Apps: p.Apps}
		return nil
	})
	out := []orgPackages{}
	for i := range slugs {
		if errs[i] == nil {
			out = append(out, got[i])
		}
	}
	return out, errors.Join(errs...)
}

// otherApps is the businesses' apps as doctor summarises them. Pure.
func otherApps(orgs []orgPackages) []doctor.OtherApp {
	out := []doctor.OtherApp{}
	for _, o := range orgs {
		for _, a := range o.Apps {
			out = append(out, doctor.OtherApp{Org: o.Org, App: a.App, Findings: a.Scan.Findings})
		}
	}
	return out
}

// scanAll is whisk scan --all: every app the person can deploy that has packages to upgrade,
// with each package, the version that fixes it and where it was found.
func scanAll(s *session) error {
	client, _, err := s.client()
	if err != nil {
		return wrap(err)
	}
	orgs, err := packagesEverywhere(s.ctx, client)
	if err != nil && len(orgs) == 0 {
		return wrap(err)
	}
	if err != nil {
		s.printer.Progress("scan: some businesses could not be asked: %v", err)
	}
	view := scanAllView(orgs)
	result := map[string]any{"apps": view.Apps, "clean": view.Clean, "not_included": view.NotIncluded}
	s.printer.Result(result, func(w io.Writer) { printScanAll(s.printer, w, view) })
	return nil
}

// scanAllApp is one app in whisk scan --all.
type scanAllApp struct {
	Org      string               `json:"org"`
	App      string               `json:"app"`
	Name     string               `json:"name"`
	Counts   api.PackageCounts    `json:"counts"`
	Checked  time.Time            `json:"checked_at"`
	Findings []api.PackageFinding `json:"findings"`
}

// scanAllResult is whisk scan --all's answer: the apps with something to fix, how many apps
// were checked with nothing to fix, and the businesses whose plan has no scanning.
type scanAllResult struct {
	Apps        []scanAllApp `json:"apps"`
	Clean       int          `json:"clean"`
	NotIncluded []string     `json:"not_included"`
}

// scanAllView is the businesses' answers as whisk scan --all shows them: each app with a serious
// finding, most to fix first. Pure.
func scanAllView(orgs []orgPackages) scanAllResult {
	out := scanAllResult{Apps: []scanAllApp{}, NotIncluded: []string{}}
	for _, o := range orgs {
		if !o.Included {
			out.NotIncluded = append(out.NotIncluded, o.Org)
			continue
		}
		for _, a := range o.Apps {
			if len(a.Scan.Findings) == 0 {
				out.Clean++
				continue
			}
			out.Apps = append(out.Apps, scanAllApp{Org: o.Org, App: a.App, Name: a.Name, Counts: a.Scan.Counts, Checked: a.Scan.FinishedAt, Findings: a.Scan.Findings})
		}
	}
	return out
}

// printScanAll is the human view of whisk scan --all.
func printScanAll(p output.Printer, w io.Writer, v scanAllResult) {
	for _, a := range v.Apps {
		fmt.Fprintf(w, "%s/%s: %s. Checked %s.\n", a.Org, a.App, scanSummary(a.Counts), at(a.Checked))
		p.Table(w, findingHeader, findingRows(a.Findings))
		fmt.Fprintln(w)
	}
	switch {
	case len(v.Apps) > 0:
		fmt.Fprintln(w, "Upgrade each package to its FIXED IN version or newer in that app's repository (a base image package by moving to a newer base image), deploy, then run whisk scan --now there.")
	case v.Clean > 0:
		fmt.Fprintf(w, "Nothing to fix now in the %d app%s checked.\n", v.Clean, map[bool]string{true: "", false: "s"}[v.Clean == 1])
	default:
		fmt.Fprintln(w, "No app you can deploy has a finished check yet.")
	}
	if len(v.Apps) > 0 && v.Clean > 0 {
		fmt.Fprintf(w, "%d other app%s checked %s nothing to fix now.\n", v.Clean, map[bool]string{true: "", false: "s"}[v.Clean == 1], map[bool]string{true: "has", false: "have"}[v.Clean == 1])
	}
	if len(v.NotIncluded) > 0 {
		fmt.Fprintf(w, "Not checked, because package scanning is in the Business plan: %s.\n", strings.Join(v.NotIncluded, ", "))
	}
}
