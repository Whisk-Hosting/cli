package stub

import (
	"encoding/json"
	"testing"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
)

// GET …/email/sends/{id} answers a message the stub took as sent, since nothing local delivers
// mail, with the platform's shape; it needs the service token, and any other id is
// EMAIL_SEND_NOT_FOUND (CONTRACT.md §13).
func TestStubEmailSendStatus(t *testing.T) {
	app := echoApp(t, nil)
	defer app.Close()
	s := newTestStub(t, app)
	a := newAPI(s)
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	auth := map[string]string{"Authorization": "Bearer " + s.serviceToken}

	rec, _ := get(t, a, "POST", prefix+"/email/send", auth, `{"to":"ana@acme.example","subject":"Your receipt","text":"Thanks."}`)
	if rec.Code != 202 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	var sent struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sent); err != nil || sent.ID == "" {
		t.Fatalf("send answered %s", rec.Body)
	}

	if rec, _ := get(t, a, "GET", prefix+"/email/sends/"+sent.ID, nil, ""); rec.Code != 401 {
		t.Errorf("no token: %d", rec.Code)
	}
	rec, _ = get(t, a, "GET", prefix+"/email/sends/"+sent.ID, auth, "")
	var got apitypes.EmailSend
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != 200 || err != nil {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	if got.ID != sent.ID || got.Status != apitypes.EmailSendSent || got.Detail == "" || got.UpdatedAt.IsZero() ||
		len(got.Events) != 1 || got.Events[0].Status != apitypes.EmailSendSent || !got.Events[0].At.Equal(got.UpdatedAt) {
		t.Errorf("status: %+v", got)
	}

	for _, id := range []string{"01UNKNOWN", sent.ID + "/x"} {
		rec, _ := get(t, a, "GET", prefix+"/email/sends/"+id, auth, "")
		var body werrors.Body
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 404 || body.Error.Code != "EMAIL_SEND_NOT_FOUND" || body.Error.Fix == "" {
			t.Errorf("%s: %d %s", id, rec.Code, rec.Body)
		}
	}
}
