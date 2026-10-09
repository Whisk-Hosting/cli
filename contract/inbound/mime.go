package inbound

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
)

// Message is a raw message read into what a handler needs. Text and HTML are the message's own
// bodies, decoded to UTF-8; every other part is an attachment.
type Message struct {
	MessageID   string
	From        string
	To          []string
	Cc          []string
	ReplyTo     string
	Subject     string
	Date        time.Time
	InReplyTo   string
	References  []string
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment is one part that is not the message's own text or html.
type Attachment struct {
	Filename    string
	ContentType string
	ContentID   string
	Inline      bool
	Content     []byte
}

// Bounds on reading, so a hostile message costs a bounded amount of work.
const (
	maxDepth = 10
	maxParts = 500
)

// ErrUnreadable is a message that is not a message at all: no header block.
var ErrUnreadable = errors.New("inbound: the message has no readable header")

var words = &mime.WordDecoder{CharsetReader: charsetReader}

// Parse reads a raw RFC 5322 message. It is lenient the way mail clients are: a part it cannot
// decode is kept as it arrived, a charset it does not know is read as UTF-8 with invalid bytes
// replaced, and a malformed multipart keeps the parts read before the fault. Only a message with
// no header at all is an error. Pure.
func Parse(raw []byte) (Message, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Message{}, ErrUnreadable
	}
	h := msg.Header
	m := Message{
		MessageID:  strings.TrimSpace(h.Get("Message-Id")),
		From:       decodeHeader(h.Get("From")),
		To:         addressList(h, "To"),
		Cc:         addressList(h, "Cc"),
		ReplyTo:    decodeHeader(h.Get("Reply-To")),
		Subject:    decodeHeader(h.Get("Subject")),
		InReplyTo:  strings.TrimSpace(h.Get("In-Reply-To")),
		References: strings.Fields(h.Get("References")),
	}
	if d, err := h.Date(); err == nil {
		m.Date = d
	}
	body, _ := io.ReadAll(msg.Body)
	r := reader{msg: &m}
	r.part(h, body, 0)
	return m, nil
}

// headers is the part of a header a part needs: mail.Header and a multipart part's header.
type headers interface{ Get(string) string }

type reader struct {
	msg   *Message
	parts int
}

// part reads one part: a multipart is walked, the first text/plain and text/html that are not
// attachments are the bodies, and everything else is an attachment.
func (r *reader) part(h headers, body []byte, depth int) {
	r.parts++
	if r.parts > maxParts {
		return
	}
	ctype, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || ctype == "" {
		ctype, params = "text/plain", map[string]string{"charset": "us-ascii"}
	}
	disposition, dparams, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
	if strings.HasPrefix(ctype, "multipart/") && depth < maxDepth {
		r.multipart(params["boundary"], body, depth)
		return
	}
	content := decodeTransfer(h.Get("Content-Transfer-Encoding"), body)
	filename := decodeHeader(firstNonEmpty(dparams["filename"], params["name"]))
	attached := disposition == "attachment" || filename != ""
	switch {
	case !attached && ctype == "text/plain" && r.msg.Text == "":
		r.msg.Text = toUTF8(content, params["charset"])
		return
	case !attached && ctype == "text/html" && r.msg.HTML == "":
		r.msg.HTML = toUTF8(content, params["charset"])
		return
	case !attached && strings.HasPrefix(ctype, "text/") && filename == "":
		// A second text part of the same kind (a signature, a forwarded note) joins the first,
		// as a mail client shows it.
		if ctype == "text/plain" {
			r.msg.Text += "\n" + toUTF8(content, params["charset"])
			return
		}
	}
	if filename == "" {
		filename = defaultName(ctype, len(r.msg.Attachments)+1)
	}
	r.msg.Attachments = append(r.msg.Attachments, Attachment{
		Filename:    filename,
		ContentType: ctype,
		ContentID:   strings.Trim(strings.TrimSpace(h.Get("Content-Id")), "<>"),
		Inline:      disposition == "inline",
		Content:     content,
	})
}

func (r *reader) multipart(boundary string, body []byte, depth int) {
	if boundary == "" {
		return
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		p, err := mr.NextRawPart()
		if err != nil {
			return
		}
		content, err := io.ReadAll(p)
		if err != nil {
			return
		}
		r.part(p.Header, content, depth+1)
		if r.parts > maxParts {
			return
		}
	}
}

// decodeTransfer undoes a part's Content-Transfer-Encoding. A part that does not decode is
// kept as it arrived.
func decodeTransfer(encoding string, body []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, body)
		out := make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
		n, err := base64.StdEncoding.Decode(out, clean)
		if err != nil {
			if n2, err2 := base64.RawStdEncoding.Decode(out, bytes.TrimRight(clean, "=")); err2 == nil {
				return out[:n2]
			}
			return body
		}
		return out[:n]
	case "quoted-printable":
		out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		if err != nil {
			return body
		}
		return out
	}
	return body
}

// toUTF8 reads text in its declared charset. An unknown charset, or none, is taken as UTF-8
// with anything invalid replaced, so a delivery always carries valid UTF-8.
func toUTF8(b []byte, charset string) string {
	charset = strings.ToLower(strings.TrimSpace(charset))
	if charset != "" && charset != "utf-8" && charset != "us-ascii" {
		if enc, err := htmlindex.Get(charset); err == nil {
			if out, err := enc.NewDecoder().Bytes(b); err == nil {
				return string(out)
			}
		}
	}
	return strings.ToValidUTF8(string(b), "�")
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(strings.ToLower(charset))
	if err != nil {
		return nil, fmt.Errorf("inbound: unknown charset %s", charset)
	}
	return enc.NewDecoder().Reader(input), nil
}

// decodeHeader reads RFC 2047 encoded words; a header that does not decode is kept as written,
// with anything that is not UTF-8 replaced.
func decodeHeader(s string) string {
	s = strings.TrimSpace(s)
	if out, err := words.DecodeHeader(s); err == nil {
		s = out
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return s
}

// addressList is a header's addresses, each as written ("Ana <ana@x.com>"), decoded. A list
// that does not parse is split on commas, so nothing written is lost.
func addressList(h mail.Header, name string) []string {
	raw := h.Get(name)
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	if list, err := (&mail.AddressParser{WordDecoder: words}).ParseList(raw); err == nil {
		out := make([]string, len(list))
		for i, a := range list {
			if a.Name == "" {
				out[i] = a.Address
			} else {
				out[i] = a.String()
			}
		}
		return out
	}
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if p = decodeHeader(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// defaultName names an attachment that came without a name, by its type.
func defaultName(ctype string, n int) string {
	ext := ".bin"
	switch {
	case ctype == "message/rfc822":
		ext = ".eml"
	case ctype == "text/calendar":
		ext = ".ics"
	default:
		if exts, _ := mime.ExtensionsByType(ctype); len(exts) > 0 {
			ext = exts[0]
		}
	}
	return fmt.Sprintf("attachment-%d%s", n, ext)
}

// SafeName is a filename that is one path segment of letters, digits, dots, hyphens and
// underscores, at most 100 bytes, for an object key. Pure.
func SafeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), ".")
	if len(out) > 100 {
		out = out[len(out)-100:]
	}
	if out == "" {
		out = "file"
	}
	return out
}
