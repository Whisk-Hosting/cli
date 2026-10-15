package stub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/inbound"
)

// The stand-in for the platform's receiving (CONTRACT.md §7 "Inbound email", §13): a raw message
// posted to POST /v1/stub/inbox is read the way the platform reads one, its original and
// attachments are kept in memory as the app's uploads, and the delivery goes to the inbox's
// handler as a stored event of the inbox source, with the same retries and replay as a webhook.
// The app reads a file back with GET /v1/orgs/<org>/apps/<app>/uploads/<id> and the service
// token, as on the platform.

// stubDomain is the receiving domain a local run gives the app's address.
const stubDomain = "in.localhost"

// stubFile is one object the stub keeps for the app.
type stubFile struct {
	ID           string    `json:"id"`
	Key          string    `json:"key"`
	Filename     string    `json:"filename"`
	Bytes        int64     `json:"bytes"`
	ContentType  string    `json:"content_type"`
	Status       string    `json:"status"`
	AuthorisedBy string    `json:"authorised_by"`
	AuthorisedAt time.Time `json:"authorised_at"`
	Visibility   string    `json:"visibility"`
	URL          string    `json:"url,omitempty"`
	content      []byte
}

// inboxAddress is the app's address in a local run.
func (s *stub) inboxAddress() string { return inbound.Address(s.manifest.Name, "dev", stubDomain) }

// receiveMail is POST /v1/stub/inbox: one raw message, as a mail server would hand it over.
func (a *api) receiveMail(w http.ResponseWriter, r *http.Request) {
	s := a.stub
	if s.manifest.Inbox == nil {
		writeError(w, r, 404, werrors.New("INBOX_NOT_DECLARED", "This app declares no inbox, so it receives no email.",
			"Add inbox: with handler: /inbound/email (or the route of your choice) to whisk.yaml and restart whisk dev.", nil))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, inbound.MaxMessageBytes+1))
	if err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The message could not be read: "+err.Error(), "POST the raw message (an .eml file) as the body.", nil))
		return
	}
	msg, err := inbound.Parse(raw)
	if err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The body is not an email message: it has no header.",
			"POST a raw message, such as an .eml file saved from a mail client, with From, To and Subject headers.", nil))
		return
	}
	target := inbound.Target{Address: s.inboxAddress()}
	for _, t := range inbound.Route(append(append([]string{}, msg.To...), msg.Cc...), stubDomain) {
		if t.App == s.manifest.Name {
			target = t
			break
		}
	}
	now := time.Now().UTC()
	evt := &webhookEvent{ID: newULID(), Source: inbound.SourceName, ReceivedAt: now,
		Headers: map[string]string{"Content-Type": "application/json"}, Verified: true, DeliveryStatus: "queued"}
	reason := inbound.Admit(inbound.Admission{Bytes: int64(len(raw)), From: msg.From, AllowFrom: s.manifest.Inbox.AllowFrom})
	var delivery inbound.Delivery
	if reason == "" {
		prefix := "app/" + s.appID + "/inbox/" + evt.ID + "/"
		rawFile := s.keep(prefix+"message.eml", "message.eml", "message/rfc822", raw)
		files := []inbound.File{}
		for i, att := range msg.Attachments {
			if i >= inbound.MaxAttachments {
				break
			}
			files = append(files, s.keep(fmt.Sprintf("%s%d-%s", prefix, i+1, inbound.SafeName(att.Filename)), att.Filename, att.ContentType, att.Content))
		}
		delivery = inbound.Build(msg, target, rawFile, files, nil, now)
	} else {
		delivery = inbound.Build(msg, target, inbound.File{}, nil, nil, now)
		evt.Reason, evt.DeliveryStatus = reason, "skipped"
	}
	evt.Body, _ = json.Marshal(delivery)
	evt.BodyText = string(evt.Body)
	s.store.putEvent(evt)
	if reason != "" {
		s.logf("inbox: dropped %s from %s (%s)", evt.ID, msg.From, reason)
		writeJSON(w, 202, map[string]any{"id": evt.ID, "delivered": false, "reason": reason})
		return
	}
	s.logf("inbox: %s from %s, %d attachments; delivering to %s", evt.ID, msg.From, len(delivery.Attachments), s.manifest.Inbox.Handler)
	s.hooks.deliverLater(r.Context(), evt.ID)
	writeJSON(w, 202, map[string]any{"id": evt.ID, "delivered": true, "recipient": target.Address, "raw": delivery.Raw, "attachments": delivery.Attachments})
}

// keep stores one object for the app and answers how a delivery names it.
func (s *stub) keep(key, filename, contentType string, content []byte) inbound.File {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	f := &stubFile{ID: newULID(), Key: key, Filename: filename, Bytes: int64(len(content)), ContentType: contentType,
		Status: "clean", AuthorisedBy: "platform", AuthorisedAt: time.Now().UTC(), Visibility: "private", content: content}
	s.store.putFile(f)
	return inbound.File{UploadID: f.ID, Key: f.Key, Bytes: f.Bytes, ContentType: f.ContentType}
}

// upload is GET /v1/orgs/<org>/apps/<app>/uploads/<id>: the record and a link to read it, for
// the app's own service token, as the platform answers it.
func (a *api) upload(w http.ResponseWriter, r *http.Request, id string) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("Reading an upload needs the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	f, ok := a.stub.store.file(id)
	if !ok {
		writeError(w, r, 404, werrors.New("NOT_FOUND", "No upload "+id+" in this run.", "Use an upload_id from a delivery; the stub keeps files in memory until it stops.", nil))
		return
	}
	out := *f
	out.URL = a.stub.apiURL + "/v1/stub/files/" + f.ID
	writeJSON(w, 200, out)
}

// fileContent is GET /v1/stub/files/<id>: the bytes, as the store's link would answer them.
func (a *api) fileContent(w http.ResponseWriter, r *http.Request, id string) {
	f, ok := a.stub.store.file(strings.Trim(id, "/"))
	if !ok {
		writeError(w, r, 404, werrors.New("NOT_FOUND", "No file "+id+" in this run.", "Read the link from GET .../uploads/<id> again.", nil))
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.WriteHeader(200)
	_, _ = w.Write(f.content)
}
