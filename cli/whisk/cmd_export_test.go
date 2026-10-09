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
