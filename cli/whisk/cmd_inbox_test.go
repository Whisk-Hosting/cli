package whisk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"

	"github.com/whisk-run/contract/inbound"
)

// A composed test message reads back the way the platform reads one: headers, the text, and the
// attachment's name and bytes.
func TestComposeMessage(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	file := []byte(strings.Repeat("%PDF-1.4 lab result\n", 20))
	cases := []struct {
		name     string
		filename string
		file     []byte
		want     int
	}{
		{"text only", "", nil, 0},
		{"one attachment", "dir/../b7 result.pdf", file, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := composeMessage("Lab <results@lab.example>", "lab.dev@in.localhost", "Batch\n7", "See attached.", c.filename, c.file, now)
			m, err := inbound.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(m.From, "results@lab.example") || m.Subject != "Batch 7" || strings.TrimSpace(m.Text) != "See attached." || len(m.Attachments) != c.want {
				t.Fatalf("parsed: %+v", m)
			}
			if c.want == 1 && (string(m.Attachments[0].Content) != string(file) || m.Attachments[0].Filename != "b7-result.pdf") {
				t.Fatalf("attachment: %q %d bytes", m.Attachments[0].Filename, len(m.Attachments[0].Content))
			}
		})
	}
}

// sendToStub answers what the stub said, passes its errors through with their codes, and says
// DEV_NOT_RUNNING when nothing answers.
func TestSendToStub(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/undeclared" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"code":"INBOX_NOT_DECLARED","message":"no inbox","fix":"declare one"}}`))
			return
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"id":"01M","delivered":true}`))
	}))
	defer stub.Close()
	ctx := context.Background()
	if got, err := sendToStub(ctx, stub.URL+"/v1/stub/inbox", []byte("From: a\r\n\r\nhi")); err != nil || got["id"] != "01M" {
		t.Fatalf("delivered: %v %v", got, err)
	}
	if _, err := sendToStub(ctx, stub.URL+"/undeclared", nil); output.ExitCode(err) == 0 || !strings.Contains(fmt.Sprint(codeOf(err)), "INBOX_NOT_DECLARED") {
		t.Fatalf("undeclared: %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	if _, err := sendToStub(ctx, url+"/v1/stub/inbox", nil); codeOf(err) != "DEV_NOT_RUNNING" {
		t.Fatalf("nothing running: %v", err)
	}
}

func codeOf(err error) string {
	var e *output.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// An inbox on a plan without one says that mail to it is dropped and which plans include it.
func TestPrintInboxNotOnPlan(t *testing.T) {
	in := api.Inbox{Declared: true, Available: true, Address: "crm.acme@in.whisk.run", Handler: "/inbound/email"}
	var b strings.Builder
	printInbox(output.Printer{}, &b, in)
	if !strings.Contains(b.String(), "does not include an inbox") || !strings.Contains(b.String(), "Team and above") {
		t.Errorf("not on the plan:\n%s", b.String())
	}
	b.Reset()
	in.Included = true
	printInbox(output.Printer{}, &b, in)
	if strings.Contains(b.String(), "does not include") {
		t.Errorf("on the plan:\n%s", b.String())
	}
}
