// Package webhook loads webhook-presets.yaml and implements the one generic HMAC verifier the
// platform ingress and whisk-stub use. A preset is configuration; Verify is the only code.
package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract"
)

// Reserved preset names that carry no fields of their own.
const (
	PresetHMAC  = "hmac"
	PresetToken = "token"
)

// Preset is one entry of webhook-presets.yaml, or the inline hmac settings of a manifest.
type Preset struct {
	Header           string        `yaml:"header" json:"header"`
	Algorithm        string        `yaml:"algorithm" json:"algorithm"`
	Encoding         string        `yaml:"encoding" json:"encoding"`
	Payload          string        `yaml:"payload" json:"payload"`
	Timestamp        *HeaderSource `yaml:"timestamp,omitempty" json:"timestamp,omitempty"`
	TimestampFormat  string        `yaml:"timestamp_format,omitempty" json:"timestamp_format,omitempty"`
	ID               *HeaderSource `yaml:"id,omitempty" json:"id,omitempty"`
	SignaturePattern string        `yaml:"signature_pattern,omitempty" json:"signature_pattern,omitempty"`
	SignaturePrefix  string        `yaml:"signature_prefix,omitempty" json:"signature_prefix,omitempty"`
	ToleranceSeconds int           `yaml:"tolerance_seconds,omitempty" json:"tolerance_seconds,omitempty"`
}

// HeaderSource says where a timestamp or id comes from: a whole header (header_name) or a
// regexp group extracted from the signature header (header).
type HeaderSource struct {
	From    string `yaml:"from" json:"from"`
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
}

// Check reports whether the preset is internally consistent: known algorithm and encoding,
// placeholders that can be filled, patterns that compile.
func (p Preset) Check() error {
	if p.Header == "" || p.Algorithm == "" || p.Encoding == "" || p.Payload == "" {
		return errors.New("header, algorithm, encoding and payload are required")
	}
	if _, err := hasher(p.Algorithm); err != nil {
		return err
	}
	if p.Encoding != "hex" && p.Encoding != "base64" {
		return fmt.Errorf("encoding %q is not hex or base64", p.Encoding)
	}
	if strings.Contains(p.Payload, "{timestamp}") && p.Timestamp == nil {
		return errors.New("payload uses {timestamp} but no timestamp source is given")
	}
	if strings.Contains(p.Payload, "{id}") && p.ID == nil {
		return errors.New("payload uses {id} but no id source is given")
	}
	if p.ToleranceSeconds > 0 && p.Timestamp == nil {
		return errors.New("tolerance_seconds needs a timestamp source")
	}
	for _, re := range []string{p.SignaturePattern, sourcePattern(p.Timestamp), sourcePattern(p.ID)} {
		if re == "" {
			continue
		}
		if _, err := regexp.Compile(re); err != nil {
			return fmt.Errorf("pattern %q does not compile: %v", re, err)
		}
	}
	switch p.TimestampFormat {
	case "", "unix", "unix_ms", "rfc3339":
	default:
		return fmt.Errorf("timestamp_format %q is not unix, unix_ms or rfc3339", p.TimestampFormat)
	}
	for _, ph := range placeholders(p.Payload) {
		switch ph {
		case "body", "timestamp", "id", "method", "url", "form_sorted":
		default:
			return fmt.Errorf("payload placeholder {%s} is not known", ph)
		}
	}
	return nil
}

func sourcePattern(s *HeaderSource) string {
	if s == nil {
		return ""
	}
	return s.Pattern
}

var rePlaceholder = regexp.MustCompile(`\{([a-z_]+)\}`)

func placeholders(tpl string) []string {
	var out []string
	for _, m := range rePlaceholder.FindAllStringSubmatch(tpl, -1) {
		out = append(out, m[1])
	}
	return out
}

var loaded = func() map[string]Preset {
	m, err := Load(contract.PresetsYAML)
	if err != nil {
		panic("webhook-presets.yaml: " + err.Error())
	}
	return m
}()

// Presets returns the embedded preset table, including the reserved hmac and token entries.
func Presets() map[string]Preset { return loaded }

// Load parses a presets document and checks every preset.
func Load(src []byte) (map[string]Preset, error) {
	var raw map[string]Preset
	if err := yaml.Unmarshal(src, &raw); err != nil {
		return nil, err
	}
	for name, p := range raw {
		if name == PresetHMAC || name == PresetToken {
			continue
		}
		if err := p.Check(); err != nil {
			return nil, fmt.Errorf("preset %s: %w", name, err)
		}
	}
	return raw, nil
}

