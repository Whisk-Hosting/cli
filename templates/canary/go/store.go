package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
)

// storeRoute is the diagnostic the platform's harness drives to see what this app's store key
// reaches (CONTRACT.md §8): each environment's key reaches its own prefix and nothing else.
// ?put=<key> writes a small object, ?get=<key> reads one, ?list=<prefix> lists one ("/" lists
// the whole bucket). Each answers ok, denied, missing or error. The answer also names the
// environment's bucket and prefix, which are not secrets.
func storeRoute(w http.ResponseWriter, req *http.Request) {
	c, err := storageClient()
	if err != nil {
		writeJSON(w, 500, map[string]any{"configured": true, "error": err.Error()})
		return
	}
	if c == nil {
		writeJSON(w, 200, map[string]any{"configured": false})
		return
	}
	bucket := os.Getenv("WHISK_STORAGE_BUCKET")
	q := req.URL.Query()
	out := map[string]any{"configured": true, "bucket": bucket, "prefix": os.Getenv("WHISK_STORAGE_PREFIX")}
	call := func(f func(ctx context.Context) error) string {
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()
		return storeOutcome(f(ctx))
	}
	if key := q.Get("put"); key != "" {
		out["put"] = call(func(ctx context.Context) error {
			_, err := c.PutObject(ctx, bucket, key, bytes.NewReader([]byte("canary")), 6, minio.PutObjectOptions{ContentType: "text/plain"})
			return err
		})
	}
	if key := q.Get("get"); key != "" {
		out["get"] = call(func(ctx context.Context) error {
			o, err := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
			if err != nil {
				return err
			}
			defer o.Close()
			_, err = io.Copy(io.Discard, o)
			return err
		})
	}
	if prefix := q.Get("list"); prefix != "" {
		if prefix == "/" {
			prefix = ""
		}
		out["list"] = call(func(ctx context.Context) error {
			for o := range c.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, MaxKeys: 10}) {
				if o.Err != nil {
					return o.Err
				}
			}
			return nil
		})
	}
	writeJSON(w, 200, out)
}

// storeOutcome names what the store answered. Pure.
func storeOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		switch {
		case resp.Code == "AccessDenied" || resp.StatusCode == http.StatusForbidden:
			return "denied"
		case resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound:
			return "missing"
		}
	}
	return "error: " + err.Error()
}
