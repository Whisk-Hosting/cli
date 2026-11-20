package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The storage routes.
var (
	GetStorage         = New[None, apitypes.StorageInfo]("GET /v1/orgs/{org}/storage")
	GetAppStorage      = New[None, apitypes.AppStorage]("GET /v1/orgs/{org}/apps/{app}/storage")
	SetStorageProvider = New[apitypes.StorageProviderRequest, apitypes.StorageProviderSet]("PUT /v1/orgs/{org}/storage/provider")
	ListUploads        = New[ListUploadsQuery, apitypes.Page[apitypes.Upload]]("GET /v1/orgs/{org}/apps/{app}/uploads")
	GetUpload          = New[None, apitypes.Upload]("GET /v1/orgs/{org}/apps/{app}/uploads/{id}")
	DeleteUpload       = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/uploads/{id}")
	ListNotifications  = New[None, apitypes.List[apitypes.Notification]]("GET /v1/orgs/{org}/notifications")
	LinkUploads        = New[apitypes.LinkUploadsRequest, apitypes.MediaLinks]("POST /v1/orgs/{org}/apps/{app}/uploads/links")
	RotateLinkKey      = New[apitypes.RotateLinkKeyRequest, apitypes.MediaLinkKey]("POST /v1/orgs/{org}/apps/{app}/uploads/links/rotate")
	PatchUpload        = New[apitypes.PatchUploadRequest, apitypes.Upload]("PATCH /v1/orgs/{org}/apps/{app}/uploads/{id}")
	AuthoriseUpload    = New[apitypes.AuthoriseUploadRequest, apitypes.UploadAuthorisation]("POST /v1/orgs/{org}/apps/{app}/uploads")
	UploadParts        = New[apitypes.UploadPartsRequest, apitypes.UploadParts]("POST /v1/orgs/{org}/apps/{app}/uploads/{id}/parts")
	CompleteUpload     = New[apitypes.UploadPartsRequest, apitypes.Upload]("POST /v1/orgs/{org}/apps/{app}/uploads/{id}/complete")

	_ = public.Add(GetStorage, GetAppStorage, SetStorageProvider, ListUploads, GetUpload, DeleteUpload, ListNotifications, LinkUploads, RotateLinkKey, PatchUpload, AuthoriseUpload, UploadParts, CompleteUpload)
)

// ListUploadsQuery is what ListUploads reads from the query: newest first, a page at a time.
type ListUploadsQuery struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}
