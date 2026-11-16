package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The packages routes.
var (
	GetPackages    = New[None, apitypes.Packages]("GET /v1/orgs/{org}/apps/{app}/packages")
	ScanPackages   = New[None, apitypes.PackageScan]("POST /v1/orgs/{org}/apps/{app}/packages/scan")
	GetPackageScan = New[None, apitypes.PackageScan]("GET /v1/orgs/{org}/apps/{app}/packages/scans/{id}")
	GetOrgPackages = New[None, apitypes.OrgPackages]("GET /v1/orgs/{org}/packages")

	_ = public.Add(GetPackages, ScanPackages, GetPackageScan, GetOrgPackages)
)
