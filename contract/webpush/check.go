package webpush

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/whisk-run/contract/apitypes"
)

// The limits of one notification, the same on the platform and in the local stub. Everything
// in this file is pure and table-tested.
const (
	// MaxRecipients is the most people one send names.
	MaxRecipients = 500
	// MaxUserKey is the longest key an app may name a person by.
	MaxUserKey = 200
	// DefaultTTL is how many seconds a push service keeps a notification for a device that is
	// off when the send names no ttl; MaxTTL is the most a send may ask, four weeks.
	DefaultTTL = 24 * 60 * 60
	MaxTTL     = 28 * 24 * 60 * 60
	// MaxTitle and MaxBody are the most characters of each.
	MaxTitle = 200
	MaxBody  = 1000
	// MaxPayload is the most bytes of JSON one notification carries to a device. A push
	// service takes 4096 encrypted bytes; the record's header and tag take 103 of them.
	MaxPayload = 3800
	// MaxNotification is the most bytes a send's own fields may take, leaving room for the id
	// Whisk adds.
	MaxNotification = MaxPayload - 64
)

// Problem is why a request cannot be taken: an error code of errors.md and its words.
type Problem struct {
	Code    string
	Message string
	Fix     string
	Details map[string]any
}

func invalid(message, fix string) *Problem {
	return &Problem{Code: "INVALID_REQUEST", Message: message, Fix: fix}
}

