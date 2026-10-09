package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
)

// feedbackAPI answers POST /v1/feedback, refusing a stale token as the platform would, and
// GET /v1/operator/feedback for the one token allowed to read it. It records what was sent.
func feedbackAPI(t *testing.T) (*httptest.Server, func() []recordedFeedback) {
	var mu sync.Mutex
	var got []recordedFeedback
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer whsk_agent_expired" {
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "TOKEN_EXPIRED", "message": "expired", "fix": "login"}})
			return
		}
		var in api.FeedbackInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		got = append(got, recordedFeedback{Auth: auth, In: in, UserAgent: r.Header.Get("User-Agent")})
		mu.Unlock()
		writeJSON(w, 201, map[string]any{"id": "01FB", "kind": "bug", "org": in.Org, "app": in.App, "redacted": 1,
			"created_at": "2026-09-29T03:00:00Z", "message": "Thank you. The Whisk team reads every piece of feedback, and it is how Whisk gets better."})
	})
	mux.HandleFunc("GET /v1/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "AUTH_REQUIRED", "message": "Listing your feedback needs a sign-in.", "fix": "login"}})
			return
		}
		if q := r.URL.Query(); q.Get("status") != "resolved" || q.Get("org") != "acme" {
			t.Errorf("own list query = %v", q)
		}
		writeJSON(w, 200, map[string]any{"next_cursor": "01FA", "items": []map[string]any{
			{"id": "01FB", "kind": "bug", "status": "resolved", "org": "acme", "app": "crm", "sent_by_you": true,
				"message": "whisk deploy printed HEALTH_CHECK_FAILED\nwith an empty excerpt", "context": map[string]string{},
				"created_at": "2026-09-29T03:00:00Z", "resolved_at": "2026-09-29T04:00:00Z", "note": "The excerpt now names\nthe port."},
			{"id": "01FC", "kind": "idea", "status": "resolved", "org": "acme", "sent_by_you": false,
				"message": "a seed command would help", "context": map[string]string{}, "created_at": "2026-09-29T02:00:00Z"}}})
	})
	mux.HandleFunc("GET /v1/feedback/{id}", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer whsk_agent_expired" {
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "TOKEN_EXPIRED", "message": "expired", "fix": "login"}})
			return
		}
		switch {
		case r.PathValue("id") == "01FB" && auth != "":
			writeJSON(w, 200, map[string]any{"id": "01FB", "kind": "bug", "status": "resolved", "org": "acme", "app": "crm", "sent_by_you": true,
				"message": "whisk deploy printed HEALTH_CHECK_FAILED", "context": map[string]string{}, "created_at": "2026-09-29T03:00:00Z",
				"resolved_at": "2026-09-29T04:00:00Z", "note": "The excerpt now names the port."})
		case r.PathValue("id") == "01FD":
			writeJSON(w, 200, map[string]any{"id": "01FD", "kind": "difficulty", "status": "open", "sent_by_you": false,
				"message": "install.sh found no writable directory", "context": map[string]string{}, "created_at": "2026-09-29T03:00:00Z"})
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That feedback was not found.", "fix": "check"}})
		}
	})
	mux.HandleFunc("GET /v1/operator/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer whsk_agent_reader" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That route was not found.", "fix": "check"}})
			return
		}
		if q := r.URL.Query(); q.Get("status") != "all" || q.Get("kind") != "bug" || q.Get("org") != "acme" || q.Get("limit") != "5" {
			t.Errorf("list query = %v", q)
		}
		writeJSON(w, 200, map[string]any{"open": 3, "next_cursor": "01FA", "items": []map[string]any{{
			"id": "01FB", "kind": "bug", "status": "open", "org": "acme", "app": "crm", "actor_kind": "agent", "email": "ana@acme.example",
			"message": "whisk deploy printed HEALTH_CHECK_FAILED\nwith an empty excerpt", "context": map[string]string{"code": "HEALTH_CHECK_FAILED"},
			"created_at": "2026-09-29T03:00:00Z"}}})
	})
	mux.HandleFunc("POST /v1/operator/feedback/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer whsk_agent_resolver":
		case "Bearer whsk_agent_reader":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "TOKEN_SCOPE", "message": "An operator read token only reads; setting a piece of feedback's status needs feedback:resolve.", "fix": "Ask the operator."}})
			return
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That route was not found.", "fix": "check"}})
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		got = append(got, recordedFeedback{Auth: r.Header.Get("Authorization"), In: api.FeedbackInput{Message: strings.TrimSpace(r.PathValue("id") + " " + in["status"] + " " + in["note"])}})
		mu.Unlock()
		out := map[string]any{"id": r.PathValue("id"), "kind": "bug", "status": in["status"], "actor_kind": "agent", "message": "m", "context": map[string]string{}, "created_at": "2026-09-29T03:00:00Z", "note": in["note"]}
		if in["status"] == "resolved" {
			out["resolved_at"] = "2026-09-29T04:00:00Z"
		}
		writeJSON(w, 200, out)
	})
	return httptest.NewServer(mux), func() []recordedFeedback { mu.Lock(); defer mu.Unlock(); return append([]recordedFeedback{}, got...) }
}

