// The Whisk conventions in one file: identity from headers, JSON logging with the request id,
// tracing, the Inngest client wired to the platform, the approval helper, the webhook
// deliveries helper and the custom domains helper. Copy this file as-is.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/go-chi/chi/v5"
	"github.com/inngest/inngestgo"
	"github.com/inngest/inngestgo/step"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if fallback == "" {
		panic(name + " is not set")
	}
	return fallback
}

func ptrIfSet(name string) *string {
	if v := os.Getenv(name); v != "" {
		return &v
	}
	return nil
}

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("app", os.Getenv("WHISK_APP_NAME"))

// startTracing starts OpenTelemetry when the platform sets OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
// (whisk dev does not); the exporter and provider read the other OTEL_* variables themselves.
// Calls through http.DefaultClient are traced, and traceRequests and traceQueries trace
// requests and SQL. The function it returns flushes spans; call it on the way out.
func startTracing(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	http.DefaultClient.Transport = otelhttp.NewTransport(http.DefaultTransport)
	return provider.Shutdown, nil
}

// traceRequests is chi middleware: a server span per request that continues the edge's
// traceparent, so a trace's id is the request id, named by its route once routed.
func traceRequests(next http.Handler) http.Handler {
	named := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
		if route := chi.RouteContext(req.Context()).RoutePattern(); route != "" {
			span := trace.SpanFromContext(req.Context())
			span.SetName(req.Method + " " + route)
			span.SetAttributes(attribute.String("http.route", route))
		}
	})
	return otelhttp.NewHandler(named, "request")
}

// traceQueries makes each statement on a connection from cfg a client span inside the
// request's span. Parameters are not recorded.
func traceQueries(cfg *pgx.ConnConfig) { cfg.Tracer = otelpgx.NewTracer() }

// Identity is who the platform says is calling. Only the X-Whisk-* headers are trusted.
type Identity struct {
	Audience  string
	UserID    string
	Email     string
	Name      string
	Org       string
	Groups    []string
	Roles     []string
	RequestID string
}

func list(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func identityFrom(h http.Header) Identity {
	audience := h.Get("X-Whisk-Audience")
	if audience == "" {
		audience = "anonymous"
	}
	return Identity{
		Audience: audience, UserID: h.Get("X-Whisk-User-Id"), Email: h.Get("X-Whisk-Email"), Name: h.Get("X-Whisk-Name"),
		Org: h.Get("X-Whisk-Org"), Groups: list(h.Get("X-Whisk-Groups")), Roles: list(h.Get("X-Whisk-Roles")), RequestID: h.Get("X-Whisk-Request-Id"),
	}
}

func (id Identity) HasRole(roles ...string) bool {
	for _, want := range roles {
		for _, have := range id.Roles {
			if have == want {
				return true
			}
		}
	}
	return false
}

// Who may see and change a record (skill §4, "Who may see and change what"). A record belongs to
// the person who made it: store their X-Whisk-User-Id beside it as its owner. The business's own
// team sees every record; a customer sees only their own; anyone else sees nothing. Query with
// ScopeFor so the database does the filtering, and answer 404, not 403, for a record the person
// may not see, so its id tells them nothing. access_test.go checks these rules over thousands of
// generated people and records; keep it passing when you change them.

// Scope is which records a person may see: Kind "all", "owner" (OwnerID's only) or "none".
type Scope struct {
	Kind    string
	OwnerID string
}

func ScopeFor(id Identity) Scope {
	switch {
	case id.UserID == "":
		return Scope{Kind: "none"}
	case id.Audience == "team":
		return Scope{Kind: "all"}
	case id.Audience == "customer":
		return Scope{Kind: "owner", OwnerID: id.UserID}
	}
	return Scope{Kind: "none"}
}

func (s Scope) Includes(ownerID string) bool {
	return s.Kind == "all" || (s.Kind == "owner" && ownerID != "" && s.OwnerID == ownerID)
}

func (id Identity) CanSee(ownerID string) bool { return ScopeFor(id).Includes(ownerID) }

// CanChange: a customer their own records; on the team, the record's owner or an owner or
// admin of the business.
func (id Identity) CanChange(ownerID string) bool {
	return id.CanSee(ownerID) && (id.Audience != "team" || ownerID == id.UserID || id.HasRole("owner", "admin"))
}

func whiskHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for name, values := range h {
		if strings.HasPrefix(strings.ToLower(name), "x-whisk-") && len(values) > 0 {
			out[strings.ToLower(name)] = values[0]
		}
	}
	return out
}

