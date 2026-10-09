package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// naughtyStrings is the Big List of Naughty Strings from contract/naughty when the contract
// sits beside this template, plus the inputs Whisk adds to it, otherwise those additions alone:
// a copy of the template must still test alone.
func naughtyStrings(t *testing.T) []string {
	t.Helper()
	extra := []string{"", " ", "\x00", "a\x00b", "\r\n", "\xff\xfe", "\xc3\x28", "../../../../etc/passwd", `..\..\windows\win.ini`,
		"%2e%2e%2f", "/", "//", "-", "--help", "$(id)", "${HOME}", "{{.}}", "%s%s%s%n", "<script>alert(1)</script>",
		"'; DROP TABLE notes; --", "‮txt.exe", "__proto__", strings.Repeat("ab", 35000)}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contract", "naughty", "blns.json"))
	if err != nil {
		return extra
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	return append(list, extra...)
}

// Identity comes only from the X-Whisk-* headers; whatever they hold, the audience is never
// empty, the lists hold no empty entry, and a role is held only when it is listed exactly.
func TestNaughtyIdentity(t *testing.T) {
	for _, s := range naughtyStrings(t) {
		h := http.Header{"X-Whisk-Audience": {s}, "X-Whisk-Roles": {s + ",admin," + s}, "X-Whisk-Groups": {s}, "X-Whisk-Email": {s}, "Other": {s}}
		id := identityFrom(h)
		if id.Audience == "" || (s != "" && id.Audience != s) {
			t.Errorf("audience %q read as %q", s, id.Audience)
		}
		for _, r := range append(append([]string{}, id.Roles...), id.Groups...) {
			if r == "" {
				t.Errorf("roles or groups from %q hold an empty entry", s)
			}
		}
		if !id.HasRole("admin") || (s != "" && !strings.Contains(s, ",") && !id.HasRole(s)) {
			t.Errorf("roles %q: %q", s, id.Roles)
		}
		if id.HasRole("owner") && !strings.Contains(s, "owner") {
			t.Errorf("roles %q hold owner", s)
		}
		for k := range whiskHeaders(h) {
			if !strings.HasPrefix(k, "x-whisk-") {
				t.Errorf("whiskHeaders let %q through", k)
			}
		}
	}
}

// A delivery with a naughty id, time or body runs the function exactly when the platform signed
// it; anything unsigned, altered or naughty in the signature is refused with 401.
func TestNaughtyDeliveries(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	t.Setenv("WHISK_DELIVERY_KEY", base64.StdEncoding.EncodeToString(key))
	var got Delivery
	h := deliveries.Handle(memoryRecord(map[string]any{}), func(_ *http.Request, d Delivery) error { got = d; return nil })
	send := func(id, at, sig, body string, orig string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/hooks/stripe", strings.NewReader(body))
		req.Header["X-Whisk-Webhook-Id"] = []string{id}
		req.Header["X-Whisk-Webhook-Received-At"] = []string{at}
		req.Header["X-Whisk-Delivery-Signature"] = []string{sig}
		req.Header["X-Whisk-Webhook-Orig-Stripe-Signature"] = []string{orig}
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	for i, s := range naughtyStrings(t) {
		id := "evt-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "-" + itoa(i)
		at := "2026-09-23T10:15:30Z"
		if why := deliveries.Verify(http.Header{"X-Whisk-Webhook-Id": {id}, "X-Whisk-Webhook-Received-At": {at}, "X-Whisk-Delivery-Signature": {deliverySignature(key, id, at, []byte(s))}}, []byte(s)); why != "" {
			t.Errorf("body %q signed but refused: %s", s, why)
		}
		if s != "" {
			if why := deliveries.Verify(http.Header{"X-Whisk-Webhook-Id": {s}, "X-Whisk-Webhook-Received-At": {s}, "X-Whisk-Delivery-Signature": {deliverySignature(key, s, s, []byte(s))}}, []byte(s)); why != "" {
				t.Errorf("id and time %q signed but refused: %s", s, why)
			}
		}
		rec := send(id, at, deliverySignature(key, id, at, []byte(s)), s, s)
		if len(s) <= 5<<20 && (rec.Code != 200 || string(got.Body) != s || got.ID != id || got.Headers.Get("Stripe-Signature") != s) {
			t.Errorf("signed delivery %q: %d %s, got %+v", s, rec.Code, rec.Body.String(), got)
		}
		for _, bad := range []*httptest.ResponseRecorder{
			send(id, at, s, s, s),
			send(id, at, deliverySignature(key, id, at, []byte(s)), s+"x", s),
			send(s, s, s, s, s),
		} {
			if bad.Code != 401 || !strings.Contains(bad.Body.String(), `"code":"DELIVERY_UNVERIFIED"`) {
				t.Errorf("unsigned delivery %q answered %d %s", s, bad.Code, bad.Body.String())
			}
		}
	}
}

// The sync diagnostic writes the file a request names, and pulls the files an index names, only
// inside the tree's directory.
func TestNaughtySyncPaths(t *testing.T) {
	dir := t.TempDir()
	for _, s := range naughtyStrings(t) {
		for _, file := range []string{s, "a/" + s, s + "/b"} {
			if !validTreeFile("diag", file) {
				continue
			}
			target := filepath.Join(dir, filepath.FromSlash(file))
			rel, err := filepath.Rel(dir, target)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Errorf("file %q lands at %q, outside the tree", file, target)
			}
			if key := treeKey("diag", file); !strings.HasPrefix(key, os.Getenv("WHISK_STORAGE_PREFIX")+"trees/diag/") {
				t.Errorf("file %q is stored at %q", file, key)
			}
		}
		if validTreeFile(s, "a.txt") && !treeName.MatchString(s) {
			t.Errorf("tree %q accepted", s)
		}
		if p, err := treePath(dir, s); err == nil {
			rel, rerr := filepath.Rel(dir, p)
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				t.Errorf("treePath(%q) = %q, outside the tree", s, p)
			}
		}
	}
}

func TestNaughtyHelpers(t *testing.T) {
	for _, s := range naughtyStrings(t) {
		dbIdentity(s)
		dbIdentity("postgres://" + s + "@db/" + s)
		if code := errorCode([]byte(s)); code != "" && !json.Valid([]byte(s)) {
			t.Errorf("errorCode(%q) = %q from a body that is not JSON", s, code)
		}
		body, _ := json.Marshal(map[string]any{"error": map[string]string{"code": s}})
		if code := errorCode(body); code != s && strings.ToValidUTF8(s, "�") == s {
			t.Errorf("errorCode read %q as %q", s, code)
		}
		for _, item := range list(s + "," + s) {
			if item == "" {
				t.Errorf("list(%q) holds an empty entry", s)
			}
		}
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
