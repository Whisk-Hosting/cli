// Package release is the pure half of `whisk update` and of the release signing that `make
// cli-dist` does: asset names, the SHA256SUMS format, and Ed25519 detached signatures over it.
package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// AssetName is the file under /dl/ for a platform: whisk_<os>_<arch>[.exe].
func AssetName(goos, goarch string) string {
	name := "whisk_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// ParseSums reads SHA256SUMS ("<hex>  <name>" per line, as sha256sum writes it) into
// name → hex digest. Lines that are not two fields are ignored.
func ParseSums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out
}

// Digest is the hex SHA-256 of data.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Newer reports whether an update is wanted: the served version differs from the built-in one.
// Versions are opaque strings from git describe, so "different" is the test; a dev build
// always wants the served release, and an empty served version never does.
func Newer(current, served string) bool {
	served = strings.TrimSpace(served)
	return served != "" && served != strings.TrimSpace(current)
}

// Stamp is the release line `make cli-dist` appends to SHA256SUMS, so the signature covers
// which version a release is and when it was built: "# whisk <version> released <unix>". The
// line has more than two fields, so ParseSums and the install scripts skip it.
type Stamp struct {
	Version  string
	Released int64
}

// StampLine renders the release line.
func StampLine(st Stamp) string {
	return fmt.Sprintf("# whisk %s released %d", st.Version, st.Released)
}

// ParseStamp finds the release line in SHA256SUMS.
func ParseStamp(sums string) (Stamp, bool) {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) != 5 || f[0] != "#" || f[1] != "whisk" || f[3] != "released" {
			continue
		}
		var t int64
		if _, err := fmt.Sscan(f[4], &t); err != nil || t <= 0 {
			continue
		}
		return Stamp{Version: f[2], Released: t}, true
	}
	return Stamp{}, false
}

// Refusal says why whisk update will not install a release; empty means it may.
type Refusal string

const (
	RefuseUnstamped Refusal = "unstamped"
	RefuseOlder     Refusal = "older"
)

// CheckStamp decides whether a verified release may replace the running build, which was
// built at released (0 when unknown, a development build). A signed release must carry its
// stamp, so an old release cannot be served without saying it is old; one built before the
// running binary is refused unless force says to reinstall it anyway.
func CheckStamp(released int64, signed bool, st Stamp, stamped, force bool) Refusal {
	switch {
	case signed && !stamped:
		return RefuseUnstamped
	case stamped && released > 0 && st.Released < released && !force:
		return RefuseOlder
	}
	return ""
}

// Keypair is a release signing key: the private half as `make release-key` writes it to
// WHISK_RELEASE_KEY (base64 of the 64-byte Ed25519 private key, one line) and the public half
// as it is pasted into release_pubkey.go (base64 of the 32-byte public key).
type Keypair struct {
	Private string
	Public  string
}

// GenerateKey makes a new signing key.
func GenerateKey() (Keypair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Keypair{}, err
	}
	return Keypair{Private: base64.StdEncoding.EncodeToString(priv), Public: base64.StdEncoding.EncodeToString(pub)}, nil
}

// PublicKey derives the public half, as release_pubkey.go holds it, from a private key, so a
// release can check that the key it signs with is the one the CLI it builds will trust.
func PublicKey(privateB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateB64))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return "", errors.New("the release key is not a base64 Ed25519 private key; run make release-key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.PrivateKey(raw).Public().(ed25519.PublicKey)), nil
}

// Sign produces the detached signature of data (SHA256SUMS) as one base64 line, the content
// of SHA256SUMS.sig.
func Sign(privateB64 string, data []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateB64))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return "", errors.New("the release key is not a base64 Ed25519 private key; run make release-key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(raw), data)), nil
}

// ErrBadSignature is returned when SHA256SUMS.sig does not sign SHA256SUMS under the key.
var ErrBadSignature = errors.New("SHA256SUMS.sig does not match SHA256SUMS under the embedded release key")

// Verify checks a detached signature against the public key. An empty public key means the
// build cannot verify and returns (false, nil); a bad signature returns ErrBadSignature.
func Verify(publicB64 string, data []byte, signatureB64 string) (verified bool, err error) {
	if strings.TrimSpace(publicB64) == "" {
		return false, nil
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicB64))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("the embedded release public key is not a base64 Ed25519 key")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false, ErrBadSignature
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return false, ErrBadSignature
	}
	return true, nil
}
