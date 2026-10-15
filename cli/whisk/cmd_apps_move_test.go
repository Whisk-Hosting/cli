package whisk

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
)

func TestRefusedAsAgent(t *testing.T) {
	cases := []struct {
		name string
		e    api.Error
		want bool
	}{
		{"an agent token's limit", api.Error{Code: "FORBIDDEN_ROLE", Details: map[string]any{"token": "agent"}}, true},
		{"a person without the role", api.Error{Code: "FORBIDDEN_ROLE"}, false},
		{"a person, signed in", api.Error{Code: "FORBIDDEN_ROLE", Details: map[string]any{"token": "user"}}, false},
		{"an agent identity", api.Error{Code: "AGENT_CATEGORY", Details: map[string]any{"required": "human"}}, true},
		{"an identity without a category", api.Error{Code: "AGENT_CATEGORY", Details: map[string]any{"required": "change"}}, false},
		{"an agent moving to a business not Whisk's own", api.Error{Code: "FORBIDDEN_ROLE", Details: map[string]any{"token": "agent", "org": "acme", "action": "app.move"}}, true},
		{"an identity without destroy", api.Error{Code: "AGENT_CATEGORY", Details: map[string]any{"required": "destroy"}}, false},
		{"anything else", api.Error{Code: "APP_MOVE_RUNNING"}, false},
	}
	for _, c := range cases {
		if got := refusedAsAgent(&c.e); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMoveLinks(t *testing.T) {
	if got, want := moveURL("https://whisk.run", "acme", "ledger", "demo"), "https://whisk.run/o/acme/apps/ledger/move?to=demo"; got != want {
		t.Errorf("moveURL = %q, want %q", got, want)
	}
	if got, want := movePage("https://whisk.run", "acme", "01J"), "https://whisk.run/o/acme/moves/01J"; got != want {
		t.Errorf("movePage = %q, want %q", got, want)
	}
	e := needsHumanMove("https://whisk.run", "acme", "ledger", "01J", "demo")
	if e.Code != "NEEDS_HUMAN" || e.Details["url"] != "https://whisk.run/o/acme/apps/ledger/move?to=demo" || e.Details["reason"] != "move_app" || e.Details["id"] != "01J" {
		t.Errorf("needsHumanMove: %+v", e)
	}
}

func TestMoveLines(t *testing.T) {
	done := api.AppMove{From: "mf", App: "ledger", To: "demo", NewApp: "ledger", URL: "https://ledger.demo.whisk.page", OldURL: "https://ledger.mf.whisk.page", Status: apitypes.AppMoveDone}
	if lines := moveLines(done, "p"); len(lines) != 2 || !strings.Contains(lines[0], "https://ledger.demo.whisk.page") || !strings.HasPrefix(lines[1], "https://ledger.mf.whisk.page now sends") {
		t.Errorf("done: %v", lines)
	}
	done.SecretsURL = "https://whisk.run/o/demo/apps/ledger/secrets?set=A"
	if lines := moveLines(done, "p"); len(lines) != 3 {
		t.Errorf("done with keys to set: %v", lines)
	}
	if lines := moveLines(api.AppMove{From: "mf", App: "ledger", Status: apitypes.AppMoveCancelled}, "p"); !strings.Contains(lines[0], "cancelled") {
		t.Errorf("cancelled: %v", lines)
	}
	if lines := moveLines(api.AppMove{From: "mf", App: "ledger", To: "demo", Status: apitypes.AppMoveRunning, Step: apitypes.MoveStepFiles}, "https://p"); !strings.Contains(lines[0], "https://p") {
		t.Errorf("going: %v", lines)
	}
}

func TestMoveFailed(t *testing.T) {
	var oe *output.Error
	err := moveFailed(api.AppMove{ID: "m1", Step: apitypes.MoveStepDatabase, Error: &werrors.Detail{Code: "SECRET_UNSET", Message: "A key has no value.", Fix: "Set it.", Details: map[string]any{"names": []string{"A"}}}}, "https://p")
	if !errors.As(err, &oe) || oe.Code != "SECRET_UNSET" || oe.Details["move"] != "m1" || oe.Details["url"] != "https://p" || !strings.Contains(oe.Fix, "https://p") {
		t.Errorf("a failed move's own error: %+v", err)
	}
	if err := moveFailed(api.AppMove{ID: "m1", Step: apitypes.MoveStepCode}, "https://p"); !errors.As(err, &oe) || oe.Code != "PLATFORM_UNAVAILABLE" {
		t.Errorf("a failed move without an error: %v", err)
	}
}

func TestAppsMove(t *testing.T) {
	agent := true
	polls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "app_1", "slug": "ledger", "hostname": "ledger.acme.whisk.page", "status": "active"})
	})
	mux.HandleFunc("POST /v1/orgs/acme/apps/{app}/move", func(w http.ResponseWriter, r *http.Request) {
		if agent {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "FORBIDDEN_ROLE", "message": "An agent token cannot do this.", "fix": "Do it yourself.", "details": map[string]any{"token": "agent", "role": "owner"}}})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "from": "acme", "app": "ledger", "to": "demo", "status": "queued", "step": "app"})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/{app}/move", func(w http.ResponseWriter, r *http.Request) {
		polls++
		out := map[string]any{"id": "m1", "from": "acme", "app": "ledger", "to": "demo", "status": "running", "step": "files"}
		if polls > 1 {
			out = map[string]any{"id": "m1", "from": "acme", "app": "ledger", "to": "demo", "new_app": "ledger", "url": "https://ledger.demo.whisk.page",
				"old_url": "https://ledger.acme.whisk.page", "status": "done", "step": "done"}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	movePoll = time.Millisecond

	r := runRemote(t, dir, srv.URL, nil, "apps", "move", "ledger")
	if r.Code == 0 || !strings.Contains(r.Stdout+r.Stderr, "INVALID_REQUEST") {
		t.Fatalf("no business named: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "apps", "move", "ledger", "--to", "demo", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code != 2 || e["code"] != "NEEDS_HUMAN" || !strings.HasSuffix(e["details"].(map[string]any)["url"].(string), "/o/acme/apps/ledger/move?to=demo") {
		t.Fatalf("an agent is given the link: %+v", r)
	}
	agent = false
	r = runRemote(t, dir, srv.URL, nil, "apps", "move", "ledger", "--to", "demo", "--json")
	move, _ := r.JSON["move"].(map[string]any)
	if r.Code != 0 || move["status"] != "done" || move["url"] != "https://ledger.demo.whisk.page" || polls < 2 {
		t.Fatalf("a person, or an agent between two of Whisk's own businesses, follows the move to its end: %+v", r)
	}
}
