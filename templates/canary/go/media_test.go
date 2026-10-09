package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signMedia asks the app's own uploads/links endpoint with the service token for the paths
// given, and answers the platform's links, or its refusal as an error.
func TestSignMedia(t *testing.T) {
	var got struct {
		Paths     []string `json:"paths"`
		ExpiresIn int64    `json:"expires_in"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/o/apps/a/uploads/links" || r.Header.Get("Authorization") != "Bearer whsk_service_test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got.Paths[0] == "/notes" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"INVALID_REQUEST"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"expires_at":"2026-10-09T05:00:00Z","links":[{"id":"01UP","path":"/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s","url":"https://x/.whisk/img/01UP?w=800&exp=1&kid=1&sig=s"}]}`))
	}))
	defer srv.Close()
	t.Setenv("WHISK_QUEUE_URL", srv.URL+"/v1/orgs/o/apps/a/events")
	t.Setenv("WHISK_SERVICE_TOKEN", "whsk_service_test")

	links, err := signMedia(context.Background(), []string{"/.whisk/img/01UP?w=800"}, 30*time.Minute)
	if err != nil || len(links.Links) != 1 || links.Links[0].ID != "01UP" || !strings.Contains(links.Links[0].Path, "sig=s") || links.ExpiresAt.IsZero() {
		t.Fatalf("signMedia = %+v %v", links, err)
	}
	if got.ExpiresIn != 1800 || got.Paths[0] != "/.whisk/img/01UP?w=800" {
		t.Errorf("asked for %+v", got)
	}
	if _, err := signMedia(context.Background(), []string{"/notes"}, 0); err == nil || !strings.Contains(err.Error(), "INVALID_REQUEST") {
		t.Errorf("a refusal is an error naming its code: %v", err)
	}
}
