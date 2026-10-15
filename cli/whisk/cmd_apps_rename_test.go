package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/config"
)

func TestRenameManifest(t *testing.T) {
	cases := []struct {
		name, src, want string
		changed         bool
	}{
		{"a plain name", "name: crm\nruntime: go\n", "name: sales\nruntime: go\n", true},
		{"a quoted name", "name: \"crm\"\n", "name: sales\n", true},
		{"a comment after it stays", "# the app\nname: crm   # its address\nruntime: go\n", "# the app\nname: sales   # its address\nruntime: go\n", true},
		{"a name that needs quoting", "name: crm\n", "name: \"123\"\n", true},
		{"another app's name is left alone", "name: billing\n", "name: billing\n", false},
		{"a nested name is not the app's", "functions:\n  - name: crm\n", "functions:\n  - name: crm\n", false},
		{"no name", "runtime: go\n", "runtime: go\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next := "sales"
			if strings.Contains(c.want, "123") {
				next = "123"
			}
			got, changed := renameManifest([]byte(c.src), "crm", next)
			if string(got) != c.want || changed != c.changed {
				t.Fatalf("got %q (%v), want %q (%v)", got, changed, c.want, c.changed)
			}
		})
	}
}

func TestAppsRename(t *testing.T) {
	var asked string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": "crm", "hostname": "crm.acme.whisk.page", "status": "active"})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/{app}/rename", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Slug string `json:"slug"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Slug == "billing" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "This business already has an app called billing.", "fix": "Choose another slug."}})
			return
		}
		asked = r.PathValue("app") + " " + in.Slug
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": in.Slug, "hostname": in.Slug + ".acme.whisk.page", "status": "active"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "whisk.yaml"), []byte("name: crm # the address\nruntime: go\n"), 0o644))

	r := runRemote(t, dir, srv.URL, nil, "apps", "rename", "Not A Slug")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "INVALID_REQUEST") || asked != "" {
		t.Fatalf("a bad slug is refused before any call: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "rename", "billing", "--json")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "already has an app called billing") {
		t.Fatalf("the platform's refusal passes through: %+v", r)
	}
	if b, _, _ := config.LoadBinding(dir); b.App != "crm" {
		t.Fatalf("a refused rename leaves the binding: %+v", b)
	}

	r = runRemote(t, dir, srv.URL, nil, "apps", "rename", "sales", "--json")
	if r.Code != 0 || asked != "app_1 sales" || r.JSON["renamed"] != true || r.JSON["binding_updated"] != true || r.JSON["manifest_updated"] != true || r.JSON["old_url"] != "https://crm.acme.whisk.page" {
		t.Fatalf("apps rename: %+v", r)
	}
	if b, _, _ := config.LoadBinding(dir); b.Org != "acme" || b.App != "sales" {
		t.Fatalf("the directory is rebound: %+v", b)
	}
	if src, _ := os.ReadFile(filepath.Join(dir, "whisk.yaml")); string(src) != "name: sales # the address\nruntime: go\n" {
		t.Fatalf("whisk.yaml names the new slug: %q", src)
	}

	// Another directory, not bound to the app, is left as it is.
	other := t.TempDir()
	must(t, config.SaveBinding(other, config.Binding{Org: "acme", App: "billing", API: srv.URL}))
	r = runRemote(t, other, srv.URL, nil, "apps", "rename", "deals", "--app", "crm", "--org", "acme")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Renamed acme/crm to acme/deals") || !strings.Contains(r.Stdout, "now sends visitors there") {
		t.Fatalf("apps rename by flag: %+v", r)
	}
	if b, _, _ := config.LoadBinding(other); b.App != "billing" {
		t.Fatalf("another app's binding is left alone: %+v", b)
	}
}
