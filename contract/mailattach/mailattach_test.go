package mailattach

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

const prefix = "app/01JAPP/"

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, n := range names {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte("x"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPlan(t *testing.T) {
	pdf := Request{Filename: "report.pdf", Content: b64("%PDF-1.7 report")}
	logo := Request{Filename: "logo.png", Content: b64("\x89PNG logo"), ContentID: "logo"}
	html := `<p><img src="cid:logo" alt="Acme"></p>`
	many := make([]Request, MaxFiles+1)
	for i := range many {
		many[i] = Request{Filename: "a.txt", Content: b64("a")}
	}
	cases := []struct {
		name string
		in   []Request
		html string
		code string
		want []File
	}{
		{name: "none", in: nil},
		{name: "a pdf and an inline logo", in: []Request{pdf, logo}, html: html, want: []File{
			{Filename: "report.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7 report")},
			{Filename: "logo.png", ContentType: "image/png", ContentID: "logo", Content: []byte("\x89PNG logo")},
		}},
		{name: "a stored file", in: []Request{{Filename: "r.pdf", StorageKey: prefix + "reports/r.pdf"}}, want: []File{
			{Filename: "r.pdf", ContentType: "application/pdf", StorageKey: prefix + "reports/r.pdf"},
		}},
		{name: "content type given wins, parameters dropped", in: []Request{{Filename: "x", Content: b64("x"), ContentType: "Text/CSV; charset=utf-8"}}, want: []File{
			{Filename: "x", ContentType: "text/csv", Content: []byte("x")},
		}},
		{name: "unknown extension", in: []Request{{Filename: "data.bin2", Content: b64("x")}}, want: []File{
			{Filename: "data.bin2", ContentType: "application/octet-stream", Content: []byte("x")},
		}},
		{name: "wrapped base64 without padding", in: []Request{{Filename: "a.txt", Content: "aGVs\nbG8"}}, want: []File{
			{Filename: "a.txt", ContentType: "text/plain", Content: []byte("hello")},
		}},
		{name: "content id in brackets", in: []Request{{Filename: "l.png", Content: b64("x"), ContentID: "<logo@acme>"}}, html: `<img src="CID:logo@acme">`, want: []File{
			{Filename: "l.png", ContentType: "image/png", ContentID: "logo@acme", Content: []byte("x")},
		}},
		{name: "too many", in: many, code: CodeInvalid},
		{name: "no filename", in: []Request{{Content: b64("x")}}, code: CodeInvalid},
		{name: "a path for a filename", in: []Request{{Filename: "../etc/passwd", Content: b64("x")}}, code: CodeInvalid},
		{name: "a line break in a filename", in: []Request{{Filename: "a\r\nb.pdf", Content: b64("x")}}, code: CodeInvalid},
		{name: "both content and key", in: []Request{{Filename: "a.pdf", Content: b64("x"), StorageKey: prefix + "a.pdf"}}, code: CodeInvalid},
		{name: "neither", in: []Request{{Filename: "a.pdf"}}, code: CodeInvalid},
		{name: "not base64", in: []Request{{Filename: "a.pdf", Content: "%%%"}}, code: CodeInvalid},
		{name: "a bad content type", in: []Request{{Filename: "a.pdf", Content: b64("x"), ContentType: "pdf"}}, code: CodeInvalid},
		{name: "another app's file", in: []Request{{Filename: "a.pdf", StorageKey: "app/01JOTHER/a.pdf"}}, code: CodeNotFound},
		{name: "the platform's files", in: []Request{{Filename: "a.zip", StorageKey: "exports/x.zip"}}, code: CodeNotFound},
		{name: "a traversal out of the prefix", in: []Request{{Filename: "a.pdf", StorageKey: prefix + "../01JOTHER/a.pdf"}}, code: CodeInvalid},
		{name: "the prefix itself", in: []Request{{Filename: "a.pdf", StorageKey: prefix}}, code: CodeInvalid},
		{name: "an executable", in: []Request{{Filename: "invoice.pdf.exe", Content: b64("x")}}, code: CodeBlocked},
		{name: "an executable with a trailing dot", in: []Request{{Filename: "run.BAT.", Content: b64("x")}}, code: CodeBlocked},
		{name: "a program's content type", in: []Request{{Filename: "a.bin", Content: b64("x"), ContentType: "application/x-msdownload"}}, code: CodeBlocked},
		{name: "a script", in: []Request{{Filename: "a.js", Content: b64("x")}}, code: CodeBlocked},
		{name: "an inline image the html never shows", in: []Request{logo}, html: "<p>hi</p>", code: CodeInvalid},
		{name: "an inline image and no html", in: []Request{logo}, code: CodeInvalid},
		{name: "a cid with no image", in: []Request{pdf}, html: html, code: CodeInvalid},
		{name: "an inline pdf", in: []Request{{Filename: "r.pdf", Content: b64("x"), ContentID: "r"}}, html: `<img src="cid:r">`, code: CodeInvalid},
		{name: "a bad content id", in: []Request{{Filename: "l.png", Content: b64("x"), ContentID: "a b"}}, html: `<img src="cid:a">`, code: CodeInvalid},
		{name: "two images one id", in: []Request{logo, logo}, html: html, code: CodeInvalid},
		{name: "over the size", in: []Request{{Filename: "big.pdf", Content: base64.StdEncoding.EncodeToString(make([]byte, MaxBytes+1))}}, code: CodeTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, p := Plan(c.in, c.html, prefix)
			if c.code != "" {
				if p == nil || p.Code != c.code {
					t.Fatalf("want %s, got %v", c.code, p)
				}
				if p.Message == "" || p.Fix == "" {
					t.Fatalf("a problem needs a message and a fix: %+v", p)
				}
				return
			}
			if p != nil {
				t.Fatalf("unexpected %v", p)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %d files, want %d", len(got), len(c.want))
			}
			for i := range got {
				g, w := got[i], c.want[i]
				if g.Filename != w.Filename || g.ContentType != w.ContentType || g.ContentID != w.ContentID ||
					g.StorageKey != w.StorageKey || !bytes.Equal(g.Content, w.Content) {
					t.Errorf("file %d: got %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestCheck(t *testing.T) {
	pe := append([]byte("MZ"), make([]byte, 100)...)
	cases := []struct {
		name  string
		files []File
		code  string
	}{
		{name: "a document", files: []File{{Filename: "a.pdf", Content: []byte("%PDF")}}},
		{name: "an office document is a zip of xml", files: []File{{Filename: "a.docx", Content: zipOf(t, "word/document.xml", "[Content_Types].xml")}}},
		{name: "a short text that starts MZ", files: []File{{Filename: "a.txt", Content: []byte("MZ is a postcode")}}},
		{name: "a windows program renamed", files: []File{{Filename: "a.pdf", Content: pe}}, code: CodeBlocked},
		{name: "a linux program renamed", files: []File{{Filename: "a.pdf", Content: []byte("\x7fELF\x02\x01")}}, code: CodeBlocked},
		{name: "a mac program renamed", files: []File{{Filename: "a.pdf", Content: []byte{0xcf, 0xfa, 0xed, 0xfe, 7}}}, code: CodeBlocked},
		{name: "an archive with a program", files: []File{{Filename: "a.zip", Content: zipOf(t, "readme.txt", "setup.exe")}}, code: CodeBlocked},
		{name: "not read from storage", files: []File{{Filename: "a.pdf", StorageKey: prefix + "a.pdf"}}, code: CodeInvalid},
		{name: "empty from storage", files: []File{{Filename: "a.pdf", StorageKey: prefix + "a.pdf", Content: []byte{}}}, code: CodeInvalid},
		{name: "together too large", files: []File{{Filename: "a", Content: make([]byte, MaxBytes/2+1)}, {Filename: "b", Content: make([]byte, MaxBytes/2)}}, code: CodeTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Check(c.files)
			switch {
			case c.code == "" && p != nil:
				t.Fatalf("unexpected %v", p)
			case c.code != "" && (p == nil || p.Code != c.code):
				t.Fatalf("want %s, got %v", c.code, p)
			}
		})
	}
}

func TestBudget(t *testing.T) {
	files := []File{{Content: make([]byte, 1000)}, {StorageKey: prefix + "x"}}
	if got := Budget(files); got != MaxBytes-1000 {
		t.Fatalf("budget %d", got)
	}
	if p := TooLarge(MaxBytes + 1); p.Code != CodeTooLarge || p.Details["limit_bytes"] != MaxBytes || !strings.Contains(p.Message, "10.0 MB") {
		t.Fatalf("%+v", p)
	}
}

// Every naughty string as each field answers files or a problem with a code, and never a file
// whose name or content id could break a header.
func TestPlanNaughty(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, r := range []Request{
			{Filename: s, Content: b64("x")},
			{Filename: "a.pdf", Content: s},
			{Filename: "a.pdf", StorageKey: s},
			{Filename: "a.pdf", StorageKey: prefix + s},
			{Filename: "a.pdf", Content: b64("x"), ContentType: s},
			{Filename: "a.png", Content: b64("x"), ContentID: s},
		} {
			files, p := Plan([]Request{r}, `<img src="cid:`+s+`">`, prefix)
			if p != nil {
				if p.Code == "" || p.Message == "" || p.Fix == "" {
					t.Fatalf("%q: problem without code, message or fix: %+v", s, p)
				}
				continue
			}
			for _, f := range files {
				if strings.ContainsAny(f.Filename+f.ContentID+f.ContentType, "\r\n\x00") {
					t.Fatalf("%q: a header-breaking value got through: %+v", s, f)
				}
				if f.StorageKey != "" && (!strings.HasPrefix(f.StorageKey, prefix) || strings.Contains(f.StorageKey, "..")) {
					t.Fatalf("%q: a key outside the prefix got through: %s", s, f.StorageKey)
				}
			}
		}
	}
}
