package apitypes

// CdnPurgeRequest is POST /orgs/:org/apps/:app/cdn/purge. Paths are what to drop from the CDN's
// copy, each starting with / and ending in * to drop everything under it; none drops the app's
// whole copy.
type CdnPurgeRequest struct {
	Paths []string `json:"paths"`
}

// CdnPurge is a purge's answer: whether the app is served through the CDN (when it is not there
// is nothing to drop and nothing was asked), whether its whole copy was dropped, and otherwise
// each address dropped, on every name the CDN serves.
type CdnPurge struct {
	On   bool     `json:"on"`
	All  bool     `json:"all"`
	Urls []string `json:"urls"`
}
