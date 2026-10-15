package stub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/medialink"
)

// The stub's stand-in for the platform's image and media addresses and its signed links
// (CONTROL-PLANE.md §6.13, CONTRACT.md §13). The stub keeps no uploads, so /.whisk/img/<id>
// answers a plain grey picture of the size asked for and /.whisk/media/<id> answers that no
// file is kept. What it does as the platform does is decide who may have them: a link signed by
// POST …/uploads/links (under a key of the run's own, generation 1) is let in until it expires
// and refused when changed, and an address without a signature needs a sign-in.

const stubKeyGeneration = 1

// mediaPath reports whether a path is one of the addresses the platform answers itself.
func mediaPath(path string) bool {
	return strings.HasPrefix(path, "/.whisk/img/") || strings.HasPrefix(path, "/.whisk/media/")
}

// media answers /.whisk/img/… and /.whisk/media/… on the edge.
func (e *edge) media(w http.ResponseWriter, r *http.Request) {
	s := e.stub
	path := r.URL.Path
	q := r.URL.Query()
	signed := false
	var expires time.Time
	if medialink.Present(q) {
		var refused *werrors.Body
		expires, refused = s.judgeLink(path, q, time.Now())
		if refused != nil {
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, r, http.StatusForbidden, *refused)
			s.logf("%s %s -> 403 %s", r.Method, path, refused.Error.Code)
			return
		}
		signed = true
	} else if !e.signedIn(r) {
		w.Header().Set("Cache-Control", "private, no-store")
		if isNavigation(r) {
			// nosemgrep: go.lang.security.injection.open-redirect.open-redirect -- the local stub, never deployed
			http.Redirect(w, r, "/.whisk/login?return="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		writeError(w, r, http.StatusUnauthorized, werrors.New("AUTH_REQUIRED", "This upload is private to the people who use the app.",
			"Sign in at /.whisk/login, or have the app's server sign the address with POST …/uploads/links.", nil))
		return
	}
	if strings.HasPrefix(path, "/.whisk/media/") {
		w.Header().Set("Cache-Control", "no-store")
		writeError(w, r, http.StatusNotFound, werrors.New("NOT_FOUND", "The stub keeps no uploads, so there is no video or audio to play here.",
			"Play uploads on the platform; the stub only checks who may have them.", nil))
		return
	}
	wide, high, err := stubImageSize(q)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		writeError(w, r, http.StatusBadRequest, werrors.New("INVALID_REQUEST", err.Error(), "Ask for /.whisk/img/<id>?w=800, with w and h from 1 to 4096.", nil))
		return
	}
	maxAge := 300
	if signed {
		maxAge = min(maxAge, max(0, int(time.Until(expires)/time.Second)))
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", maxAge))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(greyPNG(wide, high))
}

// judgeLink decides a signed request as the platform does: the signature over exactly what the
// address shows, under this run's key, then the time.
func (s *stub) judgeLink(path string, q url.Values, now time.Time) (time.Time, *werrors.Body) {
	invalid := func(msg string) *werrors.Body {
		b := werrors.New("MEDIA_LINK_INVALID", msg, "Use the link exactly as POST …/uploads/links answered it, changing nothing in its query; ask for another size with a new link.", nil)
		return &b
	}
	resource, ok := medialink.Resource(path, q)
	if !ok {
		return time.Time{}, invalid("This address cannot be signed.")
	}
	c, err := medialink.Parse(q)
	if err != nil {
		return time.Time{}, invalid("The link's exp, kid or sig is missing or malformed.")
	}
	if c.Key != stubKeyGeneration {
		return time.Time{}, invalid("The link was signed with a key this app has since replaced.")
	}
	switch err := medialink.Verify(s.mediaKey, s.appID, resource, c, now); {
	case errors.Is(err, medialink.ErrExpired):
		at := time.Unix(c.Expires, 0).UTC()
		b := werrors.New("MEDIA_LINK_EXPIRED", "This link expired at "+at.Format(time.RFC3339)+".",
			"Draw the page again: the app issues a fresh link with POST …/uploads/links each time it shows the file.", map[string]any{"expired_at": at})
		return time.Time{}, &b
	case err != nil:
		return time.Time{}, invalid("The link's signature does not match what it asks for: it was changed, or it was made for another size, file or app.")
	}
	return time.Unix(c.Expires, 0), nil
}

// links is POST …/uploads/links on the stub's API: the platform's answer, signed with the run's
// key. The stub keeps no uploads, so it signs any address of the right form.
func (a *api) links(w http.ResponseWriter, r *http.Request) {
	s := a.stub
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("Signing links needs the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	var in struct {
		Paths     []string `json:"paths"`
		ExpiresIn int64    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The request body is not JSON.", `Send {"paths": ["/.whisk/img/<id>?w=800", "/.whisk/media/<id>"], "expires_in": 3600}.`, nil))
		return
	}
	ttl, ok := medialink.Lifetime(in.ExpiresIn)
	if !ok {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", fmt.Sprintf("expires_in %d is not from 60 to 43200 seconds.", in.ExpiresIn),
			"Leave expires_in out for an hour, or send the seconds the page needs the links for.", map[string]any{"expires_in": in.ExpiresIn}))
		return
	}
	if len(in.Paths) == 0 || len(in.Paths) > medialink.MaxLinks {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", fmt.Sprintf("Ask for 1 to %d links at once; this asked for %d.", medialink.MaxLinks, len(in.Paths)),
			"Split a longer list.", map[string]any{"paths": len(in.Paths), "max": medialink.MaxLinks}))
		return
	}
	exp := medialink.Expiry(time.Now(), ttl)
	out := apitypes.MediaLinks{ExpiresAt: exp, Links: make([]apitypes.MediaLink, 0, len(in.Paths))}
	for i, raw := range in.Paths {
		kind, id, q, err := medialink.Address(raw)
		if err == nil && kind == medialink.KindImage {
			_, _, err = stubImageSize(q)
		}
		if err != nil {
			writeError(w, r, 400, werrors.New("INVALID_REQUEST", err.Error()+".",
				"Send each address as the page writes it: /.whisk/img/<id>?w=800 for an image, /.whisk/media/<id> for video and audio.",
				map[string]any{"path": raw, "index": i}))
			return
		}
		path, _, _ := strings.Cut(raw, "?")
		resource, _ := medialink.Resource(path, q)
		signed := medialink.SignedPath(raw, exp.Unix(), stubKeyGeneration, medialink.Sign(s.mediaKey, s.appID, resource, exp.Unix(), stubKeyGeneration))
		out.Links = append(out.Links, apitypes.MediaLink{ID: id, Path: signed, URL: s.edgeURL + signed})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

// stubImageSize is the size the grey picture is drawn at: w and h from 1 to 4096, one
// following from the other at 4:3, 640×480 when neither is given.
func stubImageSize(q url.Values) (int, int, error) {
	side := func(name string) (int, error) {
		v := strings.TrimSpace(q.Get(name))
		if v == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 4096 {
			return 0, fmt.Errorf("%s=%s is not a whole number of pixels from 1 to 4096", name, v)
		}
		return n, nil
	}
	wide, err := side("w")
	if err != nil {
		return 0, 0, err
	}
	high, err := side("h")
	if err != nil {
		return 0, 0, err
	}
	switch {
	case wide == 0 && high == 0:
		return 640, 480, nil
	case high == 0:
		return wide, max(1, wide*3/4), nil
	case wide == 0:
		return max(1, high*4/3), high, nil
	}
	return wide, high, nil
}

// greyPNG is a plain grey picture of the size given.
func greyPNG(wide, high int) []byte {
	img := image.NewGray(image.Rect(0, 0, wide, high))
	for i := range img.Pix {
		img.Pix[i] = 0xc8
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
