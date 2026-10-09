package stub

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/run"
	"github.com/whisk-run/contract/run/runhttp"
	"github.com/whisk-run/contract/webhook"
)

// hooks is the stand-in for hooks.whisk.run: verify with the source's preset, store the
// event whatever the verdict, deliver verified events to the handler through the internal
// path with retries, and offer replay.
type hooks struct {
	stub *stub
	// retry delays stand in for the platform's 1m, 5m, 30m, 2h, 12h.
	retryAfter []time.Duration
}

func newHooks(s *stub) *hooks {
	return &hooks{stub: s, retryAfter: []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}}
}

func (h *hooks) receive(w http.ResponseWriter, r *http.Request, rest string) {
	s := h.stub
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[0] != s.orgID || parts[1] != s.appID {
		writeError(w, r, 404, werrors.New("WEBHOOK_SOURCE_UNKNOWN", "The URL does not name this stub's org and app.", "Use the URL printed when the stub started.", nil))
		return
	}
	src, ok := s.manifest.WebhookByName(parts[2])
	if !ok {
		writeError(w, r, 404, werrors.New("WEBHOOK_SOURCE_UNKNOWN", "No webhook source named "+parts[2]+" exists on app "+s.manifest.Name+".", "Declare the source under webhooks in whisk.yaml and restart the stub.", map[string]any{"source": parts[2]}))
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, 405, werrors.New("INVALID_REQUEST", "Webhook deliveries are POST.", "POST the provider's payload to this URL.", nil))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		writeError(w, r, 413, werrors.New("BODY_TOO_LARGE", "The delivery exceeds 5 MB.", "Webhook bodies are limited to 5 MB.", nil))
		return
	}
	verdict := h.verify(src, parts[3:], r, body)
	evt := &webhookEvent{ID: newULID(), Source: src.Name, ReceivedAt: time.Now().UTC(), Headers: flattenHeaders(r.Header), Body: body, BodyText: string(body), Verified: verdict.Verified, Reason: verdict.Reason, DeliveryStatus: "never_delivered"}
	if verdict.Verified {
		evt.DeliveryStatus = "queued"
	}
	s.store.putEvent(evt)
	if !verdict.Verified {
		s.logf("webhook %s: stored %s unverified (%s)", src.Name, evt.ID, verdict.Reason)
		writeError(w, r, 401, werrors.New("WEBHOOK_UNVERIFIED", "The signature on this delivery to source "+src.Name+" did not verify ("+verdict.Reason+").", "Confirm the secret in the secrets file matches the provider's signing secret and that the preset matches the provider.", map[string]any{"source": src.Name, "reason": verdict.Reason, "event_id": evt.ID}))
		return
	}
	s.logf("webhook %s: stored %s verified; delivering to %s", src.Name, evt.ID, src.Handler)
	h.deliverLater(r.Context(), evt.ID)
	writeJSON(w, 200, map[string]any{"id": evt.ID, "verified": true})
}

func (h *hooks) verify(src manifest.Webhook, tokenParts []string, r *http.Request, body []byte) webhook.Verdict {
	s := h.stub
	switch src.Preset {
	case webhook.PresetToken:
		if len(tokenParts) == 1 && tokenParts[0] == s.urlTokens[src.Name] {
			return webhook.Verdict{Verified: true}
		}
		return webhook.Verdict{Reason: "missing_signature"}
	case webhook.PresetHMAC:
		return webhook.Verify(*src.HMAC, s.secrets[src.Secret], webhook.Request{Method: r.Method, URL: s.apiURL + r.URL.RequestURI(), Header: r.Header, Body: body}, time.Now())
	default:
		p := webhook.Presets()[src.Preset]
		return webhook.Verify(p, s.secrets[src.Secret], webhook.Request{Method: r.Method, URL: s.apiURL + r.URL.RequestURI(), Header: r.Header, Body: body}, time.Now())
	}
}

// deliveryTimeout bounds one event's deliveries: every attempt at the post timeout plus the
// waits between them, with room to spare.
const deliveryTimeout = 5 * time.Minute

// postTimeout bounds one post to the handler.
const postTimeout = 30 * time.Second

// deliverLater delivers a stored event after the request that stored it has been answered.
func (h *hooks) deliverLater(ctx context.Context, id string) {
	run.Detach(ctx, "webhook delivery", deliveryTimeout, func(ctx context.Context) error {
		h.deliver(ctx, id)
		return nil
	})
}

// deliver posts a stored event to the handler through the internal listener, retrying on
// anything but 2xx, then marks it dead.
func (h *hooks) deliver(ctx context.Context, id string) {
	s := h.stub
	evt, ok := s.store.event(id)
	if !ok {
		return
	}
	src, _ := s.manifest.WebhookByName(evt.Source)
	for attempt := 0; ; attempt++ {
		status := h.post(ctx, src, evt)
		s.store.update(id, func(e *webhookEvent) {
			e.Attempts++
			e.LastStatus = status
			if status/100 == 2 {
				e.DeliveryStatus = "delivered"
			} else if attempt >= len(h.retryAfter) {
				e.DeliveryStatus = "dead"
			} else {
				e.DeliveryStatus = "failed"
			}
		})
		if status/100 == 2 {
			s.logf("webhook %s: %s delivered (%d)", evt.Source, evt.ID, status)
			return
		}
		if attempt >= len(h.retryAfter) {
			s.logf("webhook %s: %s dead after %d attempts (last %d); replay with POST %s/v1/orgs/%s/apps/%s/webhooks/%s/events/%s/replay", evt.Source, evt.ID, attempt+1, status, s.apiURL, s.orgID, s.appID, evt.Source, evt.ID)
			return
		}
		s.logf("webhook %s: %s attempt %d failed (%d); retrying in %s", evt.Source, evt.ID, attempt+1, status, h.retryAfter[attempt])
		select {
		case <-ctx.Done():
			return
		case <-time.After(h.retryAfter[attempt]):
		}
	}
}

func (h *hooks) post(ctx context.Context, src manifest.Webhook, evt *webhookEvent) int {
	s := h.stub
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.internalURL+src.Handler, bytes.NewReader(evt.Body))
	if err != nil {
		return 0
	}
	for name, value := range evt.Headers {
		lower := strings.ToLower(name)
		if lower == "content-type" || lower == "content-length" {
			continue
		}
		req.Header.Set("X-Whisk-Webhook-Orig-"+name, value)
	}
	if ct, ok := evt.Headers["Content-Type"]; ok {
		req.Header.Set("Content-Type", ct)
	}
	receivedAt := evt.ReceivedAt.Format(time.RFC3339)
	req.Header.Set("X-Whisk-Webhook-Id", evt.ID)
	req.Header.Set("X-Whisk-Webhook-Source", evt.Source)
	req.Header.Set("X-Whisk-Webhook-Received-At", receivedAt)
	// The platform signs what it delivers (CONTRACT.md §7); the marker lets the internal
	// listener keep these headers on the stub's own posts and strip them from every other call.
	req.Header.Set(webhook.DeliverySignatureHeader, webhook.DeliverySignature(s.deliveryKey, evt.ID, receivedAt, evt.Body))
	req.Header.Set(stubDeliveryMarker, s.deliveryMarker)
	resp, err := runhttp.Client(postTimeout).Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func flattenHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for name, values := range h {
		if len(values) > 0 {
			out[http.CanonicalHeaderKey(name)] = strings.Join(values, ", ")
		}
	}
	return out
}
