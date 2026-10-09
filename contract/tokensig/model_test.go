package tokensig

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// signed is one request the computer signed, as the model remembers it.
type signed struct {
	header, method, uri, token string
	body                       []byte
	at                         time.Time
}

// TestSignatureStateMachine runs a computer signing requests with its key while the server's
// clock moves, against the model of CONTROL-PLANE.md §4.4: a signature verifies for exactly the
// request it was made for (the method in any case, the request URI, the body and the token),
// with the key it was made with, within five minutes of the server's clock either way; a
// missing header is ErrMissing, a malformed one ErrMalformed, and the time is judged before
// the signature (HARNESS.md §8.5).
func TestSignatureStateMachine(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pub, priv, _ := ed25519.GenerateKey(nil)
		otherPub, _, _ := ed25519.GenerateKey(nil)
		clock := time.Unix(1788782400, 0)
		var log []signed
		methods := rapid.SampledFrom([]string{"GET", "POST", "put", "Delete"})
		uris := rapid.SampledFrom([]string{"/v1/orgs", "/v1/orgs/acme/apps?limit=5", "/v1/orgs/acme/apps/x", "/", "/v1/orgs/a\nb"})
		tokens := rapid.SampledFrom([]string{"whsk_agent_one", "whsk_agent_two", ""})

		t.Repeat(map[string]func(*rapid.T){
			"sign": func(t *rapid.T) {
				s := signed{method: methods.Draw(t, "method"), uri: uris.Draw(t, "uri"), token: tokens.Draw(t, "token"),
					body: rapid.SliceOfN(rapid.Byte(), 0, 8).Draw(t, "body"), at: clock}
				s.header = Sign(priv, s.method, s.uri, s.at, s.body, s.token)
				log = append(log, s)
			},
			"advance": func(t *rapid.T) {
				clock = clock.Add(time.Duration(rapid.Int64Range(0, 400_000).Draw(t, "ms")) * time.Millisecond)
			},
			"rewind": func(t *rapid.T) {
				clock = clock.Add(-time.Duration(rapid.Int64Range(0, 400_000).Draw(t, "ms")) * time.Millisecond)
			},
			"verify": func(t *rapid.T) {
				if len(log) == 0 {
					t.Skip("nothing signed")
				}
				s := log[rapid.IntRange(0, len(log)-1).Draw(t, "which")]
				header, method, uri, body, token, key := s.header, s.method, s.uri, s.body, s.token, pub
				changed := true
				change := rapid.SampledFrom([]string{"same", "case", "method", "uri", "body", "token", "key", "empty", "garbage", "nosig"}).Draw(t, "change")
				switch change {
				case "same":
					changed = false
				case "case":
					method, changed = strings.ToLower(method), false
				case "method":
					method = "PATCH"
				case "uri":
					uri += "&x=1"
				case "body":
					body = append(append([]byte{}, body...), 'x')
				case "token":
					token += "x"
				case "key":
					key = otherPub
				case "empty":
					header = "  "
				case "garbage":
					header = "t=" + header
				case "nosig":
					header = strings.Split(header, ",")[0]
				}
				got := Verify(key, header, method, uri, body, token, clock)

				var want error
				skew := clock.Sub(time.Unix(s.at.Unix(), 0))
				switch {
				case strings.TrimSpace(header) == "":
					want = ErrMissing
				case header != s.header:
					want = ErrMalformed
				case skew > MaxSkew || skew < -MaxSkew:
					want = ErrStale
				case changed:
					want = ErrBad
				}
				if !errors.Is(got, want) && !(want == nil && got == nil) {
					t.Fatalf("Verify after %s at skew %v = %v, the model says %v", change, skew, got, want)
				}
			},
		})
	})
}
