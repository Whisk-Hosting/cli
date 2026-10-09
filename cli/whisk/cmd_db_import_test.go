package whisk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// importAPI is the control plane and the object store for whisk db import: the form is
// authorised for the file's size, the store takes the post, and the restore names the import.
func importAPI(t *testing.T, uploaded *string, restoreBody *map[string]any) *httptest.Server {
	mux := http.NewServeMux()
	var srv *httptest.Server
	app := "/v1/orgs/acme/apps/crm"
	mux.HandleFunc("POST "+app+"/db/imports", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["bytes"] != float64(len("PGDMP")+4) {
			t.Errorf("authorised for %v bytes", body["bytes"])
		}
		writeJSON(w, 201, map[string]any{"id": "imp1", "url": srv.URL + "/store/bucket", "method": "POST",
			"fields": map[string]any{"key": "imports/a1/imp1.pgc", "policy": "p", "Content-Type": "application/octet-stream"}, "max_bytes": body["bytes"], "expires_at": "2026-10-09T03:00:00Z"})
	})
	mux.HandleFunc("POST /store/bucket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the store was sent the token")
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		*uploaded = string(b)
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST "+app+"/restore", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(restoreBody)
		writeJSON(w, 202, map[string]any{"id": "r9", "environment": "production", "import": "imp1", "swap": (*restoreBody)["swap"], "status": "queued", "created_at": "2026-10-09T02:00:00Z"})
	})
	mux.HandleFunc("GET "+app+"/restores/r9", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "r9", "environment": "production", "import": "imp1", "swap": true, "status": "done",
			"target_database": "app_a1", "previous_database": "app_a1_pre_1", "created_at": "2026-10-09T02:00:00Z"})
	})
	srv = httptest.NewServer(mux)
	return srv
}

// TestDBImport: a custom-format dump is uploaded through the form and loaded by a restore
// naming the import; --wait follows it to done.
func TestDBImport(t *testing.T) {
	defer func(old time.Duration) { restorePoll = old }(restorePoll)
	restorePoll = time.Millisecond
	var uploaded string
	var restoreBody map[string]any
	srv := importAPI(t, &uploaded, &restoreBody)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	dump := filepath.Join(dir, "houston.dump")
	must(t, os.WriteFile(dump, []byte("PGDMPdata"), 0o600))

	r := runRemote(t, dir, srv.URL, nil, "db", "import", dump, "--swap", "--wait", "--json")
	if r.Code != 0 {
		t.Fatalf("db import: %+v", r)
	}
	if uploaded != "PGDMPdata" {
		t.Errorf("the store got %q", uploaded)
	}
	if restoreBody["import"] != "imp1" || restoreBody["swap"] != true || restoreBody["at"] != nil {
		t.Errorf("restore body %v", restoreBody)
	}
	got := r.JSON["restore"].(map[string]any)
	if got["status"] != "done" || got["import"] != "imp1" {
		t.Errorf("result %v", got)
	}

	human := runRemote(t, dir, srv.URL, nil, "db", "import", dump)
	if human.Code != 0 || !strings.Contains(human.Stdout, "from import imp1") || !strings.Contains(human.Stderr, "upload: 100%") {
		t.Errorf("human: %+v", human)
	}
}

// TestDBImportRefusesOtherFiles: a plain SQL dump, a missing file and a directory are refused
// before anything is uploaded, with how to make a dump that loads.
func TestDBImportRefusesOtherFiles(t *testing.T) {
	var uploaded string
	var restoreBody map[string]any
	srv := importAPI(t, &uploaded, &restoreBody)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	plain := filepath.Join(dir, "plain.sql")
	must(t, os.WriteFile(plain, []byte("--\n-- PostgreSQL database dump\n"), 0o600))
	for _, path := range []string{plain, filepath.Join(dir, "missing.dump"), dir} {
		r := runRemote(t, dir, srv.URL, nil, "db", "import", path, "--json")
		if r.Code != 3 && r.Code != 1 || errCode(r) != "INVALID_REQUEST" {
			t.Errorf("%s: %+v", path, r)
		}
	}
	r := runRemote(t, dir, srv.URL, nil, "db", "import", plain, "--json")
	if !strings.Contains(r.Stdout+r.Stderr, "pg_dump -Fc") {
		t.Errorf("the refusal does not say how to make the dump: %+v", r)
	}
	if uploaded != "" || restoreBody != nil {
		t.Errorf("something was sent: %q %v", uploaded, restoreBody)
	}
}
