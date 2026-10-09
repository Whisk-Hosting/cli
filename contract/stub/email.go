package stub

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/mailattach"
	"github.com/whisk-run/contract/run/runhttp"
)

// The stub's email: POST …/email/send takes the message the platform would, with the same rules
// for its attachments (mailattach), and keeps it here instead of sending it, so an app sends mail
// locally without reaching anyone. GET …/email/sent lists what it took, newest first.

// sentMail is one message the stub took.
type sentMail struct {
	ID          string     `json:"id"`
	To          []string   `json:"to"`
	From        string     `json:"from,omitempty"`
	ReplyTo     string     `json:"reply_to,omitempty"`
	Subject     string     `json:"subject"`
	HTML        string     `json:"html,omitempty"`
	Text        string     `json:"text,omitempty"`
	Attachments []sentFile `json:"attachments"`
	SentAt      time.Time  `json:"sent_at"`
}

// sentFile is what the stub keeps of an attachment: enough to check, not the bytes.
type sentFile struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id,omitempty"`
	StorageKey  string `json:"storage_key,omitempty"`
	Bytes       int    `json:"bytes"`
	SHA256      string `json:"sha256"`
}

// maxSent is how many messages the stub keeps.
const maxSent = 200

type sendRequest struct {
	To          any                        `json:"to"`
	From        string                     `json:"from"`
	Subject     string                     `json:"subject"`
	HTML        string                     `json:"html"`
	Text        string                     `json:"text"`
	ReplyTo     string                     `json:"reply_to"`
	Attachments []apitypes.EmailAttachment `json:"attachments"`
}

func (a *api) sendEmail(w http.ResponseWriter, r *http.Request) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("The email endpoint needs the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	var req sendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, mailattach.MaxRequestBytes)).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, 413, werrors.New(mailattach.CodeTooLarge, fmt.Sprintf("The message is over %d MB; attachments may hold %d MB together.", mailattach.MaxRequestBytes>>20, mailattach.MaxBytes>>20),
				"Send smaller files, fewer of them, or a link to the file in storage instead of the file.", map[string]any{"limit_bytes": mailattach.MaxBytes}))
			return
		}
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The request body is not JSON.", "Send {to, subject, html, text, from, reply_to, attachments}.", nil))
		return
	}
	to, why := checkMessage(req)
	if why != "" {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", why, "Send to (an address or a list of up to 20), a subject, and html, text or both.", nil))
		return
	}
	files, problem := mailattach.Plan(req.Attachments, req.HTML, a.stub.env("WHISK_STORAGE_PREFIX"))
	if problem == nil {
		files, problem = a.stub.readFiles(r.Context(), files)
	}
	if problem != nil {
		status := map[string]int{mailattach.CodeInvalid: 400, mailattach.CodeTooLarge: 413, mailattach.CodeBlocked: 422, mailattach.CodeNotFound: 404}[problem.Code]
		if status == 0 {
			status = 503
		}
		writeError(w, r, status, werrors.New(problem.Code, problem.Message, problem.Fix, problem.Details))
		return
	}
	m := sentMail{ID: newULID(), To: to, From: req.From, ReplyTo: req.ReplyTo, Subject: req.Subject, HTML: req.HTML, Text: req.Text,
		Attachments: []sentFile{}, SentAt: time.Now().UTC()}
	names := []string{}
	for _, f := range files {
		sum := sha256.Sum256(f.Content)
		m.Attachments = append(m.Attachments, sentFile{Filename: f.Filename, ContentType: f.ContentType, ContentID: f.ContentID,
			StorageKey: f.StorageKey, Bytes: len(f.Content), SHA256: hex.EncodeToString(sum[:])})
		label := f.Filename
		if f.Inline() {
			label += " inline cid:" + f.ContentID
		}
		names = append(names, label)
	}
	a.stub.store.putMail(&m)
	line := fmt.Sprintf("email %s to %s: %q (kept here, not sent)", m.ID, strings.Join(to, ", "), m.Subject)
	if len(names) > 0 {
		line += "; attachments: " + strings.Join(names, ", ")
	}
	a.stub.logf("%s", line)
	writeJSON(w, 202, map[string]any{"id": m.ID, "recipients": len(to), "sent_at": m.SentAt})
}

