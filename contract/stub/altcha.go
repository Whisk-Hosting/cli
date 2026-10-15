package stub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// Altcha is the proof-of-work challenge the platform applies to routes.challenge. A challenge
// is sha256(salt + number) for a secret number under maxNumber, signed with the org key; the
// client finds the number and returns the solution.
type altcha struct {
	key       []byte
	maxNumber int
}

type altchaChallenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
	MaxNumber int    `json:"maxnumber"`
}

type altchaSolution struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int    `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

func (a altcha) newChallenge(now time.Time) altchaChallenge {
	number := int(now.UnixNano()>>8) % a.maxNumber
	salt := randomToken(12) + "?expires=" + strconv.FormatInt(now.Add(10*time.Minute).Unix(), 10)
	challenge := sha256Hex(salt + strconv.Itoa(number))
	return altchaChallenge{Algorithm: "SHA-256", Challenge: challenge, Salt: salt, Signature: a.sign(challenge), MaxNumber: a.maxNumber}
}

// solve finds the number for a challenge, for tests and the canary.
func solve(c altchaChallenge) (altchaSolution, bool) {
	for n := 0; n <= c.MaxNumber; n++ {
		if sha256Hex(c.Salt+strconv.Itoa(n)) == c.Challenge {
			return altchaSolution{Algorithm: c.Algorithm, Challenge: c.Challenge, Number: n, Salt: c.Salt, Signature: c.Signature}, true
		}
	}
	return altchaSolution{}, false
}

// verify checks a base64 solution payload as the Altcha widget submits it.
func (a altcha) verify(payload string, now time.Time) bool {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	var s altchaSolution
	if err := json.Unmarshal(raw, &s); err != nil || s.Algorithm != "SHA-256" {
		return false
	}
	if u, err := url.ParseQuery(afterQuestion(s.Salt)); err == nil {
		if exp, err := strconv.ParseInt(u.Get("expires"), 10, 64); err == nil && now.Unix() > exp {
			return false
		}
	}
	if !hmac.Equal([]byte(a.sign(s.Challenge)), []byte(s.Signature)) {
		return false
	}
	return sha256Hex(s.Salt+strconv.Itoa(s.Number)) == s.Challenge
}

func (a altcha) sign(challenge string) string {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(challenge))
	return hex.EncodeToString(m.Sum(nil))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func afterQuestion(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return s[i+1:]
		}
	}
	return ""
}

func encodeSolution(s altchaSolution) string {
	b, _ := json.Marshal(s)
	return base64.StdEncoding.EncodeToString(b)
}
