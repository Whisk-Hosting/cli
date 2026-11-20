package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/apitypes"
)

// whisk audit reads one page of the org's audit log with its filters, prints who did what and
// the cursor to read on, and refuses a --since or --limit it cannot send.
func TestAuditReadsOnePage(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/acme/audit" {
			http.NotFound(w, r)
			return
		}
		asked = append(asked, r.URL.RawQuery)
		at := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
		_ = json.NewEncoder(w).Encode(apitypes.Page[apitypes.AuditEvent]{Items: []apitypes.AuditEvent{
			{ID: "e1", ActorLabel: "Ann Lee", OnBehalfOf: "claude", Action: "deploy.created", TargetKind: "app", TargetID: "crm", At: at},
		}, NextCursor: "c2"})
	}))
	defer srv.Close()
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_NO_TELEMETRY": "1"}
	r := runCLI(t, t.TempDir(), env, nil, "audit", "--org", "acme", "--action", "deploy.created", "--since", "2026-11-19T00:00:00Z", "--limit", "20")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Ann Lee for claude") || !strings.Contains(r.Stdout, "deploy.created") || !strings.Contains(r.Stdout, "whisk audit --cursor c2") {
		t.Fatalf("code %d stdout %q stderr %q", r.Code, r.Stdout, r.Stderr)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "action=deploy.created") || !strings.Contains(asked[0], "limit=20") || !strings.Contains(asked[0], "since=2026-11-19T00%3A00%3A00Z") {
		t.Errorf("asked %v", asked)
	}
	r = runCLI(t, t.TempDir(), env, nil, "audit", "--org", "acme", "--json")
	if r.Code != 0 || r.JSON["next_cursor"] != "c2" || len(r.JSON["events"].([]any)) != 1 {
		t.Errorf("--json %+v", r)
	}
	for _, bad := range [][]string{{"--since", "last tuesday"}, {"--limit", "0"}} {
		if r := runCLI(t, t.TempDir(), env, nil, append([]string{"audit", "--org", "acme"}, bad...)...); r.Code == 0 || !strings.Contains(r.Stderr, "INVALID_REQUEST") {
			t.Errorf("%v: code %d %q", bad, r.Code, r.Stderr)
		}
	}
}