// newInngest wires the SDK to the platform: runs arrive at queue.endpoint signed with
// WHISK_INNGEST_SIGNING_KEY; events sent from inside a function go to WHISK_INNGEST_URL. Event
// names are plain everywhere ("po.created"): the platform keeps each app's events to itself.
func newInngest() (inngestgo.Client, error) {
	return inngestgo.NewClient(inngestgo.ClientOpts{
		AppID:           env("WHISK_APP_NAME", "whisk-go"),
		EventKey:        ptrIfSet("WHISK_INNGEST_EVENT_KEY"),
		SigningKey:      ptrIfSet("WHISK_INNGEST_SIGNING_KEY"),
		APIBaseURL:      ptrIfSet("WHISK_INNGEST_URL"),
		EventAPIBaseURL: ptrIfSet("WHISK_INNGEST_URL"),
		Dev:             inngestgo.BoolPtr(os.Getenv("WHISK_DEV") == "1"),
		Logger:          logger,
	})
}

// Decision is what an approval resolves to; Decision is approved, rejected or expired.
type Decision struct {
	ApprovalID string            `json:"approval_id"`
	Decision   string            `json:"decision"`
	Actor      map[string]string `json:"actor,omitempty"`
	At         string            `json:"at,omitempty"`
	Note       string            `json:"note,omitempty"`
}

type ApprovalRequest struct {
	To      string
	Title   string
	Data    any
	Timeout time.Duration
}

// approval pauses the run until a human decides. It sends whisk/approval.requested in a step
// named name+"/request" and waits in a step named name for whisk/approval.decided with the
// same approval id. The platform notifies To (an org role, group:<name> or user:<email>).
func approval(ctx context.Context, runID, name string, req ApprovalRequest) (Decision, error) {
	id := runID + ":" + name
	if _, err := step.Send(ctx, name+"/request", inngestgo.Event{
		Name: "whisk/approval.requested",
		Data: map[string]any{"approval_id": id, "to": req.To, "title": req.Title, "data": req.Data, "run_id": runID},
	}); err != nil {
		return Decision{}, err
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 7 * 24 * time.Hour
	}
	evt, err := step.WaitForEvent[inngestgo.GenericEvent[Decision]](ctx, name, step.WaitForEventOpts{
		Event:   "whisk/approval.decided",
		If:      inngestgo.StrPtr(fmt.Sprintf("async.data.approval_id == %q", id)),
		Timeout: timeout,
	})
	if errors.Is(err, step.ErrEventNotReceived) {
		return Decision{ApprovalID: id, Decision: "expired"}, nil
	}
	if err != nil {
		return Decision{}, err
	}
	return evt.Data, nil
}

