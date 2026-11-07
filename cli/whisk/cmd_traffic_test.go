package whisk

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

func TestAnalyticsTraffic(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/traffic", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		if r.URL.Query().Get("agent") == "Nobody" {
			writeJSON(w, 200, map[string]any{"by": []string{"day"}, "requests": 0, "rows": []any{}})
			return
		}
		writeJSON(w, 200, map[string]any{"by": strings.Split(r.URL.Query().Get("by"), ","), "requests": 17, "addresses": 6, "kept_days": 90,
			"rows": []map[string]any{
				{"group": map[string]string{"path": "/old", "agent": "Googlebot", "cached": "true"}, "requests": 12, "addresses": 2},
				{"group": map[string]string{"path": "/", "agent": "Chrome", "cached": "false"}, "requests": 5, "addresses": 4}}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/traffic/requests", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		writeJSON(w, 200, map[string]any{"requests": []map[string]any{{"at": "2026-11-06T21:04:05Z", "method": "GET", "path": "/pricing",
			"status": 200, "cached": true, "agent": "Safari", "address": "203.0.113.9", "country": "NZ", "network": "SPARK"}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "analytics", "traffic")
	if r.Code != 0 || got.Get("by") != "day" || got.Get("period") != "7d" {
		t.Fatalf("defaults: %+v %v", r, got)
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "traffic", "--by", "path,agent,cached", "--status", "404", "--cached", "true",
		"--from", "2026-11-01", "--country", "NZ", "--user-agent", "curl/8.4.0", "--user-id", "usr_42", "--edge-cache", "hit")
	if r.Code != 0 || got.Get("status") != "404" || got.Get("cached") != "true" || got.Get("from") != "2026-11-01" || got.Get("period") != "" ||
		got.Get("country") != "NZ" || got.Get("user_agent") != "curl/8.4.0" || got.Get("user_id") != "usr_42" || got.Get("edge_cache") != "hit" {
		t.Fatalf("narrowed: %+v %v", r, got)
	}
	for _, want := range []string{"PATH", "AGENT", "CACHED", "ADDRESSES", "/old", "Googlebot", "CDN copy", "app", "17 requests in all, from 6 addresses"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("traffic lacks %q:\n%s", want, r.Stdout)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "traffic", "--agent", "Nobody")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No requests in that period match.") {
		t.Errorf("none: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "traffic", "--list", "--path", "/pricing")
	if r.Code != 0 || got.Get("path") != "/pricing" || got.Has("by") {
		t.Fatalf("list: %+v %v", r, got)
	}
	for _, want := range []string{"2026-11-06 21:04:05", "/pricing", "CDN copy", "Safari", "203.0.113.9", "NZ", "SPARK"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("list lacks %q:\n%s", want, r.Stdout)
		}
	}
}

func TestTrafficTable(t *testing.T) {
	head, rows := trafficTable([]string{"day", "kind", "user_agent", "referer"}, []api.TrafficRow{
		{Group: map[string]string{"day": "2026-11-06", "kind": "search", "user_agent": "Googlebot/2.1", "referer": ""}, Requests: 3, Addresses: 1}})
	if strings.Join(head, "|") != "DAY|KIND|USER AGENT|REFERER|REQUESTS|ADDRESSES" || strings.Join(rows[0], "|") != "2026-11-06|search engine|Googlebot/2.1|-|3|1" {
		t.Errorf("trafficTable = %v %v", head, rows)
	}
}

func TestRequestRows(t *testing.T) {
	at := time.Date(2026, 11, 6, 21, 4, 5, 0, time.UTC)
	woke := 2100
	got := requestRows([]api.TrafficRequest{
		{At: at, Method: "POST", Path: "/api", Query: "page=2", Status: 500, Millis: 30, Agent: "curl", Address: "198.51.100.1"},
		{At: at, Method: "GET", Path: "/", Status: 200, Millis: 2200, WakeMillis: &woke, Agent: "Chrome", Address: "198.51.100.2", Country: "NZ"},
	})
	if strings.Join(got[0], "|") != "2026-11-06 21:04:05|POST|/api?page=2|500|30|app|curl|198.51.100.1|-|-" ||
		strings.Join(got[1], "|") != "2026-11-06 21:04:05|GET|/|200|2200|app, woken|Chrome|198.51.100.2|NZ|-" {
		t.Errorf("requestRows = %v", got)
	}
}
