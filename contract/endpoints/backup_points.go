package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// A Promoted app's backup point route.
var (
	BackupPoints = New[None, apitypes.BackupPoints]("GET /v1/orgs/{org}/apps/{app}/backup-points")

	_ = public.Add(BackupPoints)
)
