package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// The platform signs every delivery it makes to a handler (CONTRACT.md §7):
// X-Whisk-Delivery-Signature is "v1=" + hex(HMAC-SHA256(key, id "\n" receivedAt "\n" body))
// where key is the app's WHISK_DELIVERY_KEY decoded from base64, id is X-Whisk-Webhook-Id and
// receivedAt is X-Whisk-Webhook-Received-At. The templates' deliveries helper verifies it; the
// stub and the control plane produce it here.

// DeliverySignatureHeader carries the signature.
const DeliverySignatureHeader = "X-Whisk-Delivery-Signature"

// DeliverySignature is the header value for one delivery.
func DeliverySignature(key []byte, id, receivedAt string, body []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id))
	m.Write([]byte("\n"))
	m.Write([]byte(receivedAt))
	m.Write([]byte("\n"))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

// VerifyDelivery reports why a delivery is not the platform's, or "" when it is: "no_key" when
// the app has no key, "service_app" when X-Whisk-Service-App marks an app-to-app call,
// "missing" when the signature or the headers it covers are absent, "mismatch" otherwise.
func VerifyDelivery(key []byte, h http.Header, body []byte) string {
	id, receivedAt, sig := h.Get("X-Whisk-Webhook-Id"), h.Get("X-Whisk-Webhook-Received-At"), h.Get(DeliverySignatureHeader)
	switch {
	case len(key) == 0:
		return "no_key"
	case h.Get("X-Whisk-Service-App") != "":
		return "service_app"
	case id == "" || receivedAt == "" || !strings.HasPrefix(sig, "v1="):
		return "missing"
	case !hmac.Equal([]byte(sig), []byte(DeliverySignature(key, id, receivedAt, body))):
		return "mismatch"
	}
	return ""
}