type recordedFeedback struct {
	Auth      string
	UserAgent string
	In        api.FeedbackInput
}

func TestFeedbackSendsWithContext(t *testing.T) {
	srv, sent := feedbackAPI(t)
	defer srv.Close()
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, ".whisk"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".whisk", "app.json"), []byte(`{"org":"acme","app":"crm"}`), 0o644))
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test", "WHISK_AGENT": "claude-code"}
	r := runCLI(t, dir, env, &memStore{m: map[string]config.Credential{}},
		"feedback", "the health check excerpt", "was empty", "--kind", "bug", "--code", "health_check_failed", "--command", "whisk deploy", "--json")
	if r.Code != 0 || r.JSON["id"] != "01FB" || !strings.Contains(r.JSON["message"].(string), "Thank you") {
		t.Fatalf("feedback: %+v", r)
	}
	got := sent()
	if len(got) != 1 {
		t.Fatalf("sent %d", len(got))
	}
	in := got[0].In
	if got[0].Auth != "Bearer whsk_agent_test" || in.Message != "the health check excerpt was empty" || in.Kind != "bug" || in.Org != "acme" || in.App != "crm" {
		t.Errorf("sent %+v", got[0])
	}
	if in.Context["code"] != "HEALTH_CHECK_FAILED" || in.Context["command"] != "whisk deploy" || in.Context["agent"] != "claude-code" || in.Context["cli_version"] != "dev" || in.Context["os"] == "" {
		t.Errorf("context %v", in.Context)
	}

	// The send subcommand is the same thing, which is what an MCP client calls.
	r = runCLI(t, dir, env, &memStore{m: map[string]config.Credential{}}, "feedback", "send", "doctor explained the port rule well", "--kind", "praise")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Thank you") || !strings.Contains(r.Stdout, "01FB") || !strings.Contains(r.Stdout, "1 value that looked like credentials was masked") {
		t.Fatalf("feedback send: %+v", r)
	}
}

// Feedback matters most when login is what failed: with no token, or a token the platform
// refuses, it is sent anonymously.
func TestFeedbackWithoutAWorkingLogin(t *testing.T) {
	srv, sent := feedbackAPI(t)
	defer srv.Close()
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, &memStore{m: map[string]config.Credential{}}, "feedback", "whisk login never showed a code", "--json")
	if r.Code != 0 {
		t.Fatalf("anonymous feedback: %+v", r)
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_expired"}, &memStore{m: map[string]config.Credential{}}, "feedback", "my token expired mid-deploy", "--json")
	if r.Code != 0 {
		t.Fatalf("feedback with an expired token: %+v", r)
	}
	got := sent()
	if len(got) != 2 || got[0].Auth != "" || got[1].Auth != "" {
		t.Fatalf("both should arrive without a token: %+v", got)
	}
}

func TestFeedbackNeedsAMessage(t *testing.T) {
	r := runCLI(t, t.TempDir(), nil, &memStore{m: map[string]config.Credential{}}, "feedback", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" {
		t.Fatalf("empty feedback: %+v", r)
	}
	if msg, err := feedbackMessage([]string{"-"}, strings.NewReader("  from stdin\n")); err != nil || msg != "from stdin" {
		t.Errorf("stdin message = %q %v", msg, err)
	}
}

// An agent lists its own feedback and its org's, with where each stands and the team's note.
func TestFeedbackListOwn(t *testing.T) {
	srv, _ := feedbackAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}
	args := []string{"feedback", "list", "--status", "resolved", "--org", "acme"}
	r := runCLI(t, t.TempDir(), env, store, append(args, "--json")...)
	items, _ := r.JSON["items"].([]any)
	if r.Code != 0 || len(items) != 2 || r.JSON["next_cursor"] != "01FA" || items[0].(map[string]any)["note"] != "The excerpt now names\nthe port." {
		t.Fatalf("list --json: %+v", r)
	}
	r = runCLI(t, t.TempDir(), env, store, args...)
	for _, want := range []string{"01FB", "acme/crm", "resolved", "you", "your org", "01FB, from the Whisk team: The excerpt now names the port.", "--cursor 01FA", "whisk feedback show"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("list output lacks %q:\n%s", want, r.Stdout)
		}
	}
}

