package whisk

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

// The argument of whisk errors is a platform code in any case, or a group's id from the list,
// which is pointed at whisk errors show rather than answered NOT_FOUND.
func TestErrorsArgOf(t *testing.T) {
	for arg, want := range map[string]errorsArg{
		"BUILD_FAILED": errorsArgCode,
		"build_failed": errorsArgCode,
		"62":           errorsArgGroup,
		"0":            errorsArgGroup,
		"BUILD_FAILD":  errorsArgUnknown,
		"6a":           errorsArgUnknown,
		"":             errorsArgUnknown,
	} {
		if got := errorsArgOf(arg); got != want {
			t.Errorf("errorsArgOf(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestErrorShowLines(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	at := now.Add(-5 * time.Minute)
	group := &api.ErrorGroup{ID: "62", Title: "TypeError: x is undefined", Culprit: "src/orders.ts in total", Status: "unresolved", Count: 3, Users: 1, FirstSeen: now.Add(-48 * time.Hour), LastSeen: at}
	full := api.ErrorSample{
		ID: "e3", At: at, Message: "TypeError: x is undefined", RequestID: "01J9F7N2WC1MGJYC54CHS3J8MP", Group: group,
		Frames: []api.ErrorFrame{{Function: "handle", File: "node_modules/hono/dist/hono.js", Line: 80}, {Function: "total", File: "src/orders.ts", Line: 12, InApp: true}},
		Recent: []api.ErrorOccurrence{{ID: "e3", At: at, Message: "TypeError: x is undefined", RequestID: "01J9F7N2WC1MGJYC54CHS3J8MP"}, {ID: "e1", At: at.Add(-time.Hour), Message: "TypeError: x is undefined"}},
	}
	resolved := *group
	resolved.Status = "resolved"
	for _, tc := range []struct {
		name      string
		in        api.ErrorSample
		want, not []string
	}{
		{"everything", full, []string{
			"TypeError: x is undefined",
			"Group 62, unresolved: 3 times (1 user), first seen 2d ago, last seen 5m ago.",
			"Where: src/orders.ts in total",
			"Newest occurrence e3, 5m ago (2026-11-09 11:55:00Z).",
			"Request: 01J9F7N2WC1MGJYC54CHS3J8MP. Its trace: whisk traces show 01J9F7N2WC1MGJYC54CHS3J8MP.",
			"    handle  node_modules/hono/dist/hono.js:80",
			"  > total  src/orders.ts:12",
			"Recent occurrences, newest first:",
			"  2026-11-09 10:55:00Z  -",
			"whisk errors --resolve 62",
		}, []string{"Message:", "No stack"}},
		{"no request, no stack, no recent", api.ErrorSample{ID: "e1", At: at, Message: "boom", Group: group}, []string{
			"Message: boom", "Request: none recorded.", "No stack: the error carried none.",
		}, []string{"Recent occurrences", "> "}},
		{"resolved", api.ErrorSample{ID: "e1", At: at, Group: &resolved}, []string{"It is resolved", "whisk errors --ignore 62"}, []string{"--resolve"}},
		{"a server without the group", api.ErrorSample{ID: "e1", At: at, Message: "boom"}, []string{"boom", "Newest occurrence e1"}, []string{"Group 62"}},
		{"nothing at all", api.ErrorSample{}, []string{"Error group 62", "Newest occurrence -, never (-)."}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(errorShowLines("62", tc.in, now), "\n")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in\n%s", w, got)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in\n%s", n, got)
				}
			}
		})
	}
}

// whisk errors lists groups by an id that whisk errors show opens, with the stack, the request
// id and the recent occurrences in --json; a group's id given to whisk errors points there
// (CLI.md §5.5).
func TestErrorsShow(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/v1/orgs/acme/apps/crm/errors":
			writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "62", "title": "Error: boom", "status": "unresolved", "count": 2, "users": 0, "first_seen": "2026-11-09T10:00:00Z", "last_seen": "2026-11-09T10:03:00Z"}}})
		case "/v1/orgs/acme/apps/crm/errors/62":
			writeJSON(w, 200, map[string]any{
				"id": "e3", "at": "2026-11-09T10:03:00Z", "message": "Error: boom", "request_id": "01REQ3",
				"frames": []map[string]any{{"function": "crash", "file": "main.go", "line": 12, "in_app": true}},
				"tags":   map[string]any{"request_id": "01REQ3"},
				"group":  map[string]any{"id": "62", "title": "Error: boom", "status": "unresolved", "count": 2, "users": 0, "first_seen": "2026-11-09T10:00:00Z", "last_seen": "2026-11-09T10:03:00Z"},
				"recent": []map[string]any{{"id": "e3", "at": "2026-11-09T10:03:00Z", "message": "Error: boom", "request_id": "01REQ3"}, {"id": "e1", "at": "2026-11-09T10:00:00Z", "message": "Error: boom"}},
			})
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "This app has no error group 99.", "fix": "List the app's error groups with whisk errors."}})
		}
	}))
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "errors")
	if r.Code != 0 || !strings.Contains(r.Stdout, "62") || !strings.Contains(r.Stdout, "whisk errors show 62") {
		t.Fatalf("list: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "errors", "show", "62", "--json")
	if r.Code != 0 || r.JSON["request_id"] != "01REQ3" || r.JSON["id"] != "62" {
		t.Fatalf("show --json: %+v", r)
	}
	latest, _ := r.JSON["latest"].(map[string]any)
	frames, _ := latest["frames"].([]any)
	recent, _ := r.JSON["recent"].([]any)
	group, _ := r.JSON["group"].(map[string]any)
	if len(frames) != 1 || len(recent) != 2 || group["title"] != "Error: boom" || latest["id"] != "e3" {
		t.Errorf("show --json carries frames %v, recent %v, group %v, latest %v", frames, recent, group, latest)
	}

	r = runRemote(t, dir, srv.URL, nil, "errors", "show", "62")
	if r.Code != 0 || !strings.Contains(r.Stdout, "> crash  main.go:12") || !strings.Contains(r.Stdout, "whisk traces show 01REQ3") {
		t.Fatalf("show: %+v", r)
	}

	r = runRemote(t, dir, srv.URL, nil, "errors", "show", "99", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "NOT_FOUND" {
		t.Fatalf("show of a missing group: %+v", r)
	}

	before := len(seen)
	r = runRemote(t, dir, srv.URL, nil, "errors", "62", "--json")
	e, _ := r.JSON["error"].(map[string]any)
	if r.Code == 0 || e["code"] != "NOT_AN_ERROR_CODE" || !strings.Contains(e["fix"].(string), "whisk errors show 62") {
		t.Fatalf("errors 62: %+v", r)
	}
	if len(seen) != before {
		t.Errorf("errors 62 asked the platform: %v", seen[before:])
	}

	r = runRemote(t, dir, srv.URL, nil, "errors", "BUILD_FAILED", "--json")
	if r.Code != 0 || r.JSON["code"] != "BUILD_FAILED" {
		t.Fatalf("errors BUILD_FAILED: %+v", r)
	}
}
