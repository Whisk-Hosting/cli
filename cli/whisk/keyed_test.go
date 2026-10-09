package whisk

import (
	"net/http"
	"strings"

	"github.com/whisk-run/contract/routes"
)

// missingKey reports whether a request the CLI sent should have carried an Idempotency-Key and
// did not: every write does, except one to a no-replay route, which the platform would run again
// whatever its key (routes.NoReplay).
func missingKey(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	return !routes.NoReplay(strings.TrimPrefix(r.URL.Path, "/v1")) && r.Header.Get("Idempotency-Key") == ""
}
