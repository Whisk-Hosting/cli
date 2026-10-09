package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

// cdnState is what the fake CDN routes remember between calls.
type cdnState struct {
	plan     string // team | free
	provider bool
	on       bool
	puts     []map[string]any
}

func (st *cdnState) answer() map[string]any {
	status := "off"
	if st.on {
		status = "live"
	}
	return map[string]any{
		"included": st.plan == "team", "available": st.provider, "on": st.on, "status": status,
		"error": "", "since": "2026-10-01T09:00:00Z", "zone_host": "whisk-abcdefghij.b-cdn.net",
		"hostnames": []map[string]any{
			{"hostname": "crm.acme.whisk.page", "through_cdn": st.on},
			{"hostname": "jobs.acme.com", "through_cdn": st.on},
			{"hostname": "acme.com", "through_cdn": false, "reason": "bare"},
		},
		"traffic_bytes": 3 << 30, "org_traffic_bytes": 12 << 30, "allowance_bytes": 100 << 30,
	}
}

// cdnAPI answers the app's CDN routes of CONTROL-PLANE.md §6.29 for acme/crm.
func cdnAPI(t *testing.T, st *cdnState) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/cdn", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, st.answer())
	})
	mux.HandleFunc("PUT /v1/orgs/acme/apps/crm/cdn", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("PUT cdn without Idempotency-Key")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.puts = append(st.puts, body)
		on, _ := body["on"].(bool)
		switch {
		case on && st.plan != "team":
			writeJSON(w, 402, map[string]any{"error": map[string]any{"code": "PLAN_FEATURE", "message": "The CDN is not included in the Free plan.",
				"fix": "Ask an owner to upgrade at https://whisk.run/o/acme/billing.", "details": map[string]any{"setting": "cdn", "plan": "free"}}})
			return
		case on && !st.provider:
			writeJSON(w, 503, map[string]any{"error": map[string]any{"code": "CDN_UNAVAILABLE", "message": "The CDN is not set up on this platform yet.",
				"fix": "Leave the CDN off for now; the app keeps being served directly. Try again once the platform's operator has set the CDN up.", "details": map[string]any{}}})
			return
		}
		st.on = on
		a := st.answer()
		if on {
			a["status"] = "starting"
		} else {
			a["status"] = "stopping"
		}
		writeJSON(w, 202, a)
	})
	return httptest.NewServer(mux)
}

func TestCDNStatus(t *testing.T) {
	st := &cdnState{plan: "team", provider: true, on: true}
	srv := cdnAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	for _, args := range [][]string{{"cdn"}, {"cdn", "status"}} {
		r := runRemote(t, dir, srv.URL, nil, args...)
		for _, want := range []string{"CDN for crm: live", "crm.acme.whisk.page", "through the CDN", "acme.com", "direct (a bare domain", "3.0 GB for this app", "12.0 GB of 100.0 GB included"} {
			if r.Code != 0 || !strings.Contains(r.Stdout, want) {
				t.Errorf("%v lacks %q (exit %d):\n%s", args, want, r.Code, r.Stdout)
			}
		}
	}

	r := runRemote(t, dir, srv.URL, nil, "cdn", "--json")
	c, _ := r.JSON["cdn"].(map[string]any)
	hosts, _ := c["hostnames"].([]any)
	if r.Code != 0 || r.JSON["app"] != "crm" || c["status"] != "live" || len(hosts) != 3 || c["allowance_bytes"] != float64(100<<30) {
		t.Fatalf("cdn json: %+v", r)
	}
}

func TestCDNOnOff(t *testing.T) {
	st := &cdnState{plan: "team", provider: true}
	srv := cdnAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "cdn", "on")
	if r.Code != 0 || !st.on || len(st.puts) != 1 || st.puts[0]["on"] != true || !strings.Contains(r.Stdout, "Turning the CDN on for crm") || !strings.Contains(r.Stdout, "starting") {
		t.Fatalf("cdn on: %+v puts %v", r, st.puts)
	}
	r = runRemote(t, dir, srv.URL, nil, "cdn", "off", "--json")
	c, _ := r.JSON["cdn"].(map[string]any)
	if r.Code != 0 || st.on || st.puts[1]["on"] != false || c["status"] != "stopping" {
		t.Fatalf("cdn off: %+v puts %v", r, st.puts)
	}
}

func TestCDNRefusals(t *testing.T) {
	cases := []struct {
		state cdnState
		code  string
	}{
		{cdnState{plan: "free", provider: true}, "PLAN_FEATURE"},
		{cdnState{plan: "team", provider: false}, "CDN_UNAVAILABLE"},
	}
	for _, c := range cases {
		st := c.state
		srv := cdnAPI(t, &st)
		r := runRemote(t, boundDir(t, srv.URL), srv.URL, nil, "cdn", "on", "--json")
		e, _ := r.JSON["error"].(map[string]any)
		if r.Code == 0 || e["code"] != c.code || e["fix"] == "" || st.on {
			t.Errorf("cdn on refused with %s: %+v", c.code, r)
		}
		if c.code == "PLAN_FEATURE" && e["details"].(map[string]any)["setting"] != "cdn" {
			t.Errorf("PLAN_FEATURE details: %+v", e)
		}
		srv.Close()
	}
}

func TestCDNWords(t *testing.T) {
	routes := map[api.CDNHostname]string{
		{Hostname: "a", ThroughCDN: true}:   "through the CDN",
		{Hostname: "b", Reason: "bare"}:     "direct (a bare domain pointed by A records cannot follow the app's address)",
		{Hostname: "c", Reason: "business"}: "direct (an address on the business's own domain)",
		{Hostname: "d", Reason: "preview"}:  "direct (a preview)",
		{Hostname: "e"}:                     "direct (-)",
	}
	for h, want := range routes {
		if got := cdnHostnameText(h); got != want {
			t.Errorf("cdnHostnameText(%+v) = %q, want %q", h, got, want)
		}
	}
	for _, status := range []string{"off", "starting", "switching", "live", "stopping"} {
		if got := cdnStatusText(status); !strings.HasPrefix(got, status+":") {
			t.Errorf("cdnStatusText(%q) = %q", status, got)
		}
	}
	traffic := []struct {
		c    api.CDN
		want string
	}{
		{api.CDN{TrafficBytes: 1 << 30, OrgTrafficBytes: 2 << 30, AllowanceBytes: 100 << 30}, "the business has used 2.0 GB of 100.0 GB included."},
		{api.CDN{TrafficBytes: 1 << 30, OrgTrafficBytes: 120 << 30, AllowanceBytes: 100 << 30}, "charged per started gigabyte"},
		{api.CDN{TrafficBytes: 0, OrgTrafficBytes: 0}, "0 bytes for this app, 0 bytes for the business."},
	}
	for _, c := range traffic {
		if got := cdnTrafficText(c.c); !strings.Contains(got, c.want) {
			t.Errorf("cdnTrafficText(%+v) = %q, want it to contain %q", c.c, got, c.want)
		}
	}
}
