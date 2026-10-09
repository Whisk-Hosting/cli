package api

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUploadDumpPostsTheForm: the dump goes as the form's fields followed by the file, with
// the body's whole length declared, which an object store needs, and progress reaches the
// file's size.
func TestUploadDumpPostsTheForm(t *testing.T) {
	dump := "PGDMP" + strings.Repeat("x", 70000)
	var fields map[string]string
	var file string
	var declared int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		declared = r.ContentLength
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Error(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		fields = map[string]string{}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(p)
			if p.FormName() == "file" {
				file = string(b)
				continue
			}
			if file != "" {
				t.Errorf("field %s comes after the file; a store ignores it", p.FormName())
			}
			fields[p.FormName()] = string(b)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := New(srv.URL, "", "", "test")
	var sent int64
	auth := ImportAuthorisation{URL: srv.URL, Fields: map[string]string{"key": "imports/a/b.pgc", "policy": "p", "Content-Type": "application/octet-stream"}}
	if err := c.UploadDump(context.Background(), auth, strings.NewReader(dump), int64(len(dump)), func(n int64) { sent = n }); err != nil {
		t.Fatal(err)
	}
	if file != dump {
		t.Errorf("the store got %d bytes of file, want %d", len(file), len(dump))
	}
	if fields["key"] != "imports/a/b.pgc" || fields["policy"] != "p" || len(fields) != 3 {
		t.Errorf("fields = %v", fields)
	}
	if declared <= int64(len(dump)) {
		t.Errorf("declared length %d does not cover the dump", declared)
	}
	if sent != int64(len(dump)) {
		t.Errorf("progress ended at %d, want %d", sent, len(dump))
	}
}

// TestUploadDumpRefused: a store that refuses the post is an error naming its answer.
func TestUploadDumpRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Error(w, "<Error><Code>EntityTooLarge</Code></Error>", http.StatusBadRequest)
	}))
	defer srv.Close()
	err := New(srv.URL, "", "", "test").UploadDump(context.Background(), ImportAuthorisation{URL: srv.URL}, strings.NewReader("PGDMP"), 5, nil)
	if err == nil || !strings.Contains(err.Error(), "EntityTooLarge") {
		t.Errorf("err = %v", err)
	}
}

// TestRestoreErrorReadsWrapped: a restore record carries its error as the platform stores it,
// {"error": {…}}, and a bare one reads the same.
func TestRestoreErrorReadsWrapped(t *testing.T) {
	for _, raw := range []string{
		`{"id":"r1","status":"failed","error":{"error":{"code":"RESTORE_FAILED","message":"m","fix":"f","details":{"step":"loading"}}}}`,
		`{"id":"r1","status":"failed","error":{"code":"RESTORE_FAILED","message":"m","fix":"f","details":{"step":"loading"}}}`,
	} {
		var r Restore
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		if r.Error == nil || r.Error.Code != "RESTORE_FAILED" || r.Error.Fix != "f" || r.Error.Details["step"] != "loading" {
			t.Errorf("%s read as %+v", raw, r.Error)
		}
	}
}
