package stub

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/medialink"
)

// The stub signs media links as the platform does and its edge judges them as the platform's
// does: a signed link works without a sign-in, a changed or expired one is refused, and an
// unsigned address still needs a sign-in (CONTRACT.md §13, CONTROL-PLANE.md §6.13).
func TestMediaLinks(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	s.edgeURL = "http://127.0.0.1:3000"
	api, e := newAPI(s), newEdge(s)
	linksPath := "/v1/orgs/" + s.orgID + "/apps/" + s.appID + "/uploads/links"
	bearer := map[string]string{"Authorization": "Bearer " + s.serviceToken}

	if rec, _ := get(t, api, "POST", linksPath, nil, `{"paths":["/.whisk/img/01UP"]}`); rec.Code != 401 {
		t.Fatalf("without the service token: %d", rec.Code)
	}
	for _, body := range []string{
		`{"paths":[]}`, `{"paths":["/notes"]}`, `{"paths":["/.whisk/img/01UP?w=0"]}`,
		`{"paths":["/.whisk/img/01UP?w=1&sig=x"]}`, `{"paths":["/.whisk/img/01UP"],"expires_in":5}`, `nope`,
	} {
		if rec, out := get(t, api, "POST", linksPath, bearer, body); rec.Code != 400 || out == nil {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest("POST", linksPath, strings.NewReader(`{"paths":["/.whisk/img/01UP?w=200","/.whisk/media/01VID/embed"],"expires_in":600}`))
	req.Header.Set("Authorization", bearer["Authorization"])
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	var links apitypes.MediaLinks
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil || rec.Code != 200 || len(links.Links) != 2 {
		t.Fatalf("sign: %d %s", rec.Code, rec.Body.String())
	}
	img := links.Links[0]
	if img.ID != "01UP" || !strings.HasPrefix(img.Path, "/.whisk/img/01UP?w=200&exp=") || img.URL != s.edgeURL+img.Path {
		t.Fatalf("image link: %+v", img)
	}
	if life := time.Until(links.ExpiresAt); life > 10*time.Minute || life < 7*time.Minute {
		t.Errorf("a ten-minute link lives %s", life)
	}

	rec, _ = get(t, e, "GET", img.Path, nil, "")
	cfg, err := png.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if rec.Code != 200 || err != nil || cfg.Width != 200 || cfg.Height != 150 || !strings.HasPrefix(rec.Header().Get("Cache-Control"), "private, max-age=") {
		t.Fatalf("signed, signed out: %d %v %dx%d %s", rec.Code, err, cfg.Width, cfg.Height, rec.Header().Get("Cache-Control"))
	}
	for name, path := range map[string]string{
		"a larger size":   strings.Replace(img.Path, "w=200", "w=400", 1),
		"another upload":  strings.Replace(img.Path, "01UP", "01UQ", 1),
		"a later expiry":  strings.Replace(img.Path, "exp=", "exp=9", 1),
		"another key":     strings.Replace(img.Path, "kid=1", "kid=2", 1),
		"no signature":    img.Path[:strings.Index(img.Path, "&sig=")+5],
		"the media paths": strings.Replace(img.Path, "/.whisk/img/", "/.whisk/media/", 1),
	} {
		rec, out := get(t, e, "GET", path, map[string]string{"Cookie": sessionCookie + "=sess"}, "")
		if rec.Code != 403 || !strings.Contains(rec.Body.String(), "MEDIA_LINK_INVALID") || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s, even signed in: %d %v", name, rec.Code, out)
		}
	}
	past := time.Now().Add(-time.Minute).Unix()
	res, _ := medialink.Resource("/.whisk/img/01UP", map[string][]string{"w": {"200"}})
	expired := medialink.SignedPath("/.whisk/img/01UP?w=200", past, 1, medialink.Sign(s.mediaKey, s.appID, res, past, 1))
	if rec, _ := get(t, e, "GET", expired, nil, ""); rec.Code != 403 || !strings.Contains(rec.Body.String(), "MEDIA_LINK_EXPIRED") {
		t.Errorf("expired: %d %s", rec.Code, rec.Body.String())
	}
	if rec, _ := get(t, e, "GET", "/.whisk/img/01UP?w=200", nil, ""); rec.Code != 401 || !strings.Contains(rec.Body.String(), "AUTH_REQUIRED") {
		t.Errorf("unsigned, signed out: %d", rec.Code)
	}
	if rec, _ := get(t, e, "GET", "/.whisk/img/01UP?w=200", map[string]string{"Sec-Fetch-Mode": "navigate", "Accept": "text/html"}, ""); rec.Code != 302 {
		t.Errorf("unsigned navigation, signed out: %d", rec.Code)
	}
	if rec, _ := get(t, e, "GET", "/.whisk/img/01UP?h=90", map[string]string{"Cookie": sessionCookie + "=sess"}, ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("unsigned, signed in: %d", rec.Code)
	}
	if rec, _ := get(t, e, "GET", links.Links[1].Path, nil, ""); rec.Code != 404 || !strings.Contains(rec.Body.String(), "keeps no uploads") {
		t.Errorf("a signed media link verifies and finds nothing kept: %d %s", rec.Code, rec.Body.String())
	}
}
