// Package tokensig is how a request proves it comes from the computer a device-bound agent token
// was issued to (CONTROL-PLANE.md §4.4, CLI.md §2). whisk login makes an Ed25519 key pair and
// sends the public key with the device code request; the token the person approves is bound to
// it, and every API request with that token carries a Whisk-Signature header over the request.
// The CLI signs with Sign and the control plane checks with Verify; both build the same
// Canonical string, so neither can drift from the other. Everything here is pure.
package tokensig

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Header names the signature header.
const Header = "Whisk-Signature"

// MaxSkew is how far the signed time may be from the server's clock, either way.
const MaxSkew = 300 * time.Second

// version opens the canonical string, so a later scheme can never be confused with this one.
const version = "whisk-signature-v1"

// Reasons a signature is refused. Verify returns one of them, wrapped with detail.
var (
	ErrMissing   = errors.New("no Whisk-Signature header")
	ErrMalformed = errors.New("the Whisk-Signature header is not t=<unix seconds>, sig=<base64>")
	ErrStale     = errors.New("the signed time is more than five minutes from the server's clock")
	ErrBad       = errors.New("the signature does not match this request and the token's key")
)

// Canonical is the string a request's signature covers, one field per line: the version, the
// method in upper case, the path with its query exactly as sent (the request URI), the signed
// unix time, the hex SHA-256 of the body (of nothing when there is no body) and the hex
// SHA-256 of the token value, so a signature made for one token never verifies for another.
func Canonical(method, requestURI string, t int64, body []byte, token string) []byte {
	bodySum := sha256.Sum256(body)
	tokenSum := sha256.Sum256([]byte(token))
	return []byte(strings.Join([]string{
		version,
		strings.ToUpper(method),
		requestURI,
		strconv.FormatInt(t, 10),
		hex.EncodeToString(bodySum[:]),
		hex.EncodeToString(tokenSum[:]),
	}, "\n"))
}

// Sign is the Whisk-Signature header value for a request made at now.
func Sign(key ed25519.PrivateKey, method, requestURI string, now time.Time, body []byte, token string) string {
	t := now.Unix()
	sig := ed25519.Sign(key, Canonical(method, requestURI, t, body, token))
	return fmt.Sprintf("t=%d, sig=%s", t, base64.StdEncoding.EncodeToString(sig))
}

// Parse reads a header value into its time and signature.
func Parse(header string) (int64, []byte, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, nil, ErrMissing
	}
	var t int64
	var sig []byte
	var haveT, haveSig bool
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return 0, nil, ErrMalformed
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return 0, nil, ErrMalformed
			}
			t, haveT = n, true
		case "sig":
			// The value is base64 with its padding; the '=' in it survives Cut on the first '='.
			b, err := base64.StdEncoding.DecodeString(v)
			if err != nil || len(b) != ed25519.SignatureSize {
				return 0, nil, ErrMalformed
			}
			sig, haveSig = b, true
		}
	}
	if !haveT || !haveSig {
		return 0, nil, ErrMalformed
	}
	return t, sig, nil
}

// Verify checks a request's Whisk-Signature header against the public key the token is bound
// to: present and well formed, signed within MaxSkew of now, and over exactly this method,
// request URI, body and token.
func Verify(pub ed25519.PublicKey, header, method, requestURI string, body []byte, token string, now time.Time) error {
	t, sig, err := Parse(header)
	if err != nil {
		return err
	}
	skew := now.Sub(time.Unix(t, 0))
	if skew > MaxSkew || skew < -MaxSkew {
		return ErrStale
	}
	if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, Canonical(method, requestURI, t, body, token), sig) {
		return ErrBad
	}
	return nil
}

// NewKey makes a key pair: the public key as sent to the platform and the private key's seed as
// the CLI stores it, both standard base64.
func NewKey() (publicKey, privateKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return EncodePublic(pub), base64.StdEncoding.EncodeToString(priv.Seed()), nil
}

// EncodePublic is a public key as the platform stores it.
func EncodePublic(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// ParsePublic reads a public key as whisk login sends it.
func ParsePublic(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("the public key is not 32 bytes of standard base64")
	}
	return ed25519.PublicKey(b), nil
}

// ParsePrivate reads a private key as the CLI stores it (the 32-byte seed).
func ParsePrivate(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.SeedSize {
		return nil, errors.New("the stored private key is not a 32-byte seed in standard base64")
	}
	return ed25519.NewKeyFromSeed(b), nil
}
