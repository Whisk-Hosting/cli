package whisk

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whisk-run/contract/tokensig"

	"github.com/whisk-run/cli/internal/config"
)

// renewAPI answers a signed renewal and whoami, recording the order requests came in.
type renewAPI struct {
	mu      sync.Mutex
	pub     string
	seen    []string
	refuse  bool
	renewTo time.Time
}

func (a *renewAPI) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	verify := func(r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		pub, err := tokensig.ParsePublic(a.pub)
		return err == nil && tokensig.Verify(pub, r.Header.Get(tokensig.Header), r.Method, r.URL.RequestURI(), body, "whsk_agent_login", time.Now()) == nil
	}
	mux.HandleFunc("POST /v1/tokens/self/renew", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.seen = append(a.seen, "renew")
		a.mu.Unlock()
		if !verify(r) {
			t.Error("the renewal was not signed with this computer's key")
		}
		if a.refuse {
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "TOKEN_REVOKED", "message": "revoked", "fix": "Run whisk login."}})
			return
		}
		writeJSON(w, 200, map[string]any{"expires_at": a.renewTo})
	})
	mux.HandleFunc("GET /v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.seen = append(a.seen, "whoami")
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"user": map[string]any{"email": "ana@acme.example"}, "orgs": []any{},
			"token": map[string]any{"kind": "agent", "scopes": []string{"deploy"}, "bound": true}})
	})
	return httptest.NewServer(mux)
}

func boundLogin(t *testing.T, api string, expires time.Time) (*memStore, string) {
	t.Helper()
	pub, priv, err := tokensig.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return &memStore{m: map[string]config.Credential{"default": {Token: "whsk_agent_login", API: api, PrivateKey: priv, ExpiresAt: expires}}}, pub
}

// A login with under a week left renews itself, signed, before the command's own request, and
// the new expiry is kept with the profile; one with longer left is not renewed.
func TestALoginNearItsExpiryRenewsFirst(t *testing.T) {
	a := &renewAPI{renewTo: time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)}
	srv := a.server(t)
	defer srv.Close()
	store, pub := boundLogin(t, srv.URL, time.Now().Add(3*24*time.Hour))
	a.pub = pub

	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "whoami", "--json")
	if r.Code != 0 {
		t.Fatalf("whoami: %+v", r)
	}
	if got := strings.Join(a.seen, ","); got != "renew,whoami" {
		t.Fatalf("requests %s, want the renewal before whoami", got)
	}
	if got := store.m["default"].ExpiresAt; !got.Equal(a.renewTo) {
		t.Fatalf("stored expiry %v, want %v", got, a.renewTo)
	}

	a.seen = nil
	if r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "whoami", "--json"); r.Code != 0 {
		t.Fatalf("whoami: %+v", r)
	}
	if got := strings.Join(a.seen, ","); got != "whoami" {
		t.Fatalf("a renewed login renewed again: %s", got)
	}
}

// A renewal the platform refuses is silent: the command runs and says nothing of it, and the
// stored login is left as it was.
func TestARefusedRenewalNeverStopsTheCommand(t *testing.T) {
	a := &renewAPI{refuse: true}
	srv := a.server(t)
	defer srv.Close()
	expires := time.Now().Add(2 * 24 * time.Hour).UTC().Truncate(time.Second)
	store, pub := boundLogin(t, srv.URL, expires)
	a.pub = pub

	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_API": srv.URL}, store, "whoami", "--json")
	if r.Code != 0 || r.Stderr != "" {
		t.Fatalf("whoami after a refused renewal: %+v", r)
	}
	if got := strings.Join(a.seen, ","); got != "renew,whoami" {
		t.Fatalf("requests %s", got)
	}
	if got := store.m["default"].ExpiresAt; !got.Equal(expires) {
		t.Fatalf("stored expiry changed to %v", got)
	}
}
