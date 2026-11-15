package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The uptime routes.
var (
	BackupPoints     = New[None, apitypes.BackupPoints]("GET /v1/orgs/{org}/apps/{app}/backup-points")
	AppUptime        = New[None, apitypes.Uptime]("GET /v1/orgs/{org}/apps/{app}/uptime")
	GetStatusPage    = New[None, apitypes.StatusPageAnswer]("GET /v1/orgs/{org}/apps/{app}/status-page")
	PutStatusPage    = New[apitypes.StatusPageRequest, apitypes.StatusPage]("PUT /v1/orgs/{org}/apps/{app}/status-page")
	VerifyStatusPage = New[None, apitypes.StatusPage]("POST /v1/orgs/{org}/apps/{app}/status-page/verify")
	DeleteStatusPage = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/status-page")

	_ = public.Add(BackupPoints, AppUptime, GetStatusPage, PutStatusPage, VerifyStatusPage, DeleteStatusPage)
)
