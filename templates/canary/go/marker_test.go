package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarkerRouteAnswersTheFile(t *testing.T) {
	rec := httptest.NewRecorder()
	markerRoute(rec, httptest.NewRequest("GET", "/diag/marker", nil))
	if marker == "" || rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != strings.TrimSpace(marker) {
		t.Fatalf("got %d %q, want the marker %q", rec.Code, rec.Body.String(), marker)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the marker may be cached")
	}
}
