package whisk

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	werrors "github.com/whisk-run/contract/errors"
)

func TestRunningExport(t *testing.T) {
	id, ok := runningExport(&api.Error{Code: "EXPORT_IN_PROGRESS", Details: map[string]any{"job_id": "01EXP"}})
	if !ok || id != "01EXP" {
		t.Errorf("runningExport = %q, %v", id, ok)
	}
	if _, ok := runningExport(&api.Error{Code: "FORBIDDEN_ROLE"}); ok {
		t.Error("a refusal for another reason was taken for a running export")
	}
	if _, ok := runningExport(errors.New("connection refused")); ok {
		t.Error("a transport error was taken for a running export")
	}
}

func TestFollowExportSaysEachChangeAndStopsWhenItFinishes(t *testing.T) {
	var calls int32
	states := []string{"queued", "running", "running", "done"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orgs/acme/export/01EXP" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": r.URL.Path, "fix": "-"}})
			return
		}
		n := min(int(atomic.AddInt32(&calls, 1)), len(states)) - 1
		body := map[string]any{"id": "01EXP", "status": states[n]}
		if states[n] == "done" {
			body["url"] = "https://s3.example/exports/01EXP.tar.gz?sig"
			body["bytes"] = 4096
		}
		writeJSON(w, 200, body)
	}))
	defer srv.Close()

	var log bytes.Buffer
	client := api.New(srv.URL, "whsk_agent_test", "", "test")
	e, err := followExport(context.Background(), client, "acme", "01EXP", &log, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != "done" || e.URL == "" {
		t.Errorf("followed to %+v", e)
	}
	if got := strings.Count(log.String(), "\n"); got != 3 {
		t.Errorf("reported %d changes, want queued, running and done once each:\n%s", got, log.String())
	}
}

func TestExportFailedCarriesTheRecordedError(t *testing.T) {
	err := exportFailed(api.Export{ID: "01EXP", Status: "failed", Error: &werrors.Detail{
		Code: "EXPORT_FAILED", Message: "The export stopped at apps/notes/databases/production.dump",
		Fix: "Start the export again.", Details: map[string]any{"piece": "apps/notes/databases/production.dump"},
	}})
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != "EXPORT_FAILED" || oe.Details["piece"] == nil || oe.Details["id"] != "01EXP" {
		t.Errorf("exportFailed = %#v", err)
	}
}

// export show reads the export, which signs a new short-lived link, and prints the link with
// when it stops working; an export with no archive prints its state and no link.
func TestExportShowPrintsAFreshLinkAndWhenItStops(t *testing.T) {
	until := time.Date(2026, 10, 8, 12, 5, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/orgs/acme/export/01DONE":
			writeJSON(w, 200, map[string]any{"id": "01DONE", "status": "done", "bytes": 2048,
				"url": "https://s3.example/exports/01DONE.tar.gz?sig", "url_expires_at": until,
				"expires_at": until.Add(7 * 24 * time.Hour)})
		case "/v1/orgs/acme/export/01OLD":
			writeJSON(w, 200, map[string]any{"id": "01OLD", "status": "expired"})
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "That export does not exist.", "fix": "-"}})
		}
	}))
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	for _, tc := range []struct {
		name, id string
		code     int
		json     map[string]any
		absent   string
	}{
		{name: "ready", id: "01DONE", json: map[string]any{"status": "done", "url": "https://s3.example/exports/01DONE.tar.gz?sig", "url_expires_at": "2026-10-08T12:05:00Z"}},
		{name: "expired", id: "01OLD", json: map[string]any{"status": "expired"}, absent: "url"},
		{name: "gone, or no longer the caller's", id: "01GONE", code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runRemote(t, dir, srv.URL, nil, "export", "show", tc.id, "--json")
			if r.Code != tc.code {
				t.Fatalf("exit %d, want %d: %s %s", r.Code, tc.code, r.Stdout, r.Stderr)
			}
			if tc.code != 0 {
				if e, _ := r.JSON["error"].(map[string]any); e["code"] != "NOT_FOUND" {
					t.Errorf("refusal = %v", r.JSON)
				}
				return
			}
			for k, want := range tc.json {
				if r.JSON[k] != want {
					t.Errorf("%s = %v, want %v in %v", k, r.JSON[k], want, r.JSON)
				}
			}
			if tc.absent != "" && r.JSON[tc.absent] != nil {
				t.Errorf("%s present in %v", tc.absent, r.JSON)
			}
		})
	}

	r := runRemote(t, dir, srv.URL, nil, "export", "show", "01DONE")
	if r.Code != 0 || !strings.Contains(r.Stdout, "01DONE.tar.gz?sig") || !strings.Contains(r.Stdout, "whisk export show 01DONE signs a new one") {
		t.Errorf("human output: %s %s", r.Stdout, r.Stderr)
	}
}
