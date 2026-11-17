package whisk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

// whisk call remembers each app's address on this computer for a while, so a call is one
// request to the app rather than a request to the platform for the address first. A
// remembered address that stops answering for the app is asked for again (callAddressStale).

// callAddressFor is how long a remembered address is used before the platform is asked again.
const callAddressFor = time.Hour

// callAddress is an app environment's address as this computer remembers it.
type callAddress struct {
	URL         string    `json:"url"`
	Environment string    `json:"environment"`
	Saved       time.Time `json:"saved"`
}

// callAddressPath is where the address of this platform's org/app/env is remembered. Pure.
func callAddressPath(cfgDir, apiBase, org, app, env string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%s", apiBase, org, app, env))
	return filepath.Join(cfgDir, "addresses", hex.EncodeToString(sum[:16])+".json")
}

// usable reports whether a remembered address may be used at now. Pure.
func (a callAddress) usable(now time.Time) bool {
	return a.URL != "" && a.Environment != "" && !a.Saved.After(now) && now.Sub(a.Saved) < callAddressFor
}

// callAddressStale reports whether an answer from a remembered address may mean the address is
// no longer the app's: the address could not be reached at all (no name, no connection, a
// certificate for another name), the edge knows no app there (404), or the server is not the
// one for that name (421). A request that timed out or broke after it was sent is not one: it
// may have been made, so it is never sent again. The caller asks the platform again and sends
// once more only when the address has changed, so a real 404 from the app costs one lookup and
// is never sent twice. Pure.
func callAddressStale(a callAnswer, unreached bool, sendErr error) bool {
	return unreached || (sendErr == nil && (a.Status == 404 || a.Status == 421))
}

func readCallAddress(path string) (callAddress, bool) {
	var a callAddress
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &a) != nil {
		return callAddress{}, false
	}
	return a, true
}

// writeCallAddress remembers an address; failing to only costs the next call a lookup.
func writeCallAddress(path string, a callAddress) {
	b, _ := json.Marshal(a)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tmp := path + ".tmp" + fmt.Sprint(os.Getpid())
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// callAddressResolver finds an app environment's address, remembered or asked for.
type callAddressResolver struct {
	s             *session
	client        *api.Client
	org, app, env string
	path          string
	now           func() time.Time

	// mu guards the address this run holds: a batch resolves it once and asks the platform
	// again at most once, whichever of its requests first finds it stale.
	mu        sync.Mutex
	url, name string
	cached    bool
	refreshed bool
}

func (s *session) callResolver(client *api.Client, org, app, env string) *callAddressResolver {
	path := ""
	if s.cfgDir != "" {
		path = callAddressPath(s.cfgDir, client.Base, org, app, env)
	}
	return &callAddressResolver{s: s, client: client, org: org, app: app, env: env, path: path, now: time.Now}
}

// address is the remembered address when there is a usable one, else the platform's answer,
// which is then remembered. cached says which.
func (r *callAddressResolver) address() (url, env string, cached bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.url != "" {
		return r.url, r.name, r.cached, nil
	}
	if r.path != "" {
		if a, ok := readCallAddress(r.path); ok && a.usable(r.now()) {
			r.url, r.name, r.cached = a.URL, a.Environment, true
			return r.url, r.name, true, nil
		}
	}
	if err := r.lookup(); err != nil {
		return "", "", false, err
	}
	return r.url, r.name, false, nil
}

// refresh asks the platform again after a remembered address seemed stale, once per run, and
// reports whether the address changed from was: only then is a request sent again.
func (r *callAddressResolver) refresh(was string) (url, env string, changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.refreshed && r.cached {
		r.refreshed = true
		if r.lookup() != nil {
			return "", "", false
		}
	}
	return r.url, r.name, r.url != "" && r.url != was
}

// lookup asks the platform for the address and remembers it. Holds mu.
func (r *callAddressResolver) lookup() error {
	url, env, err := r.s.appAddress(r.client, r.org, r.app, r.env)
	if err != nil {
		if r.path != "" {
			_ = os.Remove(r.path)
		}
		return err
	}
	r.url, r.name, r.cached = url, env, false
	if r.path != "" {
		writeCallAddress(r.path, callAddress{URL: url, Environment: env, Saved: r.now()})
	}
	return nil
}
