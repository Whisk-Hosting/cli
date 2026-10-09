package stub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/run/runhttp"
)

// api is the stand-in for api.whisk.run and hooks.whisk.run in one listener:
//
//	POST /v1/orgs/{org}/apps/{app}/events                      enqueue (service token)
//	GET  /v1/orgs/{org}/apps/{app}/approvals[?status=pending]   list approvals
//	POST /approvals/{id}                                        decide {decision, note}
//	GET  /v1/orgs/{org}/apps/{app}/webhooks/{name}/events       stored deliveries
//	POST /v1/orgs/{org}/apps/{app}/webhooks/{name}/events/{id}/replay
//	POST /hooks/{org}/{app}/{source}[/{token}]                  webhook ingress
//	*    everything else                                        the workflow API (Inngest dev server)
//
// The workflow API is served at the listener's root because the SDKs resolve absolute paths
// such as /fn/register and /e/<key> against WHISK_INNGEST_URL; the stub's own routes never
// collide with Inngest's.
type api struct {
	stub    *stub
	inngest *httputil.ReverseProxy
}

func newAPI(s *stub) *api {
	a := &api{stub: s}
	if s.inngestURL != nil {
		a.inngest = httputil.NewSingleHostReverseProxy(s.inngestURL)
		director := a.inngest.Director
		// nosemgrep: go.lang.security.reverseproxy-director.reverseproxy-director -- the local stub, never deployed
		a.inngest.Director = func(r *http.Request) {
			director(r)
			r.Host = s.inngestURL.Host
		}
	}
	return a
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := a.stub
	p := r.URL.Path
	prefix := "/v1/orgs/" + s.orgID + "/apps/" + s.appID
	switch {
	case p == prefix+"/events" && r.Method == http.MethodPost:
		a.enqueue(w, r)
	case p == prefix+"/approvals" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]any{"approvals": s.store.approvalList(r.URL.Query().Get("status"))})
	case strings.HasPrefix(p, "/approvals/") && r.Method == http.MethodPost:
		a.decide(w, r, strings.TrimPrefix(p, "/approvals/"))
	case strings.HasPrefix(p, prefix+"/webhooks/"):
		a.webhooks(w, r, strings.TrimPrefix(p, prefix+"/webhooks/"))
	case strings.HasPrefix(p, "/hooks/"):
		s.hooks.receive(w, r, strings.TrimPrefix(p, "/hooks/"))
	case p == "/v1/stub":
		writeJSON(w, 200, s.describe())
	default:
		a.proxyInngest(w, r)
	}
}

func (a *api) bearerOK(r *http.Request) bool {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") == a.stub.serviceToken && r.Header.Get("Authorization") != ""
}

type enqueueRequest struct {
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	DedupeKey string          `json:"dedupe_key,omitempty"`
}

func (a *api) enqueue(w http.ResponseWriter, r *http.Request) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("The queue endpoint needs the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	var req enqueueRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.Name == "" {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The event needs a name.", `Send {"name": "<event name>", "data": {...}} with an optional dedupe_key.`, map[string]any{"problems": []map[string]string{{"path": "/name", "message": "required"}}}))
		return
	}
	if req.Data == nil {
		req.Data = json.RawMessage("{}")
	}
	id, err := a.stub.sendEvent(req.Name, req.Data, req.DedupeKey)
	if err != nil {
		writeError(w, r, 503, werrors.New("PLATFORM_UNAVAILABLE", "The workflow engine did not accept the event: "+err.Error(), "Check that the Inngest dev server is running (the stub starts it unless --no-inngest was given).", nil))
		return
	}
	a.stub.logf("event %s enqueued as %s", req.Name, id)
	writeJSON(w, 200, map[string]string{"id": id})
}

// eventTimeout bounds one event sent to the dev server on this machine.
const eventTimeout = 30 * time.Second

// sendEvent posts one event to the Inngest dev server. dedupe becomes the event id, which
// Inngest uses for idempotency.
func (s *stub) sendEvent(name string, data json.RawMessage, dedupe string) (string, error) {
	if s.inngestURL == nil {
		return "", fmt.Errorf("no workflow engine configured")
	}
	evt := map[string]any{"name": name, "data": data, "ts": time.Now().UnixMilli()}
	if dedupe != "" {
		evt["id"] = dedupe
	}
	body, _ := json.Marshal(evt)
	resp, err := runhttp.Client(eventTimeout).Post(s.inngestURL.String()+"/e/"+s.eventKey, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		IDs []string `json:"ids"`
	}
	_ = json.Unmarshal(raw, &out)
	if len(out.IDs) > 0 {
		return out.IDs[0], nil
	}
	return dedupe, nil
}

