package whisk

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/contract/apitypes"
)

// elsewhereAPI answers as a person who owns acme (crm, shop), develops in beta (site), only
// reads in gamma, and whose team business delta has no scanning.
func elsewhereAPI(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	finding := func(pkg, version, fixed, severity string) map[string]any {
		return map[string]any{"id": "GHSA-" + pkg, "package": pkg, "version": version, "fixed": fixed, "severity": severity,
			"where": "source", "path": "package-lock.json", "tool": "osv-scanner", "reach": "unknown"}
	}
	app := func(slug string, attention int, fs ...map[string]any) map[string]any {
		if fs == nil {
			fs = []map[string]any{}
		}
		return map[string]any{"app": slug, "name": strings.ToUpper(slug), "scan": map[string]any{"id": "ps-" + slug, "status": "done", "trigger": "daily",
			"counts": map[string]any{"high": attention, "attention": attention}, "findings": fs, "created_at": "2026-11-16T00:00:00Z", "finished_at": "2026-11-16T00:05:00Z"}}
	}
	mux.HandleFunc("GET /v1/orgs", func(w http.ResponseWriter, r *http.Request) {
		org := func(slug, role string) map[string]any {
			return map[string]any{"id": slug, "slug": slug, "name": slug, "plan": "business", "status": "active", "role": role, "created_at": "2026-01-01T00:00:00Z"}
		}
		writeJSON(w, 200, map[string]any{"items": []any{org("acme", "owner"), org("beta", "developer"), org("gamma", "member"), org("delta", "admin")}})
	})
	mux.HandleFunc("GET /v1/orgs/acme/packages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"included": true, "apps": []any{
			app("crm", 1, finding("lodash", "4.17.15", "4.17.21", "high")),
			app("shop", 2, finding("next", "14.0.0", "14.2.25", "critical"), finding("axios", "1.6.0", "1.7.4", "high")),
		}})
	})
	mux.HandleFunc("GET /v1/orgs/beta/packages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"included": true, "apps": []any{app("site", 0)}})
	})
	mux.HandleFunc("GET /v1/orgs/gamma/packages", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a business the person only reads in was asked")
	})
	mux.HandleFunc("GET /v1/orgs/delta/packages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"included": false, "apps": []any{}})
	})
	return httptest.NewServer(mux)
}

// whisk scan --all lists, for every app the person can deploy, each package to upgrade and the
// version that fixes it, and says which apps are clean and which businesses are not checked.
func TestScanAll(t *testing.T) {
	srv := elsewhereAPI(t)
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "scan", "--all", "--json")
	apps, _ := r.JSON["apps"].([]any)
	if r.Code != 0 || len(apps) != 2 || r.JSON["clean"] != float64(1) {
		t.Fatalf("scan --all --json: %+v", r)
	}
	if ni, _ := r.JSON["not_included"].([]any); len(ni) != 1 || ni[0] != "delta" {
		t.Errorf("not included: %v", r.JSON["not_included"])
	}

	r = runRemote(t, dir, srv.URL, nil, "scan", "--all")
	for _, want := range []string{"acme/crm", "acme/shop", "lodash", "4.17.21", "next", "14.2.25", "1 other app checked has nothing to fix now", "Business plan: delta"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("scan --all lacks %q:\n%s", want, r.Stdout)
		}
	}

	r = runRemote(t, dir, srv.URL, nil, "scan", "--all", "--now", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout, "INVALID_REQUEST") {
		t.Errorf("scan --all --now: %+v", r)
	}
}

// Doctor in one app sums up the person's other apps with packages to upgrade, leaving out the
// app it checks, and points at whisk scan --all; the summary is no warning of this repository.
func TestDoctorSumsUpOtherApps(t *testing.T) {
	srv := elsewhereAPI(t)
	defer srv.Close()
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("name: crm\n"), 0o644))

	// The bare manifest fails W002, so the report is the error's details, which still carry it.
	r := runRemote(t, dir, srv.URL, nil, "doctor", "--org", "acme", "--app", "crm", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	other, _ := details["other_apps"].(map[string]any)
	if other == nil || other["apps"] != float64(1) || other["packages"] != float64(2) || !strings.Contains(other["fix"].(string), "whisk scan --all") {
		t.Fatalf("doctor --json other_apps: %+v", r)
	}
	list, _ := other["list"].([]any)
	if first, _ := list[0].(map[string]any); first["app"] != "shop" || first["critical"] != float64(1) {
		t.Errorf("list: %v", list)
	}

	r = runRemote(t, dir, srv.URL, nil, "doctor", "--org", "acme", "--app", "crm")
	if !strings.Contains(r.Stdout, "1 other app you work on has 2 packages to upgrade (1 critical): acme/shop 2.") || !strings.Contains(r.Stdout, "whisk scan --all") {
		t.Errorf("doctor human lacks the other apps:\n%s", r.Stdout)
	}
}

func TestFixingOrgs(t *testing.T) {
	got := fixingOrgs([]apitypes.Org{
		{Slug: "a", Role: apitypes.RoleOwner, Status: "active"},
		{Slug: "b", Role: apitypes.RoleDeveloper, Status: "active"},
		{Slug: "c", Role: apitypes.RoleAdmin, Status: "active"},
		{Slug: "d", Role: apitypes.RoleBilling, Status: "active"},
		{Slug: "e", Role: apitypes.RoleMember, Status: "active"},
		{Slug: "f", Role: apitypes.RoleOwner, Status: "pending"},
	})
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("fixingOrgs = %v", got)
	}
}
