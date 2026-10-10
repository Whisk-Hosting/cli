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
	if r.Code != 0 || r.JSON["included"] != true || scan == nil || len(scan["findings"].([]any)) != 3 {
		t.Fatalf("scan --json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "scan")
	for _, want := range []string{"crm: 1 to fix now, 2 in all (1 high, 1 low); 1 not called by your code.", "lodash", "4.17.19", "CVE-2020-8203", "package-lock.json", "busybox", "image", "whisk scan --now", "Not called by your code", "golang.org/x/text"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("scan human lacks %q:\n%s", want, r.Stdout)
		}
	}
	// The finding the code does not call is listed after the ones to fix, under its own heading.
	if i, j := strings.Index(r.Stdout, "Not called by your code"), strings.Index(r.Stdout, "golang.org/x/text"); i < 0 || j < i || strings.Index(r.Stdout, "lodash") > i {
		t.Errorf("not called findings are not set apart:\n%s", r.Stdout)
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
	for _, c := range []struct {
		counts api.PackageCounts
		want   string
	}{
		{api.PackageCounts{Critical: 2, Medium: 3, Attention: 2}, "2 to fix now, 5 in all (2 critical, 3 medium)"},
		{api.PackageCounts{High: 1, Attention: 1, NotCalled: 4}, "1 to fix now, 1 in all (1 high); 4 not called by your code"},
		{api.PackageCounts{NotCalled: 2}, "no known vulnerabilities your code calls; 2 not called by your code"},
	} {
		if got := scanSummary(c.counts); got != c.want {
			t.Errorf("summary = %q, want %q", got, c.want)
		}
	}
	called, notCalled := splitByReach([]api.PackageFinding{{ID: "a", Reach: "called"}, {ID: "b", Reach: "not_called"}, {ID: "c", Reach: "unknown"}, {ID: "d"}, {ID: "e", Reach: "not_called"}})
	ids := func(fs []api.PackageFinding) (out string) {
		for _, f := range fs {
			out += f.ID
		}
		return out
	}
	if ids(called) != "acd" || ids(notCalled) != "be" {
		t.Errorf("splitByReach = %q, %q", ids(called), ids(notCalled))
	}
	for _, c := range []struct {
		p    api.Packages
		want string
	}{
		{api.Packages{}, ""},
		{api.Packages{Included: true}, ""},
		{api.Packages{Included: true, Scan: &api.PackageScan{Counts: api.PackageCounts{Low: 4}}}, "packages: nothing to fix now (whisk scan lists all)"},
		{api.Packages{Included: true, Scan: &api.PackageScan{Counts: api.PackageCounts{High: 3, Attention: 3}}}, "packages: 3 known vulnerabilities to fix now; run whisk scan"},
		{api.Packages{Included: true, Scan: &api.PackageScan{Counts: api.PackageCounts{NotCalled: 5}}}, "packages: nothing to fix now (whisk scan lists all)"},
	} {
		if got := packagesLine(c.p); got != c.want {
			t.Errorf("packagesLine = %q, want %q", got, c.want)
		}
	}
}
