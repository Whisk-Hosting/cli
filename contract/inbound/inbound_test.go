package inbound

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/contract/manifest"
)

func TestAddress(t *testing.T) {
	if got := Address("orders", "acme", "In.Whisk.Run"); got != "orders.acme@in.whisk.run" {
		t.Fatalf("Address = %q", got)
	}
}

func TestRoute(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []Target
	}{
		{"an app's own address", []string{"orders.acme@in.whisk.run"},
			[]Target{{Address: "orders.acme@in.whisk.run", App: "orders", Org: "acme"}}},
		{"a display name and capitals", []string{"Orders <ORDERS.Acme@IN.whisk.run>"},
			[]Target{{Address: "orders.acme@in.whisk.run", App: "orders", Org: "acme"}}},
		{"a tag after a plus", []string{"orders.acme+lab-a@in.whisk.run"},
			[]Target{{Address: "orders.acme+lab-a@in.whisk.run", App: "orders", Org: "acme", Tag: "lab-a"}}},
		{"a business's own domain", []string{"results@lab.example.com"},
			[]Target{{Address: "results@lab.example.com", Domain: "lab.example.com"}}},
		{"no dot names no app", []string{"orders@in.whisk.run"}, []Target{}},
		{"two dots name no app", []string{"a.b.c@in.whisk.run"}, []Target{}},
		{"a slug with a bad character", []string{"or_ders.acme@in.whisk.run"}, []Target{}},
		{"not an address", []string{"nobody", "@in.whisk.run", "x@"}, []Target{}},
		{"the same address twice", []string{"orders.acme@in.whisk.run", "Orders.Acme@in.whisk.run"},
			[]Target{{Address: "orders.acme@in.whisk.run", App: "orders", Org: "acme"}}},
		{"a subdomain of the receiving domain is another domain", []string{"x@sub.in.whisk.run"},
			[]Target{{Address: "x@sub.in.whisk.run", Domain: "sub.in.whisk.run"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Route(c.in, "in.whisk.run."); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Route = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestAllowed(t *testing.T) {
	cases := []struct {
		name  string
		allow []string
		from  string
		dmarc string
		want  bool
	}{
		{"no list allows everyone", nil, "x@y.com", "", true},
		{"no list allows even a failed check", nil, "x@y.com", "fail", true},
		{"an exact address", []string{"Results@Lab.example"}, "Lab <results@lab.example>", "pass", true},
		{"another address at the domain is not the address", []string{"results@lab.example"}, "other@lab.example", "", false},
		{"a domain entry", []string{"@lab.example"}, "other@lab.example", "", true},
		{"a domain entry does not cover a subdomain", []string{"@lab.example"}, "x@eu.lab.example", "", false},
		{"a failed DMARC check never passes a list", []string{"@lab.example"}, "x@lab.example", "fail", false},
		{"an unreadable sender", []string{"@lab.example"}, "nobody", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Allowed(c.allow, c.from, c.dmarc); got != c.want {
				t.Fatalf("Allowed = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAdmit(t *testing.T) {
	ok := Admission{Bytes: 1000, From: "a@b.com"}
	cases := []struct {
		name string
		in   func(Admission) Admission
		want string
	}{
		{"a small message", func(a Admission) Admission { return a }, ""},
		{"too large", func(a Admission) Admission { a.Bytes = MaxMessageBytes + 1; return a }, ReasonTooLarge},
		{"exactly the limit", func(a Admission) Admission { a.Bytes = MaxMessageBytes; return a }, ""},
		{"a paused business", func(a Admission) Admission { a.Paused = true; return a }, ReasonPaused},
		{"a plan without an inbox", func(a Admission) Admission { a.NotOnPlan = true; return a }, ReasonNotOnPlan},
		{"paused wins over the plan", func(a Admission) Admission { a.Paused, a.NotOnPlan = true, true; return a }, ReasonPaused},
		{"too large wins over paused", func(a Admission) Admission { a.Paused, a.Bytes = true, MaxMessageBytes+1; return a }, ReasonTooLarge},
		{"a sender not on the list", func(a Admission) Admission { a.AllowFrom = []string{"@lab.example"}; return a }, ReasonSenderNotAllowed},
		{"the minute's limit", func(a Admission) Admission { a.Counts.LastMinute = PerMinute; return a }, ReasonRateLimited},
		{"the day's limit", func(a Admission) Admission { a.Counts.Today = PerDay; return a }, ReasonRateLimited},
		{"the day's bytes", func(a Admission) Admission { a.Counts.BytesToday = BytesPerDay - 999; return a }, ReasonDailyBytes},
		{"a refused sender does not reach the rates", func(a Admission) Admission {
			a.AllowFrom, a.Counts.Today = []string{"@lab.example"}, PerDay
			return a
		}, ReasonSenderNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Admit(c.in(ok)); got != c.want {
				t.Fatalf("Admit = %q, want %q", got, c.want)
			}
			if c.want != "" && !Dropped(c.want) {
				t.Fatalf("%q is not a dropped reason", c.want)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	m := Message{
		MessageID: "<1@lab>", From: "Lab <results@lab.example>", Subject: "Results",
		Text: strings.Repeat("é", MaxBodyBytes), HTML: "<p>x</p>",
		Attachments: []Attachment{{Filename: "a.pdf", ContentType: "application/pdf"}, {Filename: "b.pdf"}},
		Date:        time.Date(2026, 10, 9, 1, 2, 3, 0, time.FixedZone("NZ", 13*3600)),
	}
	raw := File{UploadID: "u0", Key: "app/A/inbox/E/message.eml", Bytes: 99, ContentType: "message/rfc822"}
	files := []File{{UploadID: "u1", Key: "app/A/inbox/E/1-a.pdf", Bytes: 5, ContentType: "application/pdf"}}
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	d := Build(m, Target{Address: "orders.acme+x@in.whisk.run", Tag: "x"}, raw, files, map[string]string{"dmarc": "pass"}, at)
	if d.FromAddress != "results@lab.example" || d.Recipient != "orders.acme+x@in.whisk.run" || d.Tag != "x" {
		t.Fatalf("addresses: %+v", d)
	}
	if !d.TextTruncated || len(d.Text) > MaxBodyBytes || !strings.HasSuffix(d.Text, "é") || d.HTMLTruncated {
		t.Fatalf("text cut wrongly: %d bytes, truncated %v", len(d.Text), d.TextTruncated)
	}
	if len(d.Attachments) != 1 || d.AttachmentsOmitted != 1 || d.Attachments[0].UploadID != "u1" || d.Attachments[0].Filename != "a.pdf" {
		t.Fatalf("attachments: %+v omitted %d", d.Attachments, d.AttachmentsOmitted)
	}
	if d.Date == nil || d.Date.Location() != time.UTC || d.Raw != raw {
		t.Fatalf("date or raw: %+v", d)
	}
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"to":[]`, `"cc":[]`, `"references":[]`, `"raw":{"upload_id":"u0"`, `"authentication":{"dmarc":"pass"}`} {
		if !strings.Contains(string(body), field) {
			t.Errorf("the delivery lacks %s: %s", field, body[:200])
		}
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"Report 2026.pdf":                 "Report-2026.pdf",
		"../../etc/passwd":                "etcpasswd",
		"":                                "file",
		"résultat.pdf":                    "rsultat.pdf",
		strings.Repeat("a", 150) + ".pdf": strings.Repeat("a", 96) + ".pdf",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSourceNameIsTheManifests(t *testing.T) {
	if SourceName != manifest.InboxSource {
		t.Fatalf("inbound.SourceName %q, manifest.InboxSource %q", SourceName, manifest.InboxSource)
	}
}
