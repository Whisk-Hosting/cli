package stub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/cdnpurge"
	werrors "github.com/whisk-run/contract/errors"
)

// cdnPurge is POST /v1/orgs/{org}/apps/{app}/cdn/purge with the app's service token. No CDN
// serves an app under whisk dev, so a purge with good paths answers on false, as the platform
// does for an app whose CDN is off, and one with a bad path is refused as there.
func (a *api) cdnPurge(w http.ResponseWriter, r *http.Request) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("The CDN purge route needs the app's service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	var in apitypes.CdnPurgeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The body is not JSON.", `Send {"paths": ["/entries/42"]}, or {} for everything.`, nil))
		return
	}
	if err := cdnPurgeProblem(in.Paths); err != nil {
		writeError(w, r, 400, *err)
		return
	}
	writeJSON(w, 200, apitypes.CdnPurge{Urls: []string{}})
}

// cdnPurgeProblem is the refusal the platform gives the paths, nil when it takes them. Pure.
func cdnPurgeProblem(paths []string) *werrors.Body {
	if len(paths) > cdnpurge.MaxPaths {
		e := werrors.New("INVALID_REQUEST", fmt.Sprintf("A purge names at most %d paths; this one named %d.", cdnpurge.MaxPaths, len(paths)),
			"Send the rest in another purge, end a path in * to drop everything under it, or send no paths to drop the whole copy.", nil)
		return &e
	}
	for _, p := range paths {
		if why := cdnpurge.PathProblem(p); why != "" {
			e := werrors.New("INVALID_REQUEST", fmt.Sprintf("The path %q %s.", p, why), "Give paths such as /entries/42 or /entries/*.", nil)
			return &e
		}
	}
	return nil
}
