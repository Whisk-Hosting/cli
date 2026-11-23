package stub

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/webpush"
)

// The stub's push notifications: the edge serves Whisk's push scripts and a public key made for
// this run, so a page turns notifications on as it does on Whisk; the API keeps devices and
// takes sends with the platform's checks (contract/webpush), and keeps each send instead of
// delivering it, so an app sends locally without a phone buzzing. GET …/push/sent lists them.

// pushKeep is how many sends the stub keeps.
const pushKeep = 200

// keptPushDetail says why a kept send's devices show kept.
const keptPushDetail = "whisk dev kept this notification rather than sending it; on Whisk each device's status moves on as its push service answers"

type pushStore struct {
	key     string
	mu      sync.Mutex
	devices []apitypes.PushDevice
	ends    map[string]string // device id → endpoint
	sends   []keptPush
}

type keptPush struct {
	apitypes.PushSent
	To           []string                `json:"to"`
	Notification webpush.Notification    `json:"notification"`
	TTL          int                     `json:"ttl"`
	Urgency      string                  `json:"urgency"`
	Fallback     string                  `json:"fallback_email,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
	Deliveries   []apitypes.PushDelivery `json:"-"`
}

func newPushStore() *pushStore {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	key := ""
	if err == nil {
		key = base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
	}
	return &pushStore{key: key, ends: map[string]string{}}
}

// servePush answers the push paths on the edge, as the platform's gate does.
func (e *edge) servePush(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case webpush.PagePath:
		pushScript(w, webpush.Page, "public, max-age=3600")
	case webpush.WorkerPath:
		w.Header().Set("Service-Worker-Allowed", "/.whisk/push/")
		pushScript(w, webpush.Worker, "no-cache")
	case webpush.KeyPath:
		writeJSON(w, 200, map[string]string{"key": e.stub.push.key})
	case webpush.InstallPath:
		in := webpush.Install{Name: e.stub.manifest.Name}
		if look := e.stub.manifest.SignIn; look != nil {
			in.Colour, in.Background = look.Colour, look.Background
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(webpush.Manifest(in))
	default:
		return false
	}
	return true
}

func pushScript(w http.ResponseWriter, body []byte, cache string) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", cache)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func pushProblem(w http.ResponseWriter, r *http.Request, p *webpush.Problem) {
	status := 400
	writeError(w, r, status, werrors.New(p.Code, p.Message, p.Fix, p.Details))
}

// push answers …/push/devices, …/push/send, …/push/sent and …/push/sends/{id}.
func (a *api) push(w http.ResponseWriter, r *http.Request, rest string) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("The push endpoints need the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	ps := a.stub.push
	switch {
	case rest == "/devices" && r.Method == http.MethodPost:
		ps.addDevice(w, r)
	case rest == "/devices" && r.Method == http.MethodGet:
		writeJSON(w, 200, apitypes.List[apitypes.PushDevice]{Items: ps.list(r.URL.Query().Get("user"))})
	case rest == "/devices" && r.Method == http.MethodDelete:
		ps.removeDevices(w, r)
	case rest == "/send" && r.Method == http.MethodPost:
		ps.send(w, r)
	case rest == "/sent" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]any{"notifications": ps.sent()})
	case strings.HasPrefix(rest, "/sends/") && r.Method == http.MethodGet:
		ps.status(w, r, strings.TrimPrefix(rest, "/sends/"))
	default:
		writeError(w, r, 404, werrors.New("NOT_FOUND", "No such push route.", "The push routes are …/push/devices, …/push/send, …/push/sent and …/push/sends/{id}.", nil))
	}
}

func (ps *pushStore) addDevice(w http.ResponseWriter, r *http.Request) {
	var in apitypes.AddPushDeviceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The request body is not JSON.", "Send {user, subscription, label}.", nil))
		return
	}
	if p := webpush.CheckUser(in.User); p != nil {
		pushProblem(w, r, p)
		return
	}
	sub, p := webpush.CheckSubscription(in.Subscription.Endpoint, in.Subscription.Keys.P256DH, in.Subscription.Keys.Auth)
	if p != nil {
		pushProblem(w, r, p)
		return
	}
	writeJSON(w, 201, ps.keep(in, sub.Endpoint))
}

// keep adds a device, or moves the one with endpoint to in's person.
func (ps *pushStore) keep(in apitypes.AddPushDeviceRequest, endpoint string) apitypes.PushDevice {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for i, d := range ps.devices {
		if ps.ends[d.ID] == endpoint {
			ps.devices[i].User, ps.devices[i].Label = in.User, in.Label
			return ps.devices[i]
		}
	}
	service, _ := webpush.Service(endpoint)
	d := apitypes.PushDevice{ID: newULID(), User: in.User, Label: in.Label, Service: service, CreatedAt: time.Now().UTC()}
	ps.devices = append(ps.devices, d)
	ps.ends[d.ID] = endpoint
	return d
}

func (ps *pushStore) list(user string) []apitypes.PushDevice {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := []apitypes.PushDevice{}
	for _, d := range ps.devices {
		if user == "" || d.User == user {
			out = append(out, d)
		}
	}
	return out
}

func (ps *pushStore) removeDevices(w http.ResponseWriter, r *http.Request) {
	user, endpoint := r.URL.Query().Get("user"), r.URL.Query().Get("endpoint")
	if user == "" && endpoint == "" {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "Name the person, the device's endpoint, or both.",
			"Send ?user=<key> to turn off every device of theirs, or ?endpoint=<subscription endpoint> for one.", nil))
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	kept, removed := ps.devices[:0], 0
	for _, d := range ps.devices {
		if (user == "" || d.User == user) && (endpoint == "" || ps.ends[d.ID] == endpoint) {
			delete(ps.ends, d.ID)
			removed++
			continue
		}
		kept = append(kept, d)
	}
	ps.devices = kept
	writeJSON(w, 200, apitypes.PushDevicesRemoved{Removed: removed})
}

func (ps *pushStore) send(w http.ResponseWriter, r *http.Request) {
	var in apitypes.SendPushRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The request body is not JSON.", "Send {to, title, body, url}.", nil))
		return
	}
	send, p := webpush.CheckSend(in)
	if p != nil {
		pushProblem(w, r, p)
		return
	}
	k := keptPush{PushSent: apitypes.PushSent{ID: newULID()}, To: send.Users, Notification: send.Notification,
		TTL: send.TTL, Urgency: send.Urgency, Fallback: send.FallbackEmail, CreatedAt: time.Now().UTC()}
	k.Notification.Send = k.ID
	ps.mu.Lock()
	defer ps.mu.Unlock()
	k.Deliveries = ps.copies(send.Users, k.CreatedAt)
	k.Devices, k.Status = len(k.Deliveries), "queued"
	if k.Devices == 0 {
		k.Status = "no_devices"
	}
	ps.sends = append(ps.sends, k)
	if len(ps.sends) > pushKeep {
		ps.sends = ps.sends[len(ps.sends)-pushKeep:]
	}
	writeJSON(w, 202, k.PushSent)
}

// copies is a kept copy for each device of users; the caller holds the lock.
func (ps *pushStore) copies(users []string, at time.Time) []apitypes.PushDelivery {
	out := []apitypes.PushDelivery{}
	for _, d := range ps.devices {
		if slices.Contains(users, d.User) {
			out = append(out, apitypes.PushDelivery{Device: d.ID, User: d.User, Status: "kept", Detail: keptPushDetail, At: &at})
		}
	}
	return out
}

func (ps *pushStore) sent() []keptPush {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := make([]keptPush, 0, len(ps.sends))
	for i := len(ps.sends) - 1; i >= 0; i-- {
		out = append(out, ps.sends[i])
	}
	return out
}

func (ps *pushStore) status(w http.ResponseWriter, r *http.Request, id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, k := range ps.sends {
		if k.ID != id {
			continue
		}
		status := "kept"
		if k.Devices == 0 {
			status = "no_devices"
		}
		writeJSON(w, 200, apitypes.PushSend{ID: k.ID, Status: status, CreatedAt: k.CreatedAt, Deliveries: k.Deliveries})
		return
	}
	writeError(w, r, 404, werrors.New("PUSH_SEND_NOT_FOUND", fmt.Sprintf("whisk dev took no notification with id %s.", id),
		fmt.Sprintf("Send the id POST …/push/send answered in this whisk dev run; the stub keeps its last %d notifications in memory.", pushKeep),
		map[string]any{"id": id}))
}
