package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

// whisk scan shows the newest check with what fixes each finding; --now asks for a check, follows
// it to the end and then shows it.
func TestScan(t *testing.T) {
	scanEvery = 10 * time.Millisecond
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "scan", "--json")
	scan, _ := r.JSON["scan"].(map[string]any)
	if r.Code != 0 || r.JSON["included"] != true || scan == nil || len(scan["findings"].([]any)) != 2 {
		t.Fatalf("scan --json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "scan")
	for _, want := range []string{"crm: 1 to fix now, 2 in all (1 high, 1 low).", "lodash", "4.17.19", "CVE-2020-8203", "package-lock.json", "busybox", "image", "whisk scan --now"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("scan human lacks %q:\n%s", want, r.Stdout)
		}
	}

	r = runRemote(t, dir, srv.URL, nil, "scan", "--now", "--json")
	if r.Code != 0 || st.scanPolls < 2 || r.JSON["scan"] == nil {
		t.Errorf("scan --now: polls %d, %+v", st.scanPolls, r)
	}
}

// A plan without scanning says so and exits 0: it is a fact about the plan, not a failure.
func TestScanNotIncluded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/packages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"included": false, "scan": nil})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/crm/packages/scan", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 402, map[string]any{"error": map[string]any{"code": "PLAN_FEATURE", "message": "Package scanning is not included in the Team plan.", "fix": "Ask an owner to move to the Business plan.", "details": map[string]any{"setting": "package_scanning"}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	r := runRemote(t, dir, srv.URL, nil, "scan")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Business plan") {
		t.Errorf("scan on Team: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "scan", "--now", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "PLAN_FEATURE") {
		t.Errorf("scan --now on Team: %+v", r)
	}
}

func TestScanWords(t *testing.T) {
	if got := scanSummary(api.PackageCounts{}); got != "no known vulnerabilities" {
		t.Errorf("empty = %q", got)
	}
	if got := scanSummary(api.PackageCounts{Critical: 2, Medium: 3, Attention: 2}); got != "2 to fix now, 5 in all (2 critical, 3 medium)" {
		t.Errorf("summary = %q", got)
	}
	for _, c := range []struct {
		p    api.Packages
		want string
	}{
		{api.Packages{}, ""},
		{api.Packages{Included: true}, ""},
		{api.Packages{Included: true, Scan: &api.PackageScan{Counts: api.PackageCounts{Low: 4}}}, "packages: nothing to fix now (whisk scan lists all)"},
		{api.Packages{Included: true, Scan: &api.PackageScan{Counts: api.PackageCounts{High: 3, Attention: 3}}}, "packages: 3 known vulnerabilities to fix now; run whisk scan"},
	} {
		if got := packagesLine(c.p); got != c.want {
			t.Errorf("packagesLine = %q, want %q", got, c.want)
		}
	}
}
