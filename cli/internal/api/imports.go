package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/whisk-run/contract/run/runhttp"
)

// ImportAuthorisation is the form a dump for an import is posted with (CONTROL-PLANE.md §6.18
// "db import").
type ImportAuthorisation struct {
	ID        string            `json:"id"`
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields"`
	MaxBytes  int64             `json:"max_bytes"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// AuthoriseImport asks for a form to upload a dump of size bytes for the app's database.
// POST /orgs/:org/apps/:app/db/imports {bytes}.
func (c *Client) AuthoriseImport(ctx context.Context, org, app string, size int64) (ImportAuthorisation, error) {
	var out ImportAuthorisation
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/db/imports", map[string]any{"bytes": size}, &out)
}

// ImportDatabase loads an uploaded dump into a fresh database beside the live one, swapped in
// when swap is set. POST /orgs/:org/apps/:app/restore {import, swap}.
func (c *Client) ImportDatabase(ctx context.Context, org, app, id string, swap bool) (Restore, error) {
	var out Restore
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/restore", map[string]any{"import": id, "swap": swap}, &out)
}

// uploadWindow bounds one form post: the largest file a form takes (5 GB), over a slow line.
const uploadWindow = 6 * time.Hour

// UploadDump posts the dump through the form, size bytes read from r, reporting the bytes sent
// so far to progress.
func (c *Client) UploadDump(ctx context.Context, auth ImportAuthorisation, r io.Reader, size int64, progress func(sent int64)) error {
	return c.postForm(ctx, auth.URL, auth.Fields, "dump.pgc", "application/octet-stream", r, size, progress)
}

// postForm posts a file to an object store's pre-signed form: the form's fields and then the
// file, size bytes read from r, with its length known up front, since an object store refuses a
// post of unknown length. The store is sent no token.
func (c *Client) postForm(ctx context.Context, url string, fields map[string]string, filename, contentType string, r io.Reader, size int64, progress func(sent int64)) error {
	head, tail, formType, err := formParts(fields, filename, contentType)
	if err != nil {
		return err
	}
	body := io.MultiReader(bytes.NewReader(head), &counting{r: io.LimitReader(r, size), report: progress}, bytes.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(head)) + size + int64(len(tail))
	req.Header.Set("Content-Type", formType)
	req.Header.Set("User-Agent", c.UserAgent)
	hc := runhttp.Client(uploadWindow)
	hc.CheckRedirect = SafeRedirect
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("uploading: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("the store refused it: %s %s", res.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// formParts is a multipart form's bytes before and after the file: every field, in a stable
// order, then the file part's header; and the closing boundary. Pure apart from the boundary.
func formParts(fields map[string]string, filename, fileType string) (head, tail []byte, contentType string, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if err := w.WriteField(k, fields[k]); err != nil {
			return nil, nil, "", err
		}
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	h.Set("Content-Type", fileType)
	if _, err := w.CreatePart(h); err != nil {
		return nil, nil, "", err
	}
	head = append([]byte(nil), buf.Bytes()...)
	buf.Reset()
	if err := w.Close(); err != nil {
		return nil, nil, "", err
	}
	tail = append([]byte(nil), buf.Bytes()...)
	return head, tail, w.FormDataContentType(), nil
}

// counting reports how much has been read through it.
type counting struct {
	r      io.Reader
	n      int64
	report func(int64)
}

func (c *counting) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.report != nil && n > 0 {
		c.report(c.n)
	}
	return n, err
}
