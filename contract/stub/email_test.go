package stub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	werrors "github.com/whisk-run/contract/errors"
)

// fakeStorage is the S3-compatible store whisk dev starts: it answers one object to a signed GET.
func fakeStorage(t *testing.T, key string, body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=whisk/") || r.Header.Get("X-Amz-Date") == "" {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/whisk/"+key {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
}

func TestStubSendsEmailWithAttachments(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	store := fakeStorage(t, "notes/reports/october 2026.pdf", []byte("%PDF-1.7 stored report"))
	defer store.Close()
	s := newTestStub(t, app)
	s.extraEnv = []string{"WHISK_STORAGE_ENDPOINT=" + store.URL, "WHISK_STORAGE_BUCKET=whisk", "WHISK_STORAGE_ACCESS_KEY=whisk",
		"WHISK_STORAGE_SECRET_KEY=whiskwhisk", "WHISK_STORAGE_PREFIX=notes/", "WHISK_STORAGE_REGION=local"}
	a := newAPI(s)
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	auth := map[string]string{"Authorization": "Bearer " + s.serviceToken}
	b64 := func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) }
	send := func(hdr map[string]string, body map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		rec, _ := get(t, a, "POST", prefix+"/email/send", hdr, string(raw))
		return rec
	}
	message := func(attachments ...map[string]any) map[string]any {
		return map[string]any{"to": "ana@acme.example", "subject": "Your report", "html": `<img src="cid:logo"><p>Attached.</p>`,
			"text": "Attached.", "attachments": attachments}
	}
	pdf := map[string]any{"filename": "report.pdf", "content": b64("%PDF-1.7 report")}
	logo := map[string]any{"filename": "logo.png", "content": b64("\x89PNG logo"), "content_id": "logo"}
	stored := map[string]any{"filename": "october.pdf", "storage_key": "notes/reports/october 2026.pdf"}

	if rec := send(nil, message(pdf, logo)); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	rec := send(auth, message(pdf, logo, stored))
	if rec.Code != 202 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	rec, _ = get(t, a, "GET", prefix+"/email/sent", nil, "")
	var list struct {
		Emails []sentMail `json:"emails"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Emails) != 1 {
		t.Fatalf("sent: %v %s", err, rec.Body)
	}
	got := list.Emails[0].Attachments
	if len(got) != 3 || got[0].ContentType != "application/pdf" || got[1].ContentID != "logo" ||
		got[2].Bytes != len("%PDF-1.7 stored report") || got[2].StorageKey == "" || got[0].SHA256 == "" {
		t.Fatalf("attachments: %+v", got)
	}

	for _, c := range []struct {
		name string
		body map[string]any
		code string
		want int
	}{
		{"a program", message(logo, map[string]any{"filename": "setup.exe", "content": b64("x")}), "EMAIL_ATTACHMENT_BLOCKED", 422},
		{"an image the html never shows", map[string]any{"to": "ana@acme.example", "subject": "s", "html": "<p>hi</p>", "attachments": []any{logo}}, "EMAIL_ATTACHMENT_INVALID", 400},
		{"a file the storage does not hold", message(logo, map[string]any{"filename": "a.pdf", "storage_key": "notes/missing.pdf"}), "EMAIL_ATTACHMENT_NOT_FOUND", 404},
		{"another app's file", message(logo, map[string]any{"filename": "a.pdf", "storage_key": "other/a.pdf"}), "EMAIL_ATTACHMENT_NOT_FOUND", 404},
		{"too large", message(logo, map[string]any{"filename": "big.pdf", "content": base64.StdEncoding.EncodeToString(make([]byte, 11<<20))}), "EMAIL_ATTACHMENT_TOO_LARGE", 413},
		{"no recipient", map[string]any{"subject": "s", "text": "t"}, "INVALID_REQUEST", 400},
	} {
		rec := send(auth, c.body)
		var b werrors.Body
		_ = json.Unmarshal(rec.Body.Bytes(), &b)
		if rec.Code != c.want || b.Error.Code != c.code || b.Error.Fix == "" {
			t.Errorf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
	}
}