// show reads one piece; one sent without signing in is found by its id without a token, and an
// expired token falls back to that.
func TestFeedbackShow(t *testing.T) {
	srv, _ := feedbackAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}, store, "feedback", "show", "01FB")
	for _, want := range []string{"Feedback 01FB, a bug", "about acme/crm", "is resolved", "From the Whisk team: The excerpt now names the port."} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("show output lacks %q: %+v", want, r)
		}
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "feedback", "show", "01FD", "--json")
	if r.Code != 0 || r.JSON["status"] != "open" {
		t.Fatalf("show without a login: %+v", r)
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_expired"}, store, "feedback", "show", "01FD")
	if r.Code != 0 || !strings.Contains(r.Stdout, "has not resolved it yet") {
		t.Fatalf("show with an expired token: %+v", r)
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}, store, "feedback", "show", "01FZ", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("show someone else's: %+v", r)
	}
}

func TestFeedbackListEveryone(t *testing.T) {
	srv, _ := feedbackAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	args := []string{"feedback", "list", "--everyone", "--status", "all", "--kind", "bug", "--org", "acme", "--limit", "5"}
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_reader"}, store, append(args, "--json")...)
	if r.Code != 0 || r.JSON["open"] != float64(3) || len(r.JSON["items"].([]any)) != 1 || r.JSON["next_cursor"] != "01FA" {
		t.Fatalf("list --json: %+v", r)
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_reader"}, store, args...)
	for _, want := range []string{"01FB", "acme/crm", "ana@acme.example (agent)", "HEALTH_CHECK_FAILED with an empty", "3 open in all.", "--cursor 01FA"} {
		if !strings.Contains(r.Stdout, want) {
			t.Errorf("list output lacks %q:\n%s", want, r.Stdout)
		}
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}, store, "feedback", "list", "--everyone", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "NOT_FOUND" {
		t.Fatalf("a token without feedback:read: %+v", r)
	}
}

func TestFeedbackRows(t *testing.T) {
	rows := feedbackRows([]api.Feedback{
		{ID: "1", Kind: "idea", Status: "open", ActorKind: "anonymous", Message: strings.Repeat("a", 80), CreatedAt: time.Now()},
		{ID: "2", Kind: "bug", Status: "resolved", Org: "acme", ActorKind: "user", Email: "ana@acme.example", Message: "short", CreatedAt: time.Now()},
	})
	if rows[0][4] != "-" || rows[0][5] != "anonymous" || len(rows[0][6]) > 64 {
		t.Errorf("anonymous row %v", rows[0])
	}
	if rows[1][4] != "acme" || rows[1][5] != "ana@acme.example" {
		t.Errorf("person row %v", rows[1])
	}
}

// resolve and reopen set a piece of feedback's status with a token scoped to feedback:resolve;
// a plain read token is told which scope it lacks.
func TestFeedbackResolveAndReopen(t *testing.T) {
	srv, sent := feedbackAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_resolver"}
	r := runCLI(t, t.TempDir(), env, store, "feedback", "resolve", "01FB", "--note", "Fixed in the next release.", "--json")
	if r.Code != 0 || r.JSON["id"] != "01FB" || r.JSON["status"] != "resolved" || r.JSON["resolved_at"] == nil || r.JSON["note"] != "Fixed in the next release." {
		t.Fatalf("resolve --json: %+v", r)
	}
	r = runCLI(t, t.TempDir(), env, store, "feedback", "reopen", "01FB")
	if r.Code != 0 || !strings.Contains(r.Stdout, "Feedback 01FB is open.") {
		t.Fatalf("reopen: %+v", r)
	}
	got := sent()
	if len(got) != 2 || got[0].In.Message != "01FB resolved Fixed in the next release." || got[1].In.Message != "01FB open" {
		t.Errorf("sent %+v", got)
	}
	r = runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_reader"}, store, "feedback", "resolve", "01FB", "--json")
	if r.Code == 0 || r.JSON["error"].(map[string]any)["code"] != "TOKEN_SCOPE" {
		t.Fatalf("a read token without feedback:resolve: %+v", r)
	}
	r = runCLI(t, t.TempDir(), env, store, "feedback", "resolve", "--json")
	if r.Code == 0 {
		t.Fatalf("resolve without an id: %+v", r)
	}
}
