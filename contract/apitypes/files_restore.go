package apitypes

import "time"

// RestoreFilesRequest is POST /orgs/:org/apps/:app/files/restore: put back the app's files
// deleted at or after Since, which is inside the week deleted files are kept. Prefix narrows it
// to the keys under it, as the app names them.
type RestoreFilesRequest struct {
	Since  time.Time `json:"since"`
	Prefix string    `json:"prefix,omitempty"`
}

// FilesRestored is what a restore of deleted files did: how many it put back, how many it left
// because a newer file now has that name, and whether more remain, for a second call to carry on.
// KeptSince is the earliest delete still kept.
type FilesRestored struct {
	Restored  int       `json:"restored"`
	Newer     int       `json:"newer"`
	More      bool      `json:"more"`
	KeptSince time.Time `json:"kept_since"`
}

var _ = more.addWire(RestoreFilesRequest{}, FilesRestored{})