// enqueue sends an event through the platform queue with the service token.
func enqueue(ctx context.Context, name string, data any, dedupeKey string) (string, error) {
	body, _ := json.Marshal(map[string]any{"name": name, "data": data, "dedupe_key": dedupeKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env("WHISK_QUEUE_URL", ""), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env("WHISK_SERVICE_TOKEN", ""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("enqueue %s: %d %s", name, resp.StatusCode, raw)
	}
	var out struct {
		ID string `json:"id"`
	}
	return out.ID, json.Unmarshal(raw, &out)
}

// MediaLinks is what signMedia answers: one link per address asked for, in the same order,
// and when they all stop working.
type MediaLinks struct {
	ExpiresAt time.Time `json:"expires_at"`
	Links     []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
		URL  string `json:"url"`
	} `json:"links"`
}

// signMedia asks the platform, with the service token, for signed links to private uploads,
// for an app that signs its people in itself (skill §9, "Signed links"). Write each address as
// the page would for someone signed in to Whisk: "/.whisk/img/<id>?w=800" for an image,
// "/.whisk/media/<id>" (or ".../embed", ".../poster.jpg") for video and audio. Each link works
// for anyone holding it, with no sign-in, until it expires (expiresIn, an hour when zero,
// twelve hours at most), so check that the person may see the file first and sign links when
// the page is drawn rather than storing them. Server side only.
func signMedia(ctx context.Context, paths []string, expiresIn time.Duration) (MediaLinks, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"paths": paths, "expires_in": int64(expiresIn / time.Second)})
	endpoint := strings.TrimSuffix(env("WHISK_QUEUE_URL", ""), "/events") + "/uploads/links"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return MediaLinks{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env("WHISK_SERVICE_TOKEN", ""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return MediaLinks{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return MediaLinks{}, fmt.Errorf("signMedia: %d %s", resp.StatusCode, raw)
	}
	var out MediaLinks
	return out, json.Unmarshal(raw, &out)
}

// platformURL is the app's own routes on the platform API, .../v1/orgs/<org>/apps/<app>, read
// from WHISK_QUEUE_URL, which is that address plus /events.
func platformURL() string {
	return strings.TrimSuffix(env("WHISK_QUEUE_URL", ""), "/events")
}

// PlatformError is the platform's error for a call the app made with its service token: a stable
// code, a sentence, a fix and the details (CONTRACT.md §10).
type PlatformError struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fix     string         `json:"fix"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *PlatformError) Error() string { return e.Code + ": " + e.Message }

// platformCall sends one request to the app's own routes with the service token, read per call
// since it rotates, within wait, and decodes the answer into out (nil to ignore it).
func platformCall(ctx context.Context, wait time.Duration, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, platformURL()+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env("WHISK_SERVICE_TOKEN", ""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error PlatformError `json:"error"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Error.Code == "" {
			return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		e.Error.Status = resp.StatusCode
		return &e.Error
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Domain is one of the app's hostnames (CONTRACT.md §8, "Custom domains"). Status is
// pending_dns, pending_certificate or active; Records are what to show whoever runs the name's
// DNS until it is verified; AddedBy is app for the ones the app added, team for the business's.
type Domain struct {
	ID         string      `json:"id"`
	Hostname   string      `json:"hostname"`
	Kind       string      `json:"kind"`
	Verified   bool        `json:"verified"`
	CertStatus string      `json:"cert_status"`
	Status     string      `json:"status"`
	AddedBy    string      `json:"added_by,omitempty"`
	Records    []DNSRecord `json:"records,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// DNSRecord is one record to create at a DNS provider.
type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// domains manages the app's own custom domains with its service token, so the app can let the
// businesses it serves point a domain of theirs at it. Keep each domain's id beside the
// customer it belongs to and remove by that id. A *PlatformError carries the platform's code:
// DOMAIN_UNVERIFIED (details.records and details.missing say what is not in place yet),
// DOMAIN_TAKEN, PLAN_LIMIT_DOMAINS, RATE_LIMITED.
var domains domainsHelper

type domainsHelper struct{}

// List is every hostname of the app: its own address and the custom domains.
func (domainsHelper) List(ctx context.Context) ([]Domain, error) {
	var out struct {
		Items []Domain `json:"items"`
	}
	err := platformCall(ctx, 15*time.Second, http.MethodGet, "/domains", nil, &out)
	return out.Items, err
}

// Add attaches a hostname; the answer carries the records to show.
func (domainsHelper) Add(ctx context.Context, hostname string) (Domain, error) {
	var d Domain
	err := platformCall(ctx, 15*time.Second, http.MethodPost, "/domains", map[string]string{"hostname": hostname}, &d)
	return d, err
}

// Verify checks the records and has the certificate issued; it can take half a minute.
func (domainsHelper) Verify(ctx context.Context, id string) (Domain, error) {
	var d Domain
	err := platformCall(ctx, 60*time.Second, http.MethodPost, "/domains/"+id+"/verify", nil, &d)
	return d, err
}

// Remove detaches a hostname the app added, by its id.
func (domainsHelper) Remove(ctx context.Context, id string) error {
	return platformCall(ctx, 15*time.Second, http.MethodDelete, "/domains/"+id, nil, nil)
}

// Delivery is one webhook delivery, proven to be the platform's: the id to dedupe on, the
// source name from the manifest, when the platform received it, the provider's raw body and
// the provider's own headers (the X-Whisk-Webhook-Orig- prefix removed).
type Delivery struct {
	ID         string
	Source     string
	ReceivedAt string
	Body       []byte
	Headers    http.Header
}

// deliveries wraps a webhook handler (CONTRACT.md §7): it reads the raw body, checks
// X-Whisk-Delivery-Signature under WHISK_DELIVERY_KEY in constant time, refuses an
// app-to-app call (X-Whisk-Service-App) or an unsigned request with 401 DELIVERY_UNVERIFIED,
// records the delivery with verified: true and dedupes on X-Whisk-Webhook-Id, and only then
// runs the app's function. A repeat (retry, replay) answers the recorded row without running it.
var deliveries deliveriesHelper

type deliveriesHelper struct{}

// deliverySignature is "v1=" + hex(HMAC-SHA256(key, id "\n" receivedAt "\n" body)).
func deliverySignature(key []byte, id, receivedAt string, body []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "\n" + receivedAt + "\n"))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

// Verify answers "" when the request is a platform delivery, or why it is not: no_key,
// service_app, missing, mismatch.
func (deliveriesHelper) Verify(h http.Header, body []byte) string {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("WHISK_DELIVERY_KEY"))
	id, receivedAt, sig := h.Get("X-Whisk-Webhook-Id"), h.Get("X-Whisk-Webhook-Received-At"), h.Get("X-Whisk-Delivery-Signature")
	switch {
	case err != nil || len(key) == 0:
		return "no_key"
	case h.Get("X-Whisk-Service-App") != "":
		return "service_app"
	case id == "" || receivedAt == "" || !strings.HasPrefix(sig, "v1="):
		return "missing"
	case !hmac.Equal([]byte(sig), []byte(deliverySignature(key, id, receivedAt, body))):
		return "mismatch"
	}
	return ""
}

// recordFunc is db.RecordEvent: the dedupe store the helper writes the delivery into.
type recordFunc func(ctx context.Context, id, kind string, payload any, verified bool) (Recorded, error)

// Handle is the http.HandlerFunc for a webhook handler route. fn runs once per delivery id
// with the typed event; its error answers 500 so the platform retries.
func (d deliveriesHelper) Handle(record recordFunc, fn func(req *http.Request, delivery Delivery) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 5<<20))
		if err != nil {
			writeJSON(w, 413, map[string]string{"error": "body too large"})
			return
		}
		if why := d.Verify(req.Header, body); why != "" {
			writeJSON(w, 401, map[string]any{"error": map[string]string{
				"code":    "DELIVERY_UNVERIFIED",
				"message": "This request is not a signed platform delivery (" + why + ").",
				"fix":     "Only the platform delivers to a webhook handler, signing each delivery under WHISK_DELIVERY_KEY. Send the webhook through the source URL, not to the handler directly.",
			}})
			return
		}
		delivery := Delivery{ID: req.Header.Get("X-Whisk-Webhook-Id"), Source: req.Header.Get("X-Whisk-Webhook-Source"), ReceivedAt: req.Header.Get("X-Whisk-Webhook-Received-At"), Body: body, Headers: http.Header{}}
		for name, values := range req.Header {
			if orig, ok := strings.CutPrefix(name, "X-Whisk-Webhook-Orig-"); ok {
				delivery.Headers[orig] = values
			}
		}
		var payload any
		if json.Unmarshal(body, &payload) != nil {
			payload = map[string]string{"raw": "not json"}
		}
		rec, err := record(req.Context(), delivery.ID, "webhook", map[string]any{"source": delivery.Source, "payload": payload}, true)
		if err != nil {
			respond(w, nil, err)
			return
		}
		if !rec.Duplicate {
			if err := fn(req, delivery); err != nil {
				respond(w, nil, err)
				return
			}
		}
		writeJSON(w, 200, rec)
	}
}
