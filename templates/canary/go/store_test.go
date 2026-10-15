package main

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestStoreOutcome(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{minio.ErrorResponse{Code: "AccessDenied", StatusCode: http.StatusForbidden}, "denied"},
		{fmt.Errorf("wrapped: %w", minio.ErrorResponse{StatusCode: http.StatusForbidden}), "denied"},
		{minio.ErrorResponse{Code: "NoSuchKey", StatusCode: http.StatusNotFound}, "missing"},
		{errors.New("dial tcp: refused"), "error: dial tcp: refused"},
	} {
		if got := storeOutcome(c.err); got != c.want {
			t.Errorf("storeOutcome(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
