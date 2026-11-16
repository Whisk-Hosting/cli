package apitypes

import "time"

// UploadParts is where an upload in parts stands: the store's id for it, the size of every part but
// the last, and each part, stored (with its ETag) or still to send (with its URL). Complete is
// set once the store has joined the parts.
type UploadParts struct {
	UploadID     string            `json:"upload_id"`
	PartSize     int64             `json:"part_size"`
	Count        int               `json:"count"`
	Parts        []UploadPartState `json:"parts"`
	URLsExpireAt time.Time         `json:"urls_expire_at,omitzero"`
	Complete     bool              `json:"complete,omitempty"`
}

// UploadPartState is one part: its number, its size, and its ETag once stored or its URL to PUT it.
type UploadPartState struct {
	Number int    `json:"number"`
	Bytes  int64  `json:"bytes"`
	ETag   string `json:"etag,omitempty"`
	URL    string `json:"url,omitempty"`
}

// UploadAuthorisation is the answer to "let my visitor upload a file": where to post, what to send
// with it, and the record the platform keeps of what it allowed.
type UploadAuthorisation struct {
	ID        string            `json:"id"`
	Key       string            `json:"key"`
	URL       string            `json:"url,omitempty"`
	Method    string            `json:"method,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	MaxBytes  int64             `json:"max_bytes"`
	ExpiresAt time.Time         `json:"expires_at"`
	// ScanMaxBytes is the largest object the virus scan reads now (the tunable
	// storage.scan_max_bytes); a larger one is stored unscanned.
	ScanMaxBytes int64 `json:"scan_max_bytes"`
	// Parts is set instead of the form for an upload in parts (§6.13 "Uploading in parts").
	Parts *UploadParts `json:"parts,omitempty"`
}

var _ = more.addWire(UploadAuthorisation{}, UploadPartState{}, UploadParts{})