// Notification is what a device is handed and shows (push-sw.js reads it).
type Notification struct {
	Title string         `json:"title"`
	Body  string         `json:"body,omitempty"`
	URL   string         `json:"url,omitempty"`
	Tag   string         `json:"tag,omitempty"`
	Icon  string         `json:"icon,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
	// Send is the send's id, which marks the push as Whisk's.
	Send string `json:"send"`
}

// Send is a checked send: who it is for, what each device shows and how it travels.
type Send struct {
	Users         []string
	Notification  Notification
	TTL           int
	Urgency       string
	FallbackEmail string
}

// CheckUser checks one key an app names a person by.
func CheckUser(s string) *Problem {
	if strings.TrimSpace(s) == "" {
		return invalid("A user key is empty.", "Name each person by the key your app knows them by.")
	}
	if len(s) > MaxUserKey || !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return invalid(fmt.Sprintf("A user key must be at most %d bytes of text without control characters.", MaxUserKey),
			"Name the person by your app's id for them, such as a database id.")
	}
	return nil
}

// Recipients reads to, one key or a list, without duplicates and in the order given.
func Recipients(v any) ([]string, *Problem) {
	fix := `Send "to" as the key your app knows the person by (X-Whisk-User-Id for Whisk's sign-in), or a list of them.`
	var raw []string
	switch got := v.(type) {
	case string:
		raw = []string{got}
	case []any:
		for _, item := range got {
			s, ok := item.(string)
			if !ok {
				return nil, invalid("A recipient is not a string.", fix)
			}
			raw = append(raw, s)
		}
	case []string:
		raw = got
	default:
		return nil, invalid("The notification names no recipient.", fix)
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if p := CheckUser(s); p != nil {
			return nil, p
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, invalid("The notification names no recipient.", fix)
	}
	if len(out) > MaxRecipients {
		return nil, &Problem{Code: "INVALID_REQUEST", Message: fmt.Sprintf("One notification may go to at most %d people; this names %d.", MaxRecipients, len(out)),
			Fix: fmt.Sprintf("Send it in batches of %d.", MaxRecipients), Details: map[string]any{"limit": MaxRecipients}}
	}
	return out, nil
}

// CheckSend checks a send and what each device is handed.
func CheckSend(in apitypes.SendPushRequest) (Send, *Problem) {
	to, p := Recipients(in.To)
	if p != nil {
		return Send{}, p
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Send{}, invalid("The notification has no title.", "Send a short title, such as who it is from or what happened.")
	}
	if utf8.RuneCountInString(title) > MaxTitle || utf8.RuneCountInString(in.Body) > MaxBody {
		return Send{}, &Problem{Code: "INVALID_REQUEST", Message: fmt.Sprintf("A title may hold %d characters and a body %d.", MaxTitle, MaxBody),
			Fix: "Shorten it; a phone shows two or three lines.", Details: map[string]any{"max_title": MaxTitle, "max_body": MaxBody}}
	}
	link, p := openURL(in.URL)
	if p != nil {
		return Send{}, p
	}
	icon, p := openURL(in.Icon)
	if p != nil {
		return Send{}, p
	}
	ttl := in.TTL
	switch {
	case ttl == 0:
		ttl = DefaultTTL
	case ttl < 0 || ttl > MaxTTL:
		return Send{}, &Problem{Code: "INVALID_REQUEST", Message: fmt.Sprintf("ttl is seconds from 1 to %d.", MaxTTL),
			Fix: "Leave ttl out for a day, or give how long the notification stays worth seeing.", Details: map[string]any{"max_ttl": MaxTTL}}
	}
	urgency, ok := Urgency(in.Urgency)
	if !ok {
		return Send{}, invalid("urgency is very-low, low, normal or high.", "Leave urgency out for normal.")
	}
	if e := in.FallbackEmail; e != "" && (!strings.Contains(e, "@") || strings.ContainsAny(e, " \r\n<>,;")) {
		return Send{}, invalid("fallback_email is not one email address.", "Send the person's address, such as the X-Whisk-Email your app was given, or leave it out.")
	}
	out := Send{Users: to, TTL: ttl, Urgency: urgency, FallbackEmail: in.FallbackEmail,
		Notification: Notification{Title: title, Body: in.Body, URL: link, Tag: in.Tag, Icon: icon, Data: in.Data}}
	b, err := json.Marshal(out.Notification)
	if err != nil {
		return Send{}, invalid("data is not JSON.", "Send data as a JSON object.")
	}
	if len(b) > MaxNotification {
		return Send{}, &Problem{Code: "PUSH_TOO_LARGE", Message: fmt.Sprintf("The notification is %d bytes; one may hold %d.", len(b), MaxNotification),
			Fix:     "Send less in body and data: put an id in data and have the page it opens fetch the rest.",
			Details: map[string]any{"bytes": len(b), "limit_bytes": MaxNotification}}
	}
	return out, nil
}

// openURL is where a tap goes, or an icon is read from: a path on the address the person turned
// notifications on at, which the service worker resolves, or a full https URL.
func openURL(v string) (string, *Problem) {
	if v == "" {
		return "", nil
	}
	fix := "Send a path on your app, such as /tickets/42, or a full https:// address."
	u, err := url.Parse(v)
	if err != nil {
		return "", invalid("The notification's link is not a URL.", fix)
	}
	if u.IsAbs() {
		if u.Scheme != "https" || u.Host == "" {
			return "", invalid("The notification's link must be https.", fix)
		}
		return u.String(), nil
	}
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.HasPrefix(v, "/\\") {
		return "", invalid("The notification's link must start with /.", fix)
	}
	return v, nil
}

// Urgency is the Urgency header for an urgency a send asked for (RFC 8030 §5.3); "" is normal.
func Urgency(v string) (string, bool) {
	switch v {
	case "":
		return "normal", true
	case "very-low", "low", "normal", "high":
		return v, true
	}
	return "", false
}

// services are the push services browsers use, by the end of their host. A subscription naming
// any other host is refused, so Whisk never posts to an address an app chose.
var services = []struct{ suffix, name string }{
	{"fcm.googleapis.com", "google"},
	{"android.googleapis.com", "google"},
	{"push.services.mozilla.com", "mozilla"},
	{"push.apple.com", "apple"},
	{"notify.windows.com", "microsoft"},
}

// Service names the push service an endpoint belongs to: an https URL on the default port whose
// host is one of the known services or below one.
func Service(endpoint string) (string, bool) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range services {
		if host == s.suffix || strings.HasSuffix(host, "."+s.suffix) {
			return s.name, true
		}
	}
	return "", false
}

// Subscription is a device's push endpoint and keys, decoded.
type Subscription struct {
	Endpoint string
	// P256DH is the browser's public key, 65 bytes; Auth its 16-byte secret.
	P256DH []byte
	Auth   []byte
}

// CheckSubscription checks a subscription as a browser gave it: an https endpoint on a known
// push service, a P-256 public key and a 16-byte auth secret, both base64url.
func CheckSubscription(endpoint, p256dh, auth string) (Subscription, *Problem) {
	bad := func(message string) (Subscription, *Problem) {
		return Subscription{}, &Problem{Code: "PUSH_SUBSCRIPTION_INVALID", Message: message,
			Fix: "Send the subscription PushManager.subscribe answered, as its toJSON() gives it: {endpoint, keys: {p256dh, auth}}."}
	}
	if endpoint == "" || p256dh == "" || auth == "" {
		return bad("The subscription is missing its endpoint or keys.")
	}
	if _, ok := Service(endpoint); !ok {
		return bad("The subscription's endpoint is not a browser push service.")
	}
	pub, err := unb64(p256dh)
	if err != nil {
		return bad("The subscription's p256dh key is not base64url.")
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return bad("The subscription's p256dh key is not a P-256 public key.")
	}
	secret, err := unb64(auth)
	if err != nil || len(secret) != 16 {
		return bad("The subscription's auth secret is not 16 bytes of base64url.")
	}
	return Subscription{Endpoint: endpoint, P256DH: pub, Auth: secret}, nil
}

// unb64 reads base64url with or without padding, as browsers differ.
func unb64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
