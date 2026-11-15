package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The storage routes.
var (
	GetStorage         = New[None, apitypes.StorageInfo]("GET /v1/orgs/{org}/storage")
	GetAppStorage      = New[None, apitypes.AppStorage]("GET /v1/orgs/{org}/apps/{app}/storage")
	SetStorageProvider = New[apitypes.StorageProviderRequest, apitypes.StorageProviderSet]("PUT /v1/orgs/{org}/storage/provider")
	ListUploads        = New[None, apitypes.List[apitypes.Upload]]("GET /v1/orgs/{org}/apps/{app}/uploads")
	GetUpload          = New[None, apitypes.Upload]("GET /v1/orgs/{org}/apps/{app}/uploads/{id}")
	DeleteUpload       = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/uploads/{id}")
	ListNotifications  = New[None, map[string]any]("GET /v1/orgs/{org}/notifications")

	_ = public.Add(GetStorage, GetAppStorage, SetStorageProvider, ListUploads, GetUpload, DeleteUpload, ListNotifications)
)
