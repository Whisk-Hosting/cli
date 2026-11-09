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

// readingAPI answers a breakdown, the visitor list and one visitor, keeping each query it got.
func readingAPI(queries map[string]url.Values) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics", func(w http.ResponseWriter, r *http.Request) {
		queries["analytics"] = r.URL.Query()
		by := r.URL.Query().Get("by")
		rows := []map[string]any{{"name": "/pricing", "count": 14}, {"name": "", "count": 2}}
		if by == "day" {
			rows = []map[string]any{{"name": "2026-11-05", "views": 30, "visitors": 9}, {"name": "2026-11-06"}}
		}
		if by == "event" {
			rows = []map[string]any{}
		}
		writeJSON(w, 200, map[string]any{"period": "", "views": 32, "visitors": 9, "visits": 11, "bounce_rate": 0.5, "average_seconds": 61,
			"filters": map[string]any{"source": r.URL.Query().Get("source")}, "by": by, "breakdown": rows,
			"pages": []any{}, "sources": []any{}, "events": []any{}, "crawlers": []any{}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/visitors", func(w http.ResponseWriter, r *http.Request) {
		queries["visitors"] = r.URL.Query()
		writeJSON(w, 200, map[string]any{"total": 51, "next_cursor": "2", "visitors": []map[string]any{{
			"id": "11111111-1111-1111-1111-111111111111", "first_at": "2026-11-06T01:00:00Z", "last_at": "2026-11-06T02:00:00Z",
			"visits": 2, "views": 5, "events": 1, "country": "US", "city": "Austin", "browser": "chrome", "os": "Windows 10", "device": "desktop"}}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics/visitors/{id}", func(w http.ResponseWriter, r *http.Request) {
		queries["visitor"] = r.URL.Query()
		writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "steps": []map[string]any{
			{"at": "2026-11-06T01:00:00Z", "kind": "view", "path": "/", "source": "google.com", "visit": "v1"},
			{"at": "2026-11-06T01:02:00Z", "kind": "event", "path": "/pricing", "event": "outbound-click", "visit": "v1"},
			{"at": "2026-11-06T09:00:00Z", "kind": "view", "path": "/blog", "query": "?ref=x", "visit": "v2"},
		}})
	})
	return httptest.NewServer(mux)
}

func TestAnalyticsReadings(t *testing.T) {
	queries := map[string]url.Values{}
	srv := readingAPI(queries)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "analytics", "--source", "google.com", "--by", "entry", "--limit", "25")
	q := queries["analytics"]
	if r.Code != 0 || q.Get("source") != "google.com" || q.Get("by") != "entry" || q.Get("limit") != "25" || q.Get("period") != "7d" {
		t.Fatalf("filtered breakdown: %+v %v", r, q)
	}
	for _, want := range []string{"ENTRY", "/pricing", "14", "(none)"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("breakdown lacks %q:\n%s", want, r.Stdout)
		}
	}

	// A reading with views and an empty breakdown says so, under its numbers.
	r = runRemote(t, dir, srv.URL, nil, "analytics", "--page", "/software/submit", "--by", "event")
	if r.Code != 0 || !strings.Contains(r.Stdout, "32") || !strings.Contains(r.Stdout, "Analytics recorded no event for these visits.") || strings.Contains(r.Stdout, "No visits") {
		t.Errorf("empty breakdown:\n%s", r.Stdout)
	}

	r = runRemote(t, dir, srv.URL, nil, "analytics", "--from", "2026-11-05", "--to", "2026-11-06", "--tz", "America/New_York", "--by", "day")
	q = queries["analytics"]
	if r.Code != 0 || q.Get("period") != "" || q.Get("from") != "2026-11-05" || q.Get("to") != "2026-11-06" || q.Get("tz") != "America/New_York" {
		t.Fatalf("from and to: %+v %v", r, q)
	}
	if !strings.Contains(r.Stdout, "DAY") || !strings.Contains(r.Stdout, "2026-11-06") {
		t.Errorf("days:\n%s", r.Stdout)
	}

	r = runRemote(t, dir, srv.URL, nil, "analytics", "visitors", "--page", "/pricing", "--cursor", "2")
	q = queries["visitors"]
	if r.Code != 0 || q.Get("page") != "/pricing" || q.Get("cursor") != "2" || q.Get("by") != "" {
		t.Fatalf("visitors: %+v %v", r, q)
	}
	for _, want := range []string{"11111111-1111-1111-1111-111111111111", "Austin, US", "chrome Windows 10", "51 visitors in all", "--cursor 2"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("visitors lacks %q:\n%s", want, r.Stdout)
		}
	}

	r = runRemote(t, dir, srv.URL, nil, "analytics", "visitor", "11111111-1111-1111-1111-111111111111", "--period", "30d")
	if r.Code != 0 || queries["visitor"].Get("period") != "30d" {
		t.Fatalf("visitor: %+v %v", r, queries["visitor"])
	}
	for _, want := range []string{"viewed", "sent outbound-click", "google.com", "/blog?ref=x"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("visitor lacks %q:\n%s", want, r.Stdout)
		}
	}
}

func TestStepRowsNumberVisits(t *testing.T) {
	at := time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC)
	rows := stepRows([]api.AnalyticsStep{{At: at, Kind: "view", Path: "/", Visit: "b"}, {At: at, Kind: "view", Path: "/x", Visit: "a"}, {At: at, Kind: "event", Event: "e", Visit: "b"}})
	if rows[0][1] != "1" || rows[1][1] != "2" || rows[2][1] != "1" || rows[2][2] != "sent e" || rows[0][4] != "-" && rows[0][4] != "—" {
		t.Errorf("stepRows = %v", rows)
	}
}

func TestBreakdownTable(t *testing.T) {
	head, rows := breakdownTable("event-property", []api.AnalyticsRow{{Name: "target=https://a.example", Count: 3}})
	if head[0] != "EVENT PROPERTY" || rows[0][0] != "target=https://a.example" || rows[0][1] != "3" {
		t.Errorf("event-property: %v %v", head, rows)
	}
	if _, rows := breakdownTable("source", []api.AnalyticsRow{{Name: "", Count: 1}}); rows[0][0] != "(direct or own pages)" {
		t.Errorf("no source is direct: %v", rows)
	}
}
