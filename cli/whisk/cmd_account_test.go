package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/config"
)

func TestExportFileName(t *testing.T) {
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC), "whisk-account-2026-10-07.json"},
		{time.Date(2026, 10, 7, 1, 2, 3, 0, time.FixedZone("NZDT", 13*3600)), "whisk-account-2026-10-06.json"},
		{time.Time{}, "whisk-account-export.json"},
	}
	for _, c := range cases {
		if got := exportFileName(c.in); got != c.want {
			t.Errorf("%v: %q, want %q", c.in, got, c.want)
		}
	}
}

// whisk account export saves the export only its owner can read; delete needs --yes, then says
// so, and a last owner hears which business holds them (CLI.md §5.1).
func TestAccountCommands(t *testing.T) {
	deleted := 0
	lastOwner := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/me/export", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"exported_at": "2026-10-07T01:02:03Z", "profile": map[string]any{"id": "u1"},
			"memberships": []any{map[string]any{"org_slug": "acme"}}, "sessions": []any{}, "audit": []any{map[string]any{}, map[string]any{}}})
	})
	mux.HandleFunc("DELETE /v1/me", func(w http.ResponseWriter, r *http.Request) {
		if lastOwner {
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "ACCOUNT_LAST_OWNER", "message": "You are the only owner of Acme (acme).", "fix": "Make someone else an owner."}})
			return
		}
		deleted++
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}
	store := &memStore{m: map[string]config.Credential{}}
	dir := t.TempDir()

	r := runCLI(t, dir, env, store, "account", "export", "--json")
	if r.Code != 0 || r.JSON["memberships"] != float64(1) || r.JSON["audit"] != float64(2) {
		t.Fatalf("export: %+v", r)
	}
	path := filepath.Join(dir, "whisk-account-2026-10-07.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the export file: %v %v", info, err)
	}
	raw, _ := os.ReadFile(path)
	var saved map[string]any
	if json.Unmarshal(raw, &saved) != nil || saved["profile"].(map[string]any)["id"] != "u1" {
		t.Errorf("saved: %s", raw)
	}

	r = runCLI(t, dir, env, store, "account", "delete", "--json")
	if r.Code != 2 || deleted != 0 {
		t.Fatalf("delete without --yes must ask a human: %+v", r)
	}
	r = runCLI(t, dir, env, store, "account", "delete", "--yes", "--json")
	if r.Code != 0 || r.JSON["deleted"] != true || deleted != 1 {
		t.Fatalf("delete: %+v", r)
	}
	lastOwner = true
	r = runCLI(t, dir, env, store, "account", "delete", "--yes", "--json")
	if r.Code == 0 || r.JSON["error"].(map[string]any)["code"] != "ACCOUNT_LAST_OWNER" {
		t.Fatalf("last owner: %+v", r)
	}
}