// proxyInngest forwards SDK traffic to the dev server and watches outgoing events for
// approval requests, which is how the platform learns a run is waiting for a human.
func (a *api) proxyInngest(w http.ResponseWriter, r *http.Request) {
	if a.inngest == nil {
		writeError(w, r, 404, werrors.New("NOT_FOUND", fmt.Sprintf("The stub does not serve %s %s and no workflow engine is configured.", r.Method, r.URL.Path), "See GET /v1/stub for the endpoints this stub serves. Start without --no-inngest to serve the workflow API here.", nil))
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/e/") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err == nil {
			a.stub.observeEvents(body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
	}
	a.inngest.ServeHTTP(w, r)
}

func (s *stub) observeEvents(body []byte) {
	var one map[string]any
	var many []map[string]any
	if err := json.Unmarshal(body, &many); err != nil {
		if json.Unmarshal(body, &one) != nil {
			return
		}
		many = []map[string]any{one}
	}
	for _, evt := range many {
		if evt["name"] != approvalRequested {
			continue
		}
		data, _ := evt["data"].(map[string]any)
		id, _ := data["approval_id"].(string)
		if id == "" {
			continue
		}
		to, _ := data["to"].(string)
		title, _ := data["title"].(string)
		fn, _ := data["function"].(string)
		run, _ := data["run_id"].(string)
		s.store.putApproval(&approval{ID: id, To: to, Title: title, Data: data["data"], Function: fn, RunID: run, RequestedAt: time.Now().UTC(), Status: "pending"})
		s.logf("approval requested: %s (%s) for %s; decide with: curl -X POST %s/approvals/%s -d '{\"decision\":\"approved\"}'", id, title, to, s.apiURL, id)
	}
}

const (
	approvalRequested = "whisk/approval.requested"
	approvalDecided   = "whisk/approval.decided"
)

func (a *api) decide(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "decision must be approved or rejected.", `Send {"decision": "approved" | "rejected", "note": "optional"}.`, nil))
		return
	}
	if _, ok := a.stub.store.approval(id); !ok {
		writeError(w, r, 404, werrors.New("NOT_FOUND", "No approval "+id+" is waiting.", "List approvals with GET "+a.stub.apiURL+"/v1/orgs/"+a.stub.orgID+"/apps/"+a.stub.appID+"/approvals.", nil))
		return
	}
	actor := map[string]any{"user_id": "", "email": "", "name": ""}
	if a.stub.identity != nil {
		actor = map[string]any{"user_id": a.stub.identity.UserID, "email": a.stub.identity.Email, "name": a.stub.identity.Name}
	}
	decision := map[string]any{"approval_id": id, "decision": req.Decision, "actor": actor, "at": time.Now().UTC().Format(time.RFC3339), "note": req.Note}
	ap, ok := a.stub.store.decide(id, decision)
	if !ok {
		writeError(w, r, 409, werrors.New("INVALID_REQUEST", "Approval "+id+" was already decided: "+ap.Status+".", "Each approval is decided once.", nil))
		return
	}
	data, _ := json.Marshal(decision)
	if _, err := a.stub.sendEvent(approvalDecided, data, ""); err != nil {
		writeError(w, r, 503, werrors.New("PLATFORM_UNAVAILABLE", "The decision could not reach the workflow engine: "+err.Error(), "Check the Inngest dev server and retry.", nil))
		return
	}
	a.stub.logf("approval %s %s by %s", id, req.Decision, actor["email"])
	writeJSON(w, 200, ap)
}

func (a *api) webhooks(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	name := parts[0]
	if _, ok := a.stub.manifest.WebhookByName(name); !ok {
		writeError(w, r, 404, werrors.New("WEBHOOK_SOURCE_UNKNOWN", "No webhook source named "+name+" is declared in whisk.yaml.", "Declare it under webhooks and restart the stub.", map[string]any{"source": name}))
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]any{"events": a.stub.store.eventsFor(name)})
	case len(parts) == 4 && parts[1] == "events" && parts[3] == "replay" && r.Method == http.MethodPost:
		e, ok := a.stub.store.event(parts[2])
		if !ok || e.Source != name {
			writeError(w, r, 404, werrors.New("NOT_FOUND", "No event "+parts[2]+" for source "+name+".", "List events with GET .../webhooks/"+name+"/events.", nil))
			return
		}
		if !e.Verified {
			writeError(w, r, 409, werrors.New("WEBHOOK_UNVERIFIED", "Event "+e.ID+" did not verify and cannot be replayed.", "Fix the secret and ask the provider to resend.", map[string]any{"source": name, "event_id": e.ID, "reason": e.Reason}))
			return
		}
		a.stub.hooks.deliverLater(r.Context(), e.ID)
		writeJSON(w, 202, map[string]string{"id": e.ID, "status": "queued"})
	default:
		writeError(w, r, 404, werrors.New("NOT_FOUND", "Unknown webhooks endpoint.", "Use GET .../webhooks/<name>/events or POST .../webhooks/<name>/events/<id>/replay.", nil))
	}
}

func authRequired(message, fix string) werrors.Body {
	return werrors.New("AUTH_REQUIRED", message, fix, nil)
}
