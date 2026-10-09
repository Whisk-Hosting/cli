package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

// whisk operator reads the platform's lines, nodes and deploys with the filters it was given
// (CLI.md §5.13), and never needs a bound app or org.
func TestOperatorCommands(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/operator/logs":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
				{"at": "2026-09-29T04:00:00Z", "line": "idle sweep stopped crm", "stream": "stdout", "unit": "whisk-node.service", "host": "node-1"},
			}})
		case "/v1/operator/logs/labels":
			_, _ = w.Write([]byte(`{"units":["whisk-node.service","whiskd.service"],"hosts":["node-1"]}`))
		case "/v1/operator/nodes":
			_, _ = w.Write([]byte(`{"items":[{"id":"n1","name":"node-1","region":"eu","status":"ready","agent_version":"1.4.0","apps":7,"usage":{"free":{"line":"Free apps: 9.1 of 16 GB in use, 3 waits today"}}}]}`))
		case "/v1/operator/deploys":
			_, _ = w.Write([]byte(`{"items":[{"id":"d1","app_id":"a1","environment":"production","commit_sha":"abcdef123","status":"failed","error":{"code":"BUILD_FAILED"},"created_at":"2026-09-29T04:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	r := runRemote(t, dir, srv.URL, nil, "operator", "logs", "--unit", "whisk-node.service", "--grep", "idle", "--since", "6h")
	if r.Code != 0 || !strings.Contains(r.Stdout, "node-1/whisk-node") || !strings.Contains(r.Stdout, "idle sweep stopped crm") {
		t.Fatalf("logs: %+v", r)
	}
	if !strings.Contains(seen[0], "unit=whisk-node.service") || !strings.Contains(seen[0], "q=idle") || !strings.Contains(seen[0], "since=6h") {
		t.Errorf("logs asked %s", seen[0])
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "units")
	if r.Code != 0 || !strings.Contains(r.Stdout, "whisk-node.service, whiskd.service") || !strings.Contains(r.Stdout, "Hosts: node-1") {
		t.Fatalf("units: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "nodes")
	if r.Code != 0 || !strings.Contains(r.Stdout, "node-1") || !strings.Contains(r.Stdout, "1.4.0") || !strings.Contains(r.Stdout, "node-1: Free apps: 9.1 of 16 GB in use, 3 waits today") {
		t.Fatalf("nodes: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "operator", "deploys", "--status", "all", "--since", "7d")
	if r.Code != 0 || !strings.Contains(r.Stdout, "BUILD_FAILED") || !strings.Contains(r.Stdout, "abcdef1") {
		t.Fatalf("deploys: %+v", r)
	}
	if last := seen[len(seen)-1]; !strings.Contains(last, "status=all") || !strings.Contains(last, "since=7d") {
		t.Errorf("deploys asked %s", last)
	}
}

func TestFormatPlatformLine(t *testing.T) {
	at := time.Date(2026, 9, 29, 4, 0, 0, 0, time.Local)
	got := formatPlatformLine(api.LogLine{At: at, Line: "boom", Stream: "stderr", Unit: "whiskd.service", Host: "cp"})
	if got != "09-29 04:00:00.000 cp/whiskd stderr  boom" {
		t.Errorf("line = %q", got)
	}
	if got := formatPlatformLine(api.LogLine{At: at, Line: "hi", Unit: "whisk-caddy-1"}); got != "09-29 04:00:00.000 whisk-caddy-1  hi" {
		t.Errorf("container line = %q", got)
	}
}
