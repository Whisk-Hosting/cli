package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func analyticsAPI(views int, query *string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/analytics", func(w http.ResponseWriter, r *http.Request) {
		*query = r.URL.RawQuery
		if r.URL.Query().Get("period") == "1y" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "The period is 24h, 7d, 30d or 90d.", "fix": "Send period=7d."}})
			return
		}
		writeJSON(w, 200, map[string]any{
			"period": r.URL.Query().Get("period"), "views": views, "visitors": 310, "visits": 420, "bounce_rate": 0.25, "average_seconds": 95,
			"pages": []map[string]any{{"name": "/pricing", "count": 380}}, "sources": []map[string]any{{"name": "", "count": 90}}, "kept_days": 396,
		})
	})
	return httptest.NewServer(mux)
}

func TestAnalytics(t *testing.T) {
	var query string
	srv := analyticsAPI(1240, &query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "analytics")
	for _, want := range []string{"VIEWS", "1240", "25%", "1m 35s", "/pricing", "(direct)"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("analytics lacks %q: %+v", want, r)
		}
	}
	if strings.Contains(r.Stdout, "EVENT") {
		t.Errorf("no events, no events table: %s", r.Stdout)
	}
	if query != "period=7d" {
		t.Errorf("default query: %q", query)
	}

	r = runRemote(t, dir, srv.URL, nil, "analytics", "--period", "30d", "--json")
	a, _ := r.JSON["analytics"].(map[string]any)
	if r.Code != 0 || a["views"] != float64(1240) || a["kept_days"] != float64(396) || a["events"] == nil {
		t.Fatalf("json: %+v", r)
	}
	if query != "period=30d" {
		t.Errorf("query: %q", query)
	}

	r = runRemote(t, dir, srv.URL, nil, "analytics", "--period", "1y", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "INVALID_REQUEST" || e["fix"] == "" {
		t.Fatalf("bad period: %+v", r)
	}
}

func TestAnalyticsWithNoVisits(t *testing.T) {
	var query string
	srv := analyticsAPI(0, &query)
	defer srv.Close()
	r := runRemote(t, boundDir(t, srv.URL), srv.URL, nil, "analytics")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No visits in that period") {
		t.Fatalf("no visits: %+v", r)
	}
}

func TestVisitLength(t *testing.T) {
	for in, want := range map[int]string{0: "0s", 42: "42s", 120: "2m", 95: "1m 35s"} {
		if got := visitLength(in); got != want {
			t.Errorf("visitLength(%d) = %q, want %q", in, got, want)
		}
	}
}
