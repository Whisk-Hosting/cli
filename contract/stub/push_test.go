package stub

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/webpush"
)

// The stub keeps a person's devices and their notifications as the platform takes them, with the
// platform's checks, and delivers nothing; the edge serves Whisk's push scripts and this run's
// key (CONTRACT.md §13).
func TestStubPush(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	a := newAPI(s)
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	auth := map[string]string{"Authorization": "Bearer " + s.serviceToken}
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	device := `{"user":"u1","label":"Chrome on Android","subscription":{"endpoint":"https://fcm.googleapis.com/fcm/send/abc","keys":{"p256dh":"` +
		base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()) + `","auth":"AAAAAAAAAAAAAAAAAAAAAA"}}}`

	if rec, _ := get(t, a, "POST", prefix+"/push/devices", nil, device); rec.Code != 401 {
		t.Errorf("no token: %d", rec.Code)
	}
	if rec, _ := get(t, a, "POST", prefix+"/push/devices", auth, device); rec.Code != 201 {
		t.Fatalf("device: %d %s", rec.Code, rec.Body)
	}
	bad := strings.Replace(device, "fcm.googleapis.com", "evil.example", 1)
	rec, _ := get(t, a, "POST", prefix+"/push/devices", auth, bad)
	var body werrors.Body
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 400 || body.Error.Code != "PUSH_SUBSCRIPTION_INVALID" {
		t.Errorf("another host: %d %s", rec.Code, rec.Body)
	}

	rec, _ = get(t, a, "POST", prefix+"/push/send", auth, `{"to":["u1","u2"],"title":"New mail","url":"/mail/1"}`)
	var sent apitypes.PushSent
	if err := json.Unmarshal(rec.Body.Bytes(), &sent); rec.Code != 202 || err != nil || sent.Devices != 1 || sent.Status != "queued" {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	rec, _ = get(t, a, "GET", prefix+"/push/sends/"+sent.ID, auth, "")
	var got apitypes.PushSend
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != 200 || err != nil || got.Status != "kept" || len(got.Deliveries) != 1 || got.Deliveries[0].User != "u1" {
		t.Errorf("status: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := get(t, a, "POST", prefix+"/push/send", auth, `{"to":"u1"}`); rec.Code != 400 {
		t.Errorf("no title: %d", rec.Code)
	}
	if rec, _ := get(t, a, "GET", prefix+"/push/sends/01UNKNOWN", auth, ""); rec.Code != 404 {
		t.Errorf("unknown send: %d", rec.Code)
	}
	if rec, _ := get(t, a, "DELETE", prefix+"/push/devices?user=u1", auth, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Errorf("remove: %d %s", rec.Code, rec.Body)
	}

	e := newEdge(s)
	for _, path := range []string{webpush.PagePath, webpush.WorkerPath, webpush.KeyPath, webpush.InstallPath} {
		rec, _ := get(t, e, "GET", path, nil, "")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}
