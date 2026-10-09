package whisk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/contract/apitypes"
)

func TestClientRowsAndSummary(t *testing.T) {
	items := []api.ClientOverview{
		{Slug: "harbour", Billing: "active", HandedOver: true, Apps: 2,
			Usage: apitypes.ClientUsage{StorageBytes: 1 << 30, Runs: 40}, Limits: apitypes.ClientUsage{StorageBytes: 10 << 30, Runs: 100000}},
		{Slug: "bay", Billing: "past_due", Apps: 1, Problems: 1, FailedDeploys: 2},
	}
	rows := clientRows(items)
	for i, want := range [][]string{
		{"harbour", "paid, handed over", "2", "-", "1.0 GB of 10.0 GB", "40 of 100000"},
		{"bay", "a payment failed", "1", "1 app broken, 2 failed deploys", "0 bytes of 0 bytes", "0 of 0"},
	} {
		if strings.Join(rows[i], "|") != strings.Join(want, "|") {
			t.Errorf("row %d = %q, want %q", i, rows[i], want)
		}
	}
	if got := clientsSummary(items); !strings.HasPrefix(got, "1 client with problems.") {
		t.Errorf("summary = %q", got)
	}
	if got := clientsSummary(items[:1]); got != "No problems." {
		t.Errorf("summary = %q", got)
	}
}

// clientsAPI answers the agency's client routes and records what was posted.
func clientsAPI(t *testing.T) (*httptest.Server, func() map[string]map[string]any) {
	var mu sync.Mutex
	got := map[string]map[string]any{}
	record := func(r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		got[r.URL.Path] = in
		mu.Unlock()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/studio/clients/overview", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"next_cursor": "", "items": []map[string]any{
			{"id": "o1", "slug": "harbour", "name": "Harbour", "status": "active", "billing": "active", "handed_over": true, "apps": 2,
				"problems": 1, "failed_deploys": 0, "usage": map[string]int64{"storage_bytes": 0, "runs": 3}, "limits": map[string]int64{"storage_bytes": 10 << 30, "runs": 100000}},
		}})
	})
	mux.HandleFunc("POST /v1/orgs/studio/clients", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, 202, map[string]any{"client": map[string]any{"id": "o2", "slug": "bay", "name": "Bay"}, "checkout_url": "https://checkout.stripe.test/c", "trial": false})
	})
	mux.HandleFunc("POST /v1/orgs/studio/clients/bay/handover", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, 201, map[string]any{"claim_url": "https://whisk.run/claim/abc", "contact": "jo@bay.example"})
	})
	srv := httptest.NewServer(mux)
	return srv, func() map[string]map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
}

func TestClientsCommands(t *testing.T) {
	srv, posted := clientsAPI(t)
	defer srv.Close()
	store := &memStore{m: map[string]config.Credential{}}
	env := map[string]string{"WHISK_API": srv.URL, "WHISK_TOKEN": "whsk_agent_test"}

	r := runCLI(t, t.TempDir(), env, store, "clients", "--org", "studio")
	for _, want := range []string{"harbour", "paid, handed over", "1 app broken", "1 client with problems."} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("clients output lacks %q: %+v", want, r)
		}
	}

	r = runCLI(t, t.TempDir(), env, store, "clients", "add", "Bay", "--contact", "jo@bay.example", "--yearly", "--no-free-month", "--org", "studio")
	for _, want := range []string{"Added Bay (bay).", "Open this to pay for it: https://checkout.stripe.test/c", "whisk init --org bay"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("clients add output lacks %q: %+v", want, r)
		}
	}
	if in := posted()["/v1/orgs/studio/clients"]; in["name"] != "Bay" || in["contact"] != "jo@bay.example" || in["interval"] != "year" || in["trial"] != false {
		t.Errorf("clients add posted %v", in)
	}

	r = runCLI(t, t.TempDir(), env, store, "clients", "handover", "bay", "--org", "studio", "--json")
	if r.Code != 0 || r.JSON["claim_url"] != "https://whisk.run/claim/abc" || r.JSON["contact"] != "jo@bay.example" {
		t.Fatalf("clients handover --json: %+v", r)
	}
	if in := posted()["/v1/orgs/studio/clients/bay/handover"]; len(in) != 0 {
		t.Errorf("handover without --contact posted %v", in)
	}
}
