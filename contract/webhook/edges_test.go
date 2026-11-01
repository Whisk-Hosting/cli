package webhook

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// echoable takes 1 to 1024 printable ASCII characters, ! to ~ both included.
func TestEchoableBounds(t *testing.T) {
	for v, want := range map[string]bool{
		"":                        false,
		"a":                       true,
		strings.Repeat("a", 1024): true,
		strings.Repeat("a", 1025): false,
		"!":                       true,
		"~":                       true,
		" ":                       false,
		"\x7f":                    false,
		"a\x20b":                  false,
	} {
		if got := echoable(v); got != want {
			t.Errorf("echoable(%.20q, %d bytes) = %v, want %v", v, len(v), got, want)
		}
	}
}

// Without a tolerance the timestamp is signed but never read as a time, so a value in no
// known format still verifies.
func TestTimestampWithoutToleranceIsNotParsed(t *testing.T) {
	p := Preset{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{timestamp}.{body}",
		Timestamp: &HeaderSource{From: "header_name", Name: "X-Time"}}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	req := Request{Method: "POST", URL: "https://hooks.whisk.run/x", Body: []byte(`{}`), Header: http.Header{}}
	req.Header.Set("X-Time", "whenever")
	req.Header.Set("X-Sig", Sign(p, "whsec_test", req, "whenever", ""))
	if v := Verify(p, "whsec_test", req, time.Now()); !v.Verified {
		t.Errorf("got %+v, want verified", v)
	}
	p.ToleranceSeconds = 300
	if v := Verify(p, "whsec_test", req, time.Now()); v.Reason != "bad_timestamp" {
		t.Errorf("with a tolerance: got %+v, want bad_timestamp", v)
	}
}

// A signature pattern without a group yields no candidate, so nothing verifies.
func TestSignaturePatternWithoutGroup(t *testing.T) {
	p := Preset{Header: "X-Sig", Algorithm: "sha256", Encoding: "hex", Payload: "{body}", SignaturePattern: `v1=[0-9a-f]+`}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	req := Request{Method: "POST", URL: "https://hooks.whisk.run/x", Body: []byte(`{}`), Header: http.Header{}}
	req.Header.Set("X-Sig", "v1="+Sign(Preset{Algorithm: "sha256", Encoding: "hex", Payload: "{body}"}, "whsec_test", req, "", ""))
	if v := Verify(p, "whsec_test", req, time.Now()); v.Verified || v.Reason != "signature_mismatch" {
		t.Errorf("got %+v, want signature_mismatch", v)
	}
	p.SignaturePattern = `v1=([0-9a-f]+)`
	if v := Verify(p, "whsec_test", req, time.Now()); !v.Verified {
		t.Errorf("with a group: got %+v, want verified", v)
	}
}