// checkMessage is the message's own checks, the platform's wording: the recipients as a list,
// or why the message cannot go.
func checkMessage(req sendRequest) ([]string, string) {
	var to []string
	switch v := req.To.(type) {
	case string:
		if v != "" {
			to = []string{v}
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, "A recipient is not an address."
			}
			to = append(to, s)
		}
	}
	switch {
	case len(to) == 0:
		return nil, "The message names no recipient."
	case len(to) > 20:
		return nil, fmt.Sprintf("A message may name 20 recipients; this one names %d.", len(to))
	case req.Subject == "":
		return nil, "The message has no subject."
	case req.HTML == "" && req.Text == "":
		return nil, "The message has no body."
	}
	for _, addr := range append(append([]string{}, to...), req.From, req.ReplyTo) {
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil || strings.ContainsAny(addr, "\r\n") {
			return nil, fmt.Sprintf("%q is not an email address.", addr)
		}
	}
	return to, ""
}

// env is a variable the stub gives the app beyond its own: the storage whisk dev started.
func (s *stub) env(name string) string {
	for _, kv := range s.extraEnv {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// fileTimeout bounds reading one attachment from the local storage.
const fileTimeout = 30 * time.Second

// readFiles reads the files named by storage_key from the storage whisk dev started, the way the
// platform reads them from the org's bucket, then checks the whole set.
func (s *stub) readFiles(ctx context.Context, files []mailattach.File) ([]mailattach.File, *mailattach.Problem) {
	out := make([]mailattach.File, len(files))
	copy(out, files)
	for i := range out {
		f := &out[i]
		if f.StorageKey == "" {
			continue
		}
		room := mailattach.Budget(out)
		content, size, err := s.getObject(ctx, f.StorageKey, room)
		switch {
		case errors.Is(err, errObjectMissing):
			return nil, &mailattach.Problem{Code: mailattach.CodeNotFound,
				Message: fmt.Sprintf("Attachment %s names %s, which the app's storage does not hold.", f.Filename, f.StorageKey),
				Fix:     "Send the key of a file the app wrote, its WHISK_STORAGE_PREFIX followed by its name, or send the file itself as content.",
				Details: map[string]any{"attachment": i, "filename": f.Filename, "storage_key": f.StorageKey}}
		case errors.Is(err, errObjectTooLarge):
			return nil, mailattach.TooLarge(mailattach.Total(out) + size)
		case err != nil:
			return nil, &mailattach.Problem{Code: "PLATFORM_UNAVAILABLE", Message: "The attachment could not be read from local storage: " + err.Error(),
				Fix: "Check that whisk dev's storage container is running, then send again."}
		}
		f.Content = content
	}
	if p := mailattach.Check(out); p != nil {
		return nil, p
	}
	return out, nil
}

var (
	errObjectMissing  = errors.New("no such object")
	errObjectTooLarge = errors.New("the object is larger than the room left")
)

// getObject reads one object from the local S3-compatible storage with the app's own
// credentials, signed with AWS Signature Version 4, path-style.
func (s *stub) getObject(ctx context.Context, key string, room int64) ([]byte, int64, error) {
	endpoint, bucket := strings.TrimRight(s.env("WHISK_STORAGE_ENDPOINT"), "/"), s.env("WHISK_STORAGE_BUCKET")
	if endpoint == "" || bucket == "" {
		return nil, 0, errObjectMissing
	}
	u, err := url.Parse(endpoint + "/" + bucket + "/" + escapeKey(key))
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, fileTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	signV4(req, s.env("WHISK_STORAGE_ACCESS_KEY"), s.env("WHISK_STORAGE_SECRET_KEY"), orDefault(s.env("WHISK_STORAGE_REGION"), "us-east-1"), time.Now().UTC())
	res, err := runhttp.Client(fileTimeout).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, 0, errObjectMissing
	case res.StatusCode/100 != 2:
		return nil, 0, fmt.Errorf("storage answered %s", res.Status)
	case res.ContentLength > room:
		return nil, res.ContentLength, errObjectTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, room+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(body)) > room {
		return nil, int64(len(body)), errObjectTooLarge
	}
	return body, int64(len(body)), nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// escapeKey escapes each segment of an object key for a URL path, keeping the slashes.
func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = s3Escape(p)
	}
	return strings.Join(parts, "/")
}

// s3Escape is URI encoding as SigV4 wants it: everything but unreserved characters.
func s3Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// signV4 signs a GET with no body in the Authorization header.
func signV4(req *http.Request, accessKey, secretKey, region string, now time.Time) {
	const payload = "UNSIGNED-PAYLOAD"
	amzDate, day := now.Format("20060102T150405Z"), now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payload)
	host := req.URL.Host
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), "",
		"host:" + host, "x-amz-content-sha256:" + payload, "x-amz-date:" + amzDate, "",
		"host;x-amz-content-sha256;x-amz-date", payload}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	hm := func(key []byte, msg string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	k := hm(hm(hm(hm([]byte("AWS4"+secretKey), day), region), "s3"), "aws4_request")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature="+hex.EncodeToString(hm(k, toSign)))
}
