package whisk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/output"
)

func TestParseAppRefs(t *testing.T) {
	asked := 0
	org := func() (string, error) { asked++; return "acme", nil }
	cases := []struct {
		name string
		args []string
		want []appRef
		code string
	}{
		{"bare names take the current org", []string{"crm", "shop"}, []appRef{{"acme", "crm"}, {"acme", "shop"}}, ""},
		{"an org names its own", []string{"examples/visitors", "crm"}, []appRef{{"examples", "visitors"}, {"acme", "crm"}}, ""},
		{"an ID passes as it is", []string{"01J9Z8Q4K3M5N7P2R6T8V0W1X3"}, []appRef{{"acme", "01J9Z8Q4K3M5N7P2R6T8V0W1X3"}}, ""},
		{"no app after the org", []string{"acme/"}, nil, "INVALID_REQUEST"},
		{"no org before the app", []string{"/crm"}, nil, "INVALID_REQUEST"},
		{"too many parts", []string{"a/b/c"}, nil, "INVALID_REQUEST"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseAppRefs(c.args, org)
			if c.code != "" {
				if outputCode(err) != c.code {
					t.Fatalf("want %s, got %v", c.code, err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, c.want)
			}
		})
	}
	asked = 0
	if _, err := parseAppRefs([]string{"examples/visitors"}, org); err != nil || asked != 0 {
		t.Fatalf("an org was looked up when every name had its own: asked %d, %v", asked, err)
	}
	if _, err := parseAppRefs([]string{"crm"}, func() (string, error) { return "", errors.New("no org") }); err == nil {
		t.Fatal("a bare name with no org to act on was accepted")
	}
}

func outputCode(err error) string {
	var e *output.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestUniqueTargetsAndDeleteURL(t *testing.T) {
	ts := uniqueTargets([]deleteTarget{
		{org: "acme", id: "A1", slug: "crm"},
		{org: "acme", id: "A1", slug: "crm"},
		{org: "examples", id: "B2", slug: "visitors"},
	})
	if len(ts) != 2 {
		t.Fatalf("duplicates kept: %+v", ts)
	}
	if got := deleteURL("https://whisk.run", ts); got != "https://whisk.run/delete-apps?apps=acme:A1,examples:B2" {
		t.Fatalf("url: %s", got)
	}
}

// whisk apps delete looks each name up and prints one link naming the apps by ID; it never
// deletes anything itself.
func TestAppsDeletePrintsOneLinkByID(t *testing.T) {
	apps := map[string]map[string]any{
		"acme/crm":          {"id": "A1", "slug": "crm", "name": "CRM"},
		"acme/A1":           {"id": "A1", "slug": "crm", "name": "CRM"},
		"acme/shop":         {"id": "A2", "slug": "shop", "name": "Shop"},
		"examples/visitors": {"id": "B1", "slug": "visitors", "name": "Visitors"},
	}
	deletes := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/{org}/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		a, ok := apps[r.PathValue("org")+"/"+r.PathValue("app")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "The app was not found.", "fix": "Check the name."}})
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	})
	mux.HandleFunc("DELETE /", func(w http.ResponseWriter, r *http.Request) { deletes++; w.WriteHeader(204) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "apps", "delete", "crm", "A1", "shop", "examples/visitors", "--yes", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	details, _ := e["details"].(map[string]any)
	want := srv.URL + "/delete-apps?apps=acme:A1,acme:A2,examples:B1"
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || details["url"] != want {
		t.Fatalf("apps delete: %+v", r)
	}
	if got := len(details["apps"].([]any)); got != 3 {
		t.Fatalf("apps listed: %d, want 3", got)
	}
	if !strings.Contains(e["message"].(string), "acme/crm, acme/shop, examples/visitors") {
		t.Fatalf("message: %v", e["message"])
	}

	r = runRemote(t, dir, srv.URL, nil, "apps", "delete", "crm")
	if r.Code != 2 || !strings.Contains(r.Stdout+r.Stderr, srv.URL+"/delete-apps?apps=acme:A1") {
		t.Fatalf("human output should print the link: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "apps", "delete", "crm", "ghost", "--json")
	if r.Code == 2 || !strings.Contains(r.Stdout+r.Stderr, "NOT_FOUND") || strings.Contains(r.Stdout+r.Stderr, "delete-apps") {
		t.Fatalf("an unknown name should fail with no link: %+v", r)
	}
	if deletes != 0 {
		t.Fatalf("the CLI deleted %d apps itself", deletes)
	}
}
