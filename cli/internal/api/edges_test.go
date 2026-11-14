package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// The edges of the stream readers, the log query and the calls a person may approve.

var errStop = errors.New("stop")

func TestReadSSEStopsOnTheCallersError(t *testing.T) {
	var seen []string
	fn := func(ev DeployEvent) error {
		seen = append(seen, string(ev.Status))
		if ev.Status == "building" {
			return errStop
		}
		return nil
	}
	done, err := readSSE(strings.NewReader("data: {\"status\":\"queued\"}\n\ndata: {\"status\":\"building\"}\n\ndata: {\"status\":\"live\",\"done\":true}\n\n"), fn)
	if !errors.Is(err, errStop) || done || !reflect.DeepEqual(seen, []string{"queued", "building"}) {
		t.Fatalf("done %v err %v seen %q", done, err, seen)
	}
	// The last event needs no blank line after it, and its error still stops the reader.
	seen = nil
	done, err = readSSE(strings.NewReader("data: {\"status\":\"queued\"}\n\ndata: {\"status\":\"building\"}"), fn)
	if !errors.Is(err, errStop) || done || len(seen) != 2 {
		t.Fatalf("last event: done %v err %v seen %q", done, err, seen)
	}
	done, err = readSSE(strings.NewReader("data: {\"status\":\"live\",\"done\":true}"), fn)
	if !done || err != nil {
		t.Fatalf("last event done: %v %v", done, err)
	}
}

func TestReadLineEventsBatches(t *testing.T) {
	var lines []string
	fn := func(batch []LogLine) error {
		for _, l := range batch {
			lines = append(lines, l.Line)
			if l.Line == "stop" {
				return errStop
			}
		}
		return nil
	}
	err := readLineEvents(strings.NewReader("\n: keepalive\n\ndata: [{\"line\":\"a\"}]\n\ndata: [{\"line\":\"b\"}]\n\ndata: [{\"line\":\"c\"}]"), fn)
	if err != nil || !reflect.DeepEqual(lines, []string{"a", "b", "c"}) {
		t.Fatalf("err %v lines %q", err, lines)
	}
	lines = nil
	if err := readLineEvents(strings.NewReader("data: [{\"line\":\"stop\"}]\n\ndata: [{\"line\":\"after\"}]\n\n"), fn); !errors.Is(err, errStop) || len(lines) != 1 {
		t.Fatalf("a refusal: %v %q", err, lines)
	}
	lines = nil
	if err := readLineEvents(strings.NewReader("data: [{\"line\":\"a\"}]\n\ndata: [{\"line\":\"stop\"}]"), fn); !errors.Is(err, errStop) {
		t.Fatalf("a refusal in the last batch: %v %q", err, lines)
	}
}

// chunks hands out its parts one Read at a time, an empty one included.
type chunks struct{ parts []string }

func (c *chunks) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.parts[0])
	c.parts = c.parts[1:]
	return n, nil
}

func TestCountingReportsOnlyProgress(t *testing.T) {
	var reports []int64
	c := &counting{r: &chunks{parts: []string{"", "ab", "", "cde"}}, report: func(n int64) { reports = append(reports, n) }}
	if _, err := io.ReadAll(c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reports, []int64{2, 5}) || c.n != 5 {
		t.Fatalf("reports %v, n %d", reports, c.n)
	}
	quiet := &counting{r: strings.NewReader("xy")}
	if b, err := io.ReadAll(quiet); err != nil || string(b) != "xy" || quiet.n != 2 {
		t.Fatalf("no report: %q %v %d", b, err, quiet.n)
	}
}

func TestLogQueryValues(t *testing.T) {
	for _, c := range []struct {
		q    LogQuery
		want url.Values
	}{
		{LogQuery{}, url.Values{}},
		{LogQuery{Env: "preview", Grep: "boom", Since: "5m", Limit: 1}, url.Values{"env": {"preview"}, "q": {"boom"}, "since": {"5m"}, "limit": {"1"}}},
		{LogQuery{Limit: -1}, url.Values{}},
	} {
		if got := c.q.values(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: %v, want %v", c.q, got, c.want)
		}
	}
}

func TestAskingAnswers(t *testing.T) {
	ctx := context.Background()
	ok := &recorder{status: http.StatusOK, body: `{"id":"mem_1","email":"a@example.com"}`}
	m, asked, err := ok.serve(t).InviteMember(ctx, "acme", Invite{})
	if err != nil || asked != nil || m.ID != "mem_1" {
		t.Fatalf("an answer: %+v %+v %v", m, asked, err)
	}
	empty := &recorder{status: http.StatusNoContent}
	if asked, err := empty.serve(t).RemoveMember(ctx, "acme", "mem_1"); err != nil || asked != nil {
		t.Fatalf("no body: %+v %v", asked, err)
	}
	if _, _, err := empty.serve(t).InviteMember(ctx, "acme", Invite{}); err != nil {
		t.Fatalf("no body for an answer: %v", err)
	}
	bad := &recorder{status: http.StatusOK, body: `<html>`}
	if _, _, err := bad.serve(t).InviteMember(ctx, "acme", Invite{}); err == nil {
		t.Fatal("a body that is not JSON was taken")
	}
	refused := &recorder{status: http.StatusForbidden, body: `{"error":{"code":"FORBIDDEN","message":"no"}}`}
	if asked, err := refused.serve(t).RemoveMember(ctx, "acme", "mem_1"); err == nil || asked != nil {
		t.Fatalf("a refusal: %+v %v", asked, err)
	}
}
