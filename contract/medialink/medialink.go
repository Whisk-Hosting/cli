// Package medialink is the signed media link (CONTROL-PLANE.md §6.13 "Signed links",
// CONTRACT.md §8): an address of a private uploaded image or video that works without a Whisk
// sign-in until it expires, because it carries an HMAC over exactly what it shows. The platform
// signs with a key of each app's own that it never hands out; the edge's media handler checks
// in constant time. Everything here is pure, so the platform and the stub agree byte for byte.
//
// A signed link is the media address with three query parameters added:
//
//	exp  when it stops working, in Unix seconds
//	kid  the generation of the app's key it was signed with
//	sig  base64url (no padding) of HMAC-SHA256(key, message)
//
// The message is the lines "whisk-media-link-v1", the app id, the resource, exp and kid,
// joined by "\n". The resource is what the link shows: for an image the upload and every
// parameter that changes the picture, normalised ("img/<id>?w=800&h=0&fit=contain&format=auto"),
// and for video or audio the upload as a whole ("media/<id>"), which covers its description,
// its embed page and each of its files, because the player reads several of them.
package medialink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The query parameters a signed link adds.
const (
	ParamExpires   = "exp"
	ParamKey       = "kid"
	ParamSignature = "sig"
)

const (
	// DefaultLifetime is how long a link lives when the request names no lifetime.
	DefaultLifetime = time.Hour
	// MinLifetime and MaxLifetime bound what a request may ask for. A link is a bearer
	// credential for a private file, so it is short: a working day at most.
	MinLifetime = time.Minute
	MaxLifetime = 12 * time.Hour
	// MaxLinks is how many links one request may ask for: a page's worth of thumbnails.
	MaxLinks = 100
)

const version = "whisk-media-link-v1"

// Why a signed link is refused.
var (
	// ErrMalformed: exp, kid or sig is missing or not of its form.
	ErrMalformed = errors.New("the link's exp, kid or sig is missing or malformed")
	// ErrBad: the signature is not this app's over this resource; the link was changed, made
	// for another app or size, or signed with a key since rotated.
	ErrBad = errors.New("the link's signature does not match")
	// ErrExpired: a genuine link whose time has passed.
	ErrExpired = errors.New("the link has expired")
)

// Claim is what a signed link's query says about itself.
type Claim struct {
	Expires int64
	Key     int
	Sig     []byte
}

// Present reports whether a query carries a signature at all, which is what makes a request a
// signed one: such a request is judged by its signature alone, whoever sends it.
func Present(q url.Values) bool {
	_, ok := q[ParamSignature]
	return ok
}

// Parse reads the claim from a query. Each parameter appears once.
func Parse(q url.Values) (Claim, error) {
	if len(q[ParamExpires]) != 1 || len(q[ParamKey]) != 1 || len(q[ParamSignature]) != 1 {
		return Claim{}, ErrMalformed
	}
	exp, err := strconv.ParseInt(q.Get(ParamExpires), 10, 64)
	if err != nil || exp <= 0 {
		return Claim{}, ErrMalformed
	}
	kid, err := strconv.Atoi(q.Get(ParamKey))
	if err != nil || kid < 1 {
		return Claim{}, ErrMalformed
	}
	// The decoder skips line breaks, so the length is checked on the text: one link has one
	// spelling.
	raw := q.Get(ParamSignature)
	sig, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(sig) != sha256.Size || len(raw) != base64.RawURLEncoding.EncodedLen(sha256.Size) {
		return Claim{}, ErrMalformed
	}
	return Claim{Expires: exp, Key: kid, Sig: sig}, nil
}

// Message is what is signed.
func Message(appID, resource string, exp int64, kid int) []byte {
	return []byte(strings.Join([]string{version, appID, resource, strconv.FormatInt(exp, 10), strconv.Itoa(kid)}, "\n"))
}

// Sign is the signature, base64url without padding.
func Sign(key []byte, appID, resource string, exp int64, kid int) string {
	return base64.RawURLEncoding.EncodeToString(mac(key, appID, resource, exp, kid))
}

// Query is the query a link adds to its address, in a fixed order.
func Query(exp int64, kid int, sig string) string {
	return ParamExpires + "=" + strconv.FormatInt(exp, 10) + "&" + ParamKey + "=" + strconv.Itoa(kid) + "&" + ParamSignature + "=" + sig
}

// Carry is the signed part of a request's query, to add to the addresses an answer names
// (a video's files) so they work with the same link.
func Carry(q url.Values) string {
	return Query(parseInt64(q.Get(ParamExpires)), parseInt(q.Get(ParamKey)), q.Get(ParamSignature))
}

// Verify checks a claim against the resource a request names, the signature first and in
// constant time, then the time: a link someone changed is ErrBad whatever its exp says, and
// only a genuine link is ever told it expired.
func Verify(key []byte, appID, resource string, c Claim, now time.Time) error {
	if !hmac.Equal(c.Sig, mac(key, appID, resource, c.Expires, c.Key)) {
		return ErrBad
	}
	if now.Unix() >= c.Expires {
		return ErrExpired
	}
	return nil
}