// Names returns the preset names in order.
func Names() []string {
	out := make([]string, 0, len(loaded))
	for k := range loaded {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Verdict is the outcome of Verify.
type Verdict struct {
	Verified bool `json:"verified"`
	// Reason is one of "" (verified), "missing_signature", "signature_mismatch",
	// "missing_timestamp", "bad_timestamp", "stale_timestamp", "missing_id", "no_secret".
	Reason string `json:"reason,omitempty"`
	// ID is the provider message id when the preset extracts one.
	ID string `json:"id,omitempty"`
}

// Request is what the verifier needs from a delivery. URL is the full URL the provider called.
type Request struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// Verify checks a delivery against a preset with the given secret. now is the verification
// time for the tolerance check. Verification is constant-time on the signature comparison and
// tests every candidate signature in the header.
func Verify(p Preset, secret string, req Request, now time.Time) Verdict {
	if secret == "" {
		return Verdict{Reason: "no_secret"}
	}
	sigHeader := req.Header.Get(p.Header)
	if sigHeader == "" {
		return Verdict{Reason: "missing_signature"}
	}
	vars := map[string]string{"body": string(req.Body), "method": strings.ToUpper(req.Method), "url": req.URL}
	if strings.Contains(p.Payload, "{form_sorted}") {
		vars["form_sorted"] = formSorted(req.Body)
	}
	verdict := Verdict{}
	if p.Timestamp != nil {
		ts, ok := extract(p.Timestamp, sigHeader, req.Header)
		if !ok {
			return Verdict{Reason: "missing_timestamp"}
		}
		vars["timestamp"] = ts
		if p.ToleranceSeconds > 0 {
			t, err := parseTimestamp(ts, p.TimestampFormat)
			if err != nil {
				return Verdict{Reason: "bad_timestamp"}
			}
			if d := now.Sub(t); d > time.Duration(p.ToleranceSeconds)*time.Second || d < -time.Duration(p.ToleranceSeconds)*time.Second {
				return Verdict{Reason: "stale_timestamp"}
			}
		}
	}
	if p.ID != nil {
		id, ok := extract(p.ID, sigHeader, req.Header)
		if !ok {
			return Verdict{Reason: "missing_id"}
		}
		vars["id"] = id
		verdict.ID = id
	}
	payload := rePlaceholder.ReplaceAllStringFunc(p.Payload, func(ph string) string { return vars[ph[1:len(ph)-1]] })
	expected := mac(p.Algorithm, secret, payload)
	for _, candidate := range candidates(p, sigHeader) {
		got, err := decode(p.Encoding, candidate)
		if err != nil {
			continue
		}
		// An unknown algorithm has no MAC; an empty candidate must not equal it.
		if len(expected) > 0 && subtle.ConstantTimeCompare(got, expected) == 1 {
			verdict.Verified = true
			return verdict
		}
	}
	verdict.Reason = "signature_mismatch"
	return verdict
}

// Sign produces the signature header value for a request, for tests, fixtures and the stub's
// tunnel. It fills the header in the same shape the provider would use.
func Sign(p Preset, secret string, req Request, timestamp, id string) string {
	vars := map[string]string{"body": string(req.Body), "method": strings.ToUpper(req.Method), "url": req.URL, "timestamp": timestamp, "id": id}
	if strings.Contains(p.Payload, "{form_sorted}") {
		vars["form_sorted"] = formSorted(req.Body)
	}
	payload := rePlaceholder.ReplaceAllStringFunc(p.Payload, func(ph string) string { return vars[ph[1:len(ph)-1]] })
	sig := encode(p.Encoding, mac(p.Algorithm, secret, payload))
	switch {
	case p.SignaturePattern != "" && p.Timestamp != nil && p.Timestamp.From == "header":
		// Stripe shape: t=<ts>,v1=<sig>
		return "t=" + timestamp + "," + "v1=" + sig
	case p.SignaturePrefix != "":
		return p.SignaturePrefix + sig
	default:
		return sig
	}
}

func extract(src *HeaderSource, sigHeader string, h http.Header) (string, bool) {
	switch src.From {
	case "header_name":
		v := h.Get(src.Name)
		return v, v != ""
	case "header":
		re := regexp.MustCompile(src.Pattern)
		m := re.FindStringSubmatch(sigHeader)
		if len(m) < 2 {
			return "", false
		}
		return m[1], true
	}
	return "", false
}

func candidates(p Preset, sigHeader string) []string {
	if p.SignaturePattern != "" {
		re := regexp.MustCompile(p.SignaturePattern)
		var out []string
		for _, m := range re.FindAllStringSubmatch(sigHeader, -1) {
			if len(m) > 1 {
				out = append(out, m[1])
			}
		}
		return out
	}
	// Space-separated lists (Standard Webhooks) and single values.
	var out []string
	for _, part := range strings.Fields(sigHeader) {
		if p.SignaturePrefix != "" {
			if !strings.HasPrefix(part, p.SignaturePrefix) {
				continue
			}
			part = strings.TrimPrefix(part, p.SignaturePrefix)
		}
		out = append(out, part)
	}
	return out
}

func parseTimestamp(s, format string) (time.Time, error) {
	switch format {
	case "unix_ms":
		n, err := strconv.ParseInt(s, 10, 64)
		return time.UnixMilli(n), err
	case "rfc3339":
		return time.Parse(time.RFC3339, s)
	default:
		n, err := strconv.ParseInt(s, 10, 64)
		return time.Unix(n, 0), err
	}
}

func hasher(alg string) (func() hash.Hash, error) {
	switch alg {
	case "sha1":
		return sha1.New, nil
	case "sha256":
		return sha256.New, nil
	case "sha512":
		return sha512.New, nil
	}
	return nil, fmt.Errorf("algorithm %q is not sha1, sha256 or sha512", alg)
}

func mac(alg, secret, payload string) []byte {
	h, err := hasher(alg)
	if err != nil {
		return nil
	}
	m := hmac.New(h, []byte(secret))
	m.Write([]byte(payload))
	return m.Sum(nil)
}

func decode(enc, s string) ([]byte, error) {
	if enc == "base64" {
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			return b, nil
		}
		return base64.RawStdEncoding.DecodeString(s)
	}
	return hex.DecodeString(s)
}

func encode(enc string, b []byte) string {
	if enc == "base64" {
		return base64.StdEncoding.EncodeToString(b)
	}
	return hex.EncodeToString(b)
}

// formSorted renders a form-encoded body as Twilio signs it: fields sorted by name, each
// appended as name followed by value, no separators.
func formSorted(body []byte) string {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		for _, v := range values[k] {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	return b.String()
}
