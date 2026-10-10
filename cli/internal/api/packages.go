package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// PackageFinding is one known vulnerability in one installed package (CONTROL-PLANE.md §6.28).
type PackageFinding = apitypes.PackageFinding

// PackageReach is whether the app's code calls a finding: called, not_called or unknown.
type PackageReach = apitypes.PackageReach

// The reaches.
const (
	ReachCalled    = apitypes.ReachCalled
	ReachNotCalled = apitypes.ReachNotCalled
	ReachUnknown   = apitypes.ReachUnknown
)

// PackageCounts is how many findings the app's code may call a scan has at each severity, with a
// fix, and serious, and how many were set aside as not called.
type PackageCounts = apitypes.PackageCounts

// PackageScan is one check of an app's packages.
type PackageScan = apitypes.PackageScan

// Packages is GET /orgs/:org/apps/:app/packages: whether the plan includes scanning, the newest
// finished scan and the check waiting or running.
type Packages = apitypes.Packages

func packagesPath(org, app string) string { return appPath(org, app) + "/packages" }

// GetPackages reads the app's newest package scan.
func (c *Client) GetPackages(ctx context.Context, org, app string) (Packages, error) {
	var out Packages
	return out, c.Do(ctx, http.MethodGet, packagesPath(org, app), nil, &out)
}

// ScanPackages asks for a check of the live production build now.
func (c *Client) ScanPackages(ctx context.Context, org, app string) (PackageScan, error) {
	var out PackageScan
	return out, c.Do(ctx, http.MethodPost, packagesPath(org, app)+"/scan", nil, &out)
}

// GetPackageScan reads one check.
func (c *Client) GetPackageScan(ctx context.Context, org, app, id string) (PackageScan, error) {
	var out PackageScan
	return out, c.Do(ctx, http.MethodGet, packagesPath(org, app)+"/scans/"+pathSeg(id), nil, &out)
}

// Lockfile is one lockfile sent for a check: its path from the app's root and its text.
type Lockfile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// PackageCheck is POST /orgs/:org/apps/:app/packages/check: what OSV-Scanner found in lockfiles
// that are not deployed yet.
type PackageCheck struct {
	Counts   PackageCounts    `json:"counts"`
	Findings []PackageFinding `json:"findings"`
}

// CheckPackages checks lockfiles before they are deployed (Business plan).
func (c *Client) CheckPackages(ctx context.Context, org, app string, files []Lockfile) (PackageCheck, error) {
	var out PackageCheck
	return out, c.Do(ctx, http.MethodPost, packagesPath(org, app)+"/check", map[string]any{"files": files}, &out)
}
