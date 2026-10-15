package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// whisk email status <id> reads the bound app's message by its id, says what its status means,
// lists each step, prints the answer under --json, and passes a refusal through with its code
// (CLI.md §5.8, CONTROL-PLANE.md §6.12 "Delivery").
func TestEmailStatus(t *testing.T) {
	var asked []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/email/sends/{id}", func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.EscapedPath())
		switch r.PathValue("id") {
		case "re_bounced":
			writeJSON(w, 200, map[string]any{"id": "re_bounced", "status": "bounced", "detail": "Permanent 550 no such user",
				"updated_at": "2026-10-16T09:02:00Z", "events": []any{
					map[string]any{"status": "sent", "at": "2026-10-16T09:00:00Z"},
					map[string]any{"status": "bounced", "at": "2026-10-16T09:02:00Z"}}})
		case "01ABC@mail.acme.example":
			writeJSON(w, 200, map[string]any{"id": "01ABC@mail.acme.example", "status": "delivered", "detail": "",
				"updated_at": "2026-10-16T09:01:00Z", "events": []any{
					map[string]any{"status": "sent", "at": "2026-10-16T09:00:00Z"},
					map[string]any{"status": "delivered", "at": "2026-10-16T09:01:00Z"}}})
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "EMAIL_SEND_NOT_FOUND",
				"message": "crm sent no message with id " + r.PathValue("id") + " in the last 30 days.",
				"fix":     "Send the id the app's POST …/email/send answered, with the token of the app that sent it, within 30 days of sending."}})
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path, "fix": "check the path"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "email", "status", "re_bounced")
	if r.Code != 0 || !strings.Contains(r.Stdout, "bounced: the receiving server refused it") || !strings.Contains(r.Stdout, "Detail: Permanent 550 no such user") ||
		!strings.Contains(r.Stdout, "STATUS") || strings.Count(r.Stdout, "\n") < 5 {
		t.Errorf("for a person: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "email", "status", "01ABC@mail.acme.example", "--json")
	send, _ := r.JSON["send"].(map[string]any)
	if r.Code != 0 || r.JSON["org"] != "acme" || r.JSON["app"] != "crm" || send["status"] != "delivered" || len(send["events"].([]any)) != 2 {
		t.Errorf("--json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "email", "status", "01ABC@mail.acme.example")
	if !strings.Contains(r.Stdout, "inbox or spam folder cannot be known") || strings.Contains(r.Stdout, "Detail:") {
		t.Errorf("a delivered message with no detail: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "email", "status", "re_nothing", "--json")
	if r.Code == 0 || errCode(r) != "EMAIL_SEND_NOT_FOUND" {
		t.Errorf("an unknown id: %+v", r)
	}
	if r := runRemote(t, dir, srv.URL, nil, "email", "status"); r.Code == 0 {
		t.Errorf("no id: %+v", r)
	}
	if len(asked) != 4 || asked[1] != "/v1/orgs/acme/apps/crm/email/sends/01ABC@mail.acme.example" {
		t.Errorf("asked %v", asked)
	}
}
