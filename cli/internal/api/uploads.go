package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/whisk-run/contract/run/runhttp"

	"github.com/whisk-run/contract/apitypes"
)

// Upload is one file the app authorised into its business's storage, and what the scan and the
// conversion made of it (CONTROL-PLANE.md §6.13).
type Upload = apitypes.Upload

// UploadAuthorisation is the form a file is posted to the store with (CONTROL-PLANE.md §6.13
// "Authorising an upload").
type UploadAuthorisation struct {
	ID        string            `json:"id"`
	Key       string            `json:"key"`
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields"`
	MaxBytes  int64             `json:"max_bytes"`
	ExpiresAt time.Time         `json:"expires_at"`
	// Parts is set instead of the form for an upload in parts.
	Parts *UploadParts `json:"parts,omitempty"`
}

// UploadParts is where an upload in parts stands (CONTROL-PLANE.md §6.13 "Uploading in parts"):
// the store's id for it, the size of every part but the last, and each part, stored (ETag) or
// still to send (URL). Complete is set once the store has joined them.
type UploadParts struct {
	UploadID     string       `json:"upload_id"`
	PartSize     int64        `json:"part_size"`
	Count        int          `json:"count"`
	Parts        []UploadPart `json:"parts"`
	URLsExpireAt time.Time    `json:"urls_expire_at"`
	Complete     bool         `json:"complete"`
}

// UploadPart is one part of an upload in parts.
type UploadPart struct {
	Number int    `json:"number"`
	Bytes  int64  `json:"bytes"`
	ETag   string `json:"etag,omitempty"`
	URL    string `json:"url,omitempty"`
}

// AuthoriseUpload asks for a form for one file of size bytes, or with parts to upload it in
// parts. POST /orgs/:org/apps/:app/uploads {filename, content_type, max_bytes, visibility, parts}.
func (c *Client) AuthoriseUpload(ctx context.Context, org, app, filename, contentType, visibility string, size int64, parts bool) (UploadAuthorisation, error) {
	var out UploadAuthorisation
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/uploads", map[string]any{
		"filename": filename, "content_type": contentType, "max_bytes": size, "visibility": visibility, "parts": parts,
	}, &out)
}

// UploadPartsOf reads where an upload in parts stands, with fresh URLs for the parts still to
// send. POST /orgs/:org/apps/:app/uploads/:id/parts {upload_id}.
func (c *Client) UploadPartsOf(ctx context.Context, org, app, id, storeID string) (UploadParts, error) {
	var out UploadParts
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/uploads/"+pathSeg(id)+"/parts", map[string]string{"upload_id": storeID}, &out)
}

// CompleteUpload has the store join an upload's parts into the file.
// POST /orgs/:org/apps/:app/uploads/:id/complete {upload_id}.
func (c *Client) CompleteUpload(ctx context.Context, org, app, id, storeID string) (Upload, error) {
	var out Upload
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/uploads/"+pathSeg(id)+"/complete", map[string]string{"upload_id": storeID}, &out)
}

// partWindow bounds one part's PUT: a part of 16 MB over a slow line.
const partWindow = 30 * time.Minute

// PartError is a part the store did not take, with its answer's status (0 when it was never
// reached), so the caller can tell an expired URL from a dropped connection.
type PartError struct {
	Status int
	Err    error
}

func (e *PartError) Error() string { return e.Err.Error() }
func (e *PartError) Unwrap() error { return e.Err }

// PutPart sends one part to its pre-signed URL, size bytes read from r, and answers the store's
// ETag for it. The store is sent no token.
func (c *Client) PutPart(ctx context.Context, url string, r io.Reader, size int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, r)
	if err != nil {
		return "", &PartError{Err: err}
	}
	req.ContentLength = size
	req.Header.Set("User-Agent", c.UserAgent)
	hc := runhttp.Client(partWindow)
	hc.CheckRedirect = SafeRedirect
	res, err := hc.Do(req)
	if err != nil {
		return "", &PartError{Err: fmt.Errorf("sending a part: %w", err)}
	}
	defer res.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	if res.StatusCode/100 != 2 {
		return "", &PartError{Status: res.StatusCode, Err: fmt.Errorf("the store refused a part: %s %s", res.Status, strings.TrimSpace(string(msg)))}
	}
	return res.Header.Get("ETag"), nil
}

// PostUpload posts the file through the form, size bytes read from r, reporting the bytes sent
// so far to progress. The bytes go straight to the store, never through the platform.
func (c *Client) PostUpload(ctx context.Context, auth UploadAuthorisation, filename, contentType string, r io.Reader, size int64, progress func(sent int64)) error {
	return c.postForm(ctx, auth.URL, auth.Fields, filename, contentType, r, size, progress)
}

// GetUpload reads one upload. GET /orgs/:org/apps/:app/uploads/:id.
func (c *Client) GetUpload(ctx context.Context, org, app, id string) (Upload, error) {
	var out Upload
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/uploads/"+pathSeg(id), nil, &out)
}

// ListUploads lists what the app has authorised, newest first. GET /orgs/:org/apps/:app/uploads.
func (c *Client) ListUploads(ctx context.Context, org, app string) ([]Upload, error) {
	return listAll[Upload](ctx, c, appPath(org, app)+"/uploads")
}
