package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The route that puts back an app's deleted files (control-plane/internal/api/handlers_files_restore.go).
var (
	RestoreFiles = New[apitypes.RestoreFilesRequest, apitypes.FilesRestored]("POST /v1/orgs/{org}/apps/{app}/files/restore")

	_ = public.Add(RestoreFiles)
)
