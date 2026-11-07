package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func crawlersAPI(query *string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/crawlers", func(w http.ResponseWriter, r *http.Request) {
		*query = r.URL.RawQuery
		if r.URL.Query().Get("crawler") == "Nobody" {
			writeJSON(w, 200, map[string]any{"crawler": "Nobody", "crawlers": []any{}, "pages": []any{}, "statuses": []any{}, "journeys": []any{}})
			return
		}
		writeJSON(w, 200, map[string]any{
			"period": "7d", "crawler": r.URL.Query().Get("crawler"), "journey_gap_minutes": 30, "kept_days": 90,
			"crawlers": []map[string]any{{"name": "Googlebot", "kind": "search", "requests": 42, "pages": 7, "journeys": 3, "errors": 2, "cached": 5,
				"first": "2026-11-06T09:00:00Z", "last": "2026-11-06T10:00:00Z"}},
			"pages":    []map[string]any{{"path": "/pricing", "requests": 12, "crawlers": 2, "last_at": "2026-11-06T10:00:00Z", "last_status": 200}},
			"statuses": []map[string]any{{"status": 404, "requests": 2}},
			"journeys": []map[string]any{{"id": "MTc5Mw", "name": "Googlebot", "kind": "search", "network": "66.249.66.0",
				"start": "2026-11-06T09:00:00Z", "end": "2026-11-06T09:12:00Z", "requests": 9, "pages": 4, "first_path": "/", "last_path": "/blog/x"}},
		})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/crawlers/journeys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "MTc5Mw" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "Journey not found.", "fix": "Pick a journey from whisk analytics crawlers."}})
			return
		}
		writeJSON(w, 200, map[string]any{
			"journey": map[string]any{"id": "MTc5Mw", "name": "Googlebot", "kind": "search", "network": "66.249.66.0",
				"start": "2026-11-06T09:00:00Z", "end": "2026-11-06T09:12:00Z", "requests": 2, "pages": 2, "first_path": "/", "last_path": "/old",
				"user_agent": "Mozilla/5.0 (compatible; Googlebot/2.1)", "address": "66.249.66.1", "country": "US", "asn": "Google LLC"},
			"requests": []map[string]any{
				{"at": "2026-11-06T09:00:00Z", "method": "GET", "path": "/", "status": 200, "bytes": 5120, "millis": 42, "source": "edge"},
				{"at": "2026-11-06T09:12:00Z", "method": "GET", "path": "/old", "status": 404, "bytes": 300, "millis": 0, "source": "cdn", "cached": true},
			},
			"more": true,
		})
	})
	return httptest.NewServer(mux)
}

func TestAnalyticsCrawlers(t *testing.T) {
	var query string
	srv := crawlersAPI(&query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "analytics", "crawlers")
	for _, want := range []string{"Googlebot", "search engine", "42", "/pricing", "404", "MTc5Mw", "66.249.66.0", "12m", "/ → /blog/x"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("crawlers lacks %q: %+v", want, r)
		}
	}
	if query != "period=7d" {
		t.Errorf("default query: %q", query)
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "crawlers", "--period", "30d", "--crawler", "Googlebot", "--json")
	c, _ := r.JSON["crawlers"].(map[string]any)
	if r.Code != 0 || c["crawler"] != "Googlebot" || query != "crawler=Googlebot&period=30d" {
		t.Fatalf("json: %+v %q", r, query)
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "crawlers", "--crawler", "Nobody")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No crawlers in that period") {
		t.Fatalf("none: %+v", r)
	}
}

func TestAnalyticsJourney(t *testing.T) {
	var query string
	srv := crawlersAPI(&query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "analytics", "journey", "MTc5Mw")
	for _, want := range []string{"Googlebot (search engine) from 66.249.66.1", "Network: US, Google LLC", "User agent: Mozilla/5.0 (compatible; Googlebot/2.1)", "/old", "404", "5120", "CDN", "the journey has more"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("journey lacks %q: %+v", want, r)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "analytics", "journey", "nope", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "NOT_FOUND" {
		t.Fatalf("unknown journey: %+v", r)
	}
}

func TestPlaceWords(t *testing.T) {
	for _, c := range [][3]string{{"US", "Google LLC", "US, Google LLC"}, {"", "Google LLC", "Google LLC"}, {"US", "", "US"}, {"", "", ""}} {
		if got := placeWords(c[0], c[1]); got != c[2] {
			t.Errorf("placeWords(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestSpan(t *testing.T) {
	for in, want := range map[time.Duration]string{0: "0s", 42 * time.Second: "42s", 12 * time.Minute: "12m", 2 * time.Hour: "2h", 125 * time.Minute: "2h 5m"} {
		if got := span(in); got != want {
			t.Errorf("span(%s) = %q, want %q", in, got, want)
		}
	}
}
