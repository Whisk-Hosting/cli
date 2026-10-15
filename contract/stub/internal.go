package stub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/whisk-run/contract/webhook"
)

// internal is the stand-in for the platform's internal listener: everything arriving here is
// a platform delivery (function runs from the Inngest dev server, webhook deliveries from the
// stub itself) or an app-to-app call carrying the service token. Identity is service.
type internal struct {
	stub  *stub
	proxy *httputil.ReverseProxy
}

func newInternal(s *stub) *internal {
	return &internal{stub: s, proxy: httputil.NewSingleHostReverseProxy(s.upstream)}
}

func (i *internal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s := i.stub
	stripIdentityHeaders(r.Header, r.Header.Get(stubDeliveryMarker) == s.deliveryMarker && s.deliveryMarker != "")
	i.jobHeaders(r)
	reqID := newULID()
	r.Header.Set("X-Whisk-Request-Id", reqID)
	w.Header().Set("X-Whisk-Request-Id", reqID)
	r.Header.Set("X-Whisk-Audience", "service")
	r.Header.Set("X-Whisk-Org", s.orgID)
	caller := "platform"
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if strings.TrimPrefix(auth, "Bearer ") != s.serviceToken {
			writeError(w, r, http.StatusUnauthorized, authRequired("The service token is not valid.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN, read from the environment at request time."))
			return
		}
		r.Header.Set("X-Whisk-Service-App", s.appID)
		r.Header.Del("Authorization")
		caller = "app " + s.manifest.Name
	}
	rec := &statusRecorder{ResponseWriter: w, status: 200}
	i.proxy.ServeHTTP(rec, r)
	s.logf("internal %s %s -> %d (service, %s) %s", r.Method, r.URL.Path, rec.status, caller, time.Since(start).Round(time.Millisecond))
}

// stubDeliveryMarker is the private header the stub's own webhook posts carry; it never
// reaches the app.
const stubDeliveryMarker = "Whisk-Stub-Delivery"

// stripIdentityHeaders removes every inbound X-Whisk-* header. The platform's own webhook
// delivery headers and signature survive only on the stub's own posts (ownDelivery); an
// app-to-app call can never carry them, so a handler that sees them knows the platform sent
// the request. Job headers are set afresh by jobHeaders from the run body.
func stripIdentityHeaders(h http.Header, ownDelivery bool) {
	h.Del(stubDeliveryMarker)
	for name := range h {
		lower := strings.ToLower(name)
		delivery := strings.HasPrefix(lower, "x-whisk-webhook-") || lower == strings.ToLower(webhook.DeliverySignatureHeader)
		if strings.HasPrefix(lower, "x-whisk-") && !(ownDelivery && delivery) {
			h.Del(name)
		}
	}
}

// jobHeaders sets X-Whisk-Job-Id and X-Whisk-Job-Attempt on function-run requests from the
// run context the Inngest protocol carries in the body.
func (i *internal) jobHeaders(r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != i.stub.manifest.Queue.Endpoint {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	var payload struct {
		Ctx struct {
			RunID   string `json:"run_id"`
			Attempt int    `json:"attempt"`
		} `json:"ctx"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Ctx.RunID != "" {
		r.Header.Set("X-Whisk-Job-Id", payload.Ctx.RunID)
		r.Header.Set("X-Whisk-Job-Attempt", strconv.Itoa(payload.Ctx.Attempt))
	}
}