func mac(key []byte, appID, resource string, exp int64, kid int) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(Message(appID, resource, exp, kid))
	return m.Sum(nil)
}

// ImageResource is what a link to an image shows: the upload and the parameters that change
// the picture, each normalised the way the image handler reads it (a missing w or h is 0, fit
// is contain unless cover, format auto unless named, jpg is jpeg). Other parameters do not
// change the picture and are not covered, so a page may still add its own.
func ImageResource(id string, q url.Values) string {
	side := func(name string) string {
		v := strings.TrimSpace(q.Get(name))
		if v == "" {
			return "0"
		}
		if n, err := strconv.Atoi(v); err == nil {
			return strconv.Itoa(n)
		}
		return url.QueryEscape(v)
	}
	fit := strings.ToLower(strings.TrimSpace(q.Get("fit")))
	if fit == "" {
		fit = "contain"
	}
	format := strings.ToLower(strings.TrimSpace(q.Get("format")))
	switch format {
	case "":
		format = "auto"
	case "jpg":
		format = "jpeg"
	}
	return "img/" + id + "?w=" + side("w") + "&h=" + side("h") + "&fit=" + url.QueryEscape(fit) + "&format=" + url.QueryEscape(format)
}

// MediaResource is what a link to video or audio shows: the upload as a whole.
func MediaResource(id string) string { return "media/" + id }

// Resource is what a request path names, from the path as a browser sends it:
// /.whisk/img/<id> with its query, or /.whisk/media/<id> and anything under it. ok is false
// for any other path.
func Resource(path string, q url.Values) (resource string, ok bool) {
	if id, found := strings.CutPrefix(path, "/.whisk/img/"); found && id != "" && !strings.Contains(id, "/") {
		return ImageResource(id, q), true
	}
	if rest, found := strings.CutPrefix(path, "/.whisk/media/"); found {
		id, _, _ := strings.Cut(rest, "/")
		if id != "" {
			return MediaResource(id), true
		}
	}
	return "", false
}

// Kinds of address a link may be for.
const (
	KindImage = "img"
	KindMedia = "media"
)

// Address reads a path an app asks to have signed, written as the page would write it for
// someone signed in: /.whisk/img/<id> with the size it asks for, or /.whisk/media/<id> or a
// file under it (720.mp4, poster.jpg, original, embed), with no query. It answers the kind of
// address, the upload id and the query. A path that already carries exp, kid or sig, names a
// host, or is anything else is refused with the reason in words.
func Address(raw string) (kind, id string, q url.Values, err error) {
	u, perr := url.Parse(raw)
	switch {
	case perr != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Fragment != "" || !strings.HasPrefix(raw, "/"):
		return "", "", nil, fmt.Errorf("%q is not a path such as /.whisk/img/<id>?w=800 or /.whisk/media/<id>", raw)
	}
	q, perr = url.ParseQuery(u.RawQuery)
	if perr != nil {
		return "", "", nil, fmt.Errorf("%q has a query that does not parse", raw)
	}
	if q.Has(ParamExpires) || q.Has(ParamKey) || q.Has(ParamSignature) {
		return "", "", nil, fmt.Errorf("%q already carries exp, kid or sig; send the address without them", raw)
	}
	if rest, ok := strings.CutPrefix(u.Path, "/.whisk/img/"); ok && rest != "" && !strings.Contains(rest, "/") {
		return KindImage, rest, q, nil
	}
	if rest, ok := strings.CutPrefix(u.Path, "/.whisk/media/"); ok {
		id, file, nested := strings.Cut(rest, "/")
		switch {
		case id == "" || (nested && (file == "" || strings.Contains(file, "/"))):
		case u.RawQuery != "":
			return "", "", nil, fmt.Errorf("%q has a query; video and audio take none", raw)
		default:
			return KindMedia, id, q, nil
		}
	}
	return "", "", nil, fmt.Errorf("%q is not an image or media address; sign /.whisk/img/<id>?w=… or /.whisk/media/<id>", raw)
}

// SignedPath is the path a page uses: the address asked for with the link's query added.
func SignedPath(raw string, exp int64, kid int, sig string) string {
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	return raw + sep + Query(exp, kid, sig)
}

// Expiry is when a link issued now for ttl stops working: now+ttl rounded down to a step of a
// quarter of ttl (at least 15 seconds). Every link for the same thing issued within one step
// is the same address, so a page drawn again within it is answered from the browser's cache;
// a link lives between three quarters of ttl and ttl.
func Expiry(now time.Time, ttl time.Duration) time.Time {
	step := max(ttl/4, 15*time.Second).Truncate(time.Second)
	end := now.Add(ttl).Unix()
	s := int64(step / time.Second)
	return time.Unix(end-end%s, 0).UTC()
}

// Lifetime is the lifetime a request asks for in seconds: 0 is DefaultLifetime, and anything
// outside MinLifetime to MaxLifetime is refused.
func Lifetime(seconds int64) (time.Duration, bool) {
	if seconds == 0 {
		return DefaultLifetime, true
	}
	d := time.Duration(seconds) * time.Second
	if seconds < 0 || seconds > int64(MaxLifetime/time.Second) || d < MinLifetime {
		return 0, false
	}
	return d, true
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
