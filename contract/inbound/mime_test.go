package inbound

import (
	"strconv"
	"strings"
	"testing"
)

// crlf writes a message the way it travels.
func crlf(s string) []byte {
	return []byte(strings.ReplaceAll(strings.TrimPrefix(s, "\n"), "\n", "\r\n"))
}

func TestParsePlain(t *testing.T) {
	m, err := Parse(crlf(`
From: =?UTF-8?Q?Andr=C3=A9_Lab?= <results@lab.example>
To: Orders <orders.acme@in.whisk.run>, b@x.com
Cc: c@x.com
Subject: =?UTF-8?B?UsOpc3VsdGF0cw==?=
Message-ID: <abc@lab.example>
In-Reply-To: <prev@acme>
References: <a@x> <b@x>
Date: Fri, 09 Oct 2026 10:00:00 +1300
Content-Type: text/plain; charset=iso-8859-1
Content-Transfer-Encoding: quoted-printable

Caf=E9 ready.
`))
	if err != nil {
		t.Fatal(err)
	}
	if m.From != "André Lab <results@lab.example>" || m.Subject != "Résultats" || m.MessageID != "<abc@lab.example>" {
		t.Fatalf("headers: %+v", m)
	}
	if len(m.To) != 2 || m.To[0] != `"Orders" <orders.acme@in.whisk.run>` || m.To[1] != "b@x.com" || len(m.Cc) != 1 {
		t.Fatalf("to/cc: %q %q", m.To, m.Cc)
	}
	if m.InReplyTo != "<prev@acme>" || len(m.References) != 2 || m.Date.IsZero() {
		t.Fatalf("threading: %+v", m)
	}
	if strings.TrimSpace(m.Text) != "Café ready." || m.HTML != "" || len(m.Attachments) != 0 {
		t.Fatalf("body: %q %q %d", m.Text, m.HTML, len(m.Attachments))
	}
}

func TestParseMultipart(t *testing.T) {
	m, err := Parse(crlf(`
From: results@lab.example
To: orders.acme@in.whisk.run
Subject: Batch 12
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/alternative; boundary="alt"

--alt
Content-Type: text/plain; charset=utf-8

All results attached.
--alt
Content-Type: text/html; charset=utf-8

<p>All results attached.</p>
--alt--
--outer
Content-Type: application/pdf; name="batch-12.pdf"
Content-Disposition: attachment; filename="batch-12.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQK
--outer
Content-Type: image/png
Content-Disposition: inline
Content-ID: <logo@lab>
Content-Transfer-Encoding: base64

iVBORw0KGgo=
--outer
Content-Type: text/csv; charset=utf-8
Content-Disposition: attachment; filename*=UTF-8''r%C3%A9sultats.csv

id,value
1,2
--outer--
`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(m.Text) != "All results attached." || strings.TrimSpace(m.HTML) != "<p>All results attached.</p>" {
		t.Fatalf("bodies: %q %q", m.Text, m.HTML)
	}
	if len(m.Attachments) != 3 {
		t.Fatalf("attachments: %+v", m.Attachments)
	}
	pdf, logo, csv := m.Attachments[0], m.Attachments[1], m.Attachments[2]
	if pdf.Filename != "batch-12.pdf" || pdf.ContentType != "application/pdf" || string(pdf.Content) != "%PDF-1.4\n" || pdf.Inline {
		t.Fatalf("pdf: %+v %q", pdf, pdf.Content)
	}
	if !logo.Inline || logo.ContentID != "logo@lab" || !strings.HasPrefix(logo.Filename, "attachment-2") || len(logo.Content) != 8 {
		t.Fatalf("inline image: %+v", logo)
	}
	if csv.Filename != "résultats.csv" || !strings.Contains(string(csv.Content), "1,2") {
		t.Fatalf("csv: %+v", csv)
	}
}

func TestParseHostile(t *testing.T) {
	cases := map[string][]byte{
		"no header":            []byte("\x00\x01"),
		"a boundary never met": crlf("Subject: x\nContent-Type: multipart/mixed; boundary=b\n\nno parts"),
		"bad base64":           crlf("Subject: x\nContent-Type: application/pdf\nContent-Transfer-Encoding: base64\n\n!!!notbase64"),
		"deep nesting":         nested(50),
		"invalid utf-8":        []byte("Subject: \xff\xfe\r\nContent-Type: text/plain; charset=nope\r\n\r\n\xff ok"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := Parse(raw)
			if name == "no header" {
				if err == nil {
					t.Fatal("a message with no header parsed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !validUTF8(m.Subject) || !validUTF8(m.Text) {
				t.Fatalf("not UTF-8: %q %q", m.Subject, m.Text)
			}
		})
	}
}

func nested(depth int) []byte {
	var b strings.Builder
	b.WriteString("Subject: deep\r\nContent-Type: multipart/mixed; boundary=b0\r\n\r\n")
	for i := 1; i < depth; i++ {
		b.WriteString("--b" + strconv.Itoa(i-1) + "\r\nContent-Type: multipart/mixed; boundary=b" + strconv.Itoa(i) + "\r\n\r\n")
	}
	b.WriteString("--b" + strconv.Itoa(depth-1) + "\r\nContent-Type: text/plain\r\n\r\nbottom\r\n")
	return []byte(b.String())
}

func validUTF8(s string) bool { return strings.ToValidUTF8(s, "") == s }

func FuzzParse(f *testing.F) {
	f.Add(crlf("Subject: x\nContent-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain\n\nhi\n--b--\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		m, err := Parse(raw)
		if err != nil {
			return
		}
		if !validUTF8(m.Text) || !validUTF8(m.HTML) || !validUTF8(m.Subject) {
			t.Fatalf("Parse answered text that is not UTF-8")
		}
	})
}
