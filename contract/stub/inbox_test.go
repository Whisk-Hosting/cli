package stub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/inbound"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/webhook"
)

const inboxManifest = `
whisk: 1
name: lab-results
inbox:
  handler: /inbound/email
  allow_from: ["@lab.example"]
`

const labMessage = "From: Lab <results@lab.example>\r\nTo: lab-results.dev+batch@in.localhost\r\nSubject: Batch 12\r\n" +
	"Message-ID: <b12@lab.example>\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n" +
	"--x\r\nContent-Type: text/plain\r\n\r\nResults attached.\r\n" +
	"--x\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"batch-12.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n--x--\r\n"

// The stub's inbox: a raw message posted to /v1/stub/inbox reaches the handler signed, as a
// stored event of the inbox source, with files the app reads back with its service token.
func TestInbox(t *testing.T) {
	type hit struct {
		h    http.Header
		body []byte
	}
	seen := make(chan hit, 4)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- hit{r.Header.Clone(), body}
		w.WriteHeader(204)
	}))
	defer app.Close()
	m, err := manifest.Parse([]byte(inboxManifest))
	if err != nil {
		t.Fatal(err)
	}
	up, _ := url.Parse(app.URL)
	s := &stub{manifest: m, orgID: newULID(), appID: newULID(), upstream: up, serviceToken: "whsk_service_test",
		deliveryKey: randomBytes(32), deliveryMarker: randomToken(16), secrets: map[string]string{}, urlTokens: map[string]string{},
		bodyLimit: 8 << 20, store: newStore()}
	s.hooks = newHooks(s)
	internal := httptest.NewServer(newInternal(s))
	defer internal.Close()
	s.internalURL, s.apiURL = internal.URL, "http://api.test"
	a := newAPI(s)

	rec, _ := get(t, a, "POST", "/v1/stub/inbox", nil, labMessage)
	if rec.Code != 202 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var got hit
	select {
	case got = <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery")
	}
	if got.h.Get("X-Whisk-Webhook-Source") != inbound.SourceName || webhook.VerifyDelivery(s.deliveryKey, got.h, got.body) != "" {
		t.Fatalf("delivery not signed as the inbox's: %v", got.h)
	}
	var d inbound.Delivery
	if err := json.Unmarshal(got.body, &d); err != nil {
		t.Fatal(err)
	}
	if d.Subject != "Batch 12" || d.FromAddress != "results@lab.example" || d.Tag != "batch" || len(d.Attachments) != 1 || d.Raw.UploadID == "" {
		t.Fatalf("delivery: %s", got.body)
	}
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	rec, _ = get(t, a, "GET", prefix+"/uploads/"+d.Attachments[0].UploadID, nil, "")
	if rec.Code != 401 {
		t.Fatalf("an upload without the service token: %d", rec.Code)
	}
	rec, out := get(t, a, "GET", prefix+"/uploads/"+d.Raw.UploadID, map[string]string{"Authorization": "Bearer whsk_service_test"}, "")
	if rec.Code != 200 || !strings.HasPrefix(out["url"], "http://api.test/v1/stub/files/") {
		t.Fatalf("raw upload: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, a, "GET", strings.TrimPrefix(out["url"], "http://api.test"), nil, "")
	if rec.Body.String() != labMessage {
		t.Fatalf("the stored message differs from what was sent")
	}

	// A sender the inbox does not allow is kept, not delivered.
	rec, _ = get(t, a, "POST", "/v1/stub/inbox", nil, strings.Replace(labMessage, "results@lab.example", "x@spam.example", 1))
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), inbound.ReasonSenderNotAllowed) {
		t.Fatalf("a refused sender: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, a, "GET", prefix+"/webhooks/inbox/events", nil, "")
	var list struct {
		Events []webhookEvent `json:"events"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Events) != 2 || list.Events[1].Reason != inbound.ReasonSenderNotAllowed {
		t.Fatalf("inbox history: %s", rec.Body.String())
	}
	rec, _ = get(t, a, "POST", prefix+"/webhooks/inbox/events/"+list.Events[0].ID+"/replay", nil, "")
	if rec.Code != 202 {
		t.Fatalf("replay: %d", rec.Code)
	}
	select {
	case again := <-seen:
		if again.h.Get("X-Whisk-Webhook-Id") != got.h.Get("X-Whisk-Webhook-Id") {
			t.Fatal("a replay keeps the id")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no replay")
	}
	rec, _ = get(t, a, "POST", prefix+"/webhooks/inbox/events/"+list.Events[1].ID+"/replay", nil, "")
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "INBOX_MESSAGE_DROPPED") {
		t.Fatalf("replay of a dropped message: %d %s", rec.Code, rec.Body.String())
	}
	rec, _ = get(t, a, "POST", "/v1/stub/inbox", nil, "\x00")
	if rec.Code != 400 {
		t.Fatalf("not a message: %d", rec.Code)
	}
}

func TestInboxNotDeclared(t *testing.T) {
	s := newTestStub(t, httptest.NewServer(http.NotFoundHandler()))
	rec, _ := get(t, newAPI(s), "POST", "/v1/stub/inbox", nil, labMessage)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "INBOX_NOT_DECLARED") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
