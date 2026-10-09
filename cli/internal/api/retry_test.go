package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRetryWait(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		attempt    int
		safe       bool
		status     int
		retryAfter string
		jitter     float64
		want       time.Duration
		again      bool
	}{
		{"transport failure, first", 1, true, 0, "", 0.5, 500 * time.Millisecond, true},
		{"502 second", 2, true, 502, "", 0.5, time.Second, true},
		{"503 third", 3, true, 503, "", 0.5, 2 * time.Second, true},
		{"504 low jitter", 1, true, 504, "", 0, 375 * time.Millisecond, true},
		{"high jitter", 1, true, 504, "", 1, 625 * time.Millisecond, true},
		{"three retries is the cap", 4, true, 503, "", 0.5, 0, false},
		{"not safe", 1, false, 503, "", 0.5, 0, false},
		{"not safe, transport", 1, false, 0, "", 0.5, 0, false},
		{"500 is a verdict", 1, true, 500, "", 0.5, 0, false},
		{"404", 1, true, 404, "", 0.5, 0, false},
		{"429 is not retried here", 1, true, 429, "3", 0.5, 0, false},
		{"Retry-After seconds", 1, true, 503, "3", 0.5, 3 * time.Second, true},
		{"Retry-After capped", 1, true, 503, "120", 0.5, 10 * time.Second, true},
		{"Retry-After huge", 1, true, 503, "99999999999999", 0.5, 10 * time.Second, true},
		{"Retry-After date", 1, true, 503, now.Add(4 * time.Second).Format(http.TimeFormat), 0.5, 4 * time.Second, true},
		{"Retry-After past date", 1, true, 503, now.Add(-time.Hour).Format(http.TimeFormat), 0.5, 0, true},
		{"Retry-After junk", 1, true, 503, "soon", 0.5, 500 * time.Millisecond, true},
		{"Retry-After negative", 1, true, 503, "-4", 0.5, 500 * time.Millisecond, true},
	}
	for _, tc := range cases {
		got, again := retryWait(tc.attempt, tc.safe, tc.status, tc.retryAfter, tc.jitter, now)
		if got != tc.want || again != tc.again {
			t.Errorf("%s: retryWait = %v, %v; want %v, %v", tc.name, got, again, tc.want, tc.again)
		}
	}
}

func TestStreamStep(t *testing.T) {
	down := &Unavailable{Err: errors.New("connection reset")}
	cases := []struct {
		name     string
		failures int
		got      bool
		err      error
		next     int
		wait     time.Duration
		again    bool
	}{
		{"clean end", 0, false, nil, 1, 500 * time.Millisecond, true},
		{"clean end after data resets", 3, true, nil, 1, 500 * time.Millisecond, true},
		{"drop, second in a row", 1, false, down, 2, time.Second, true},
		{"drop, fourth in a row", 3, false, down, 4, 4 * time.Second, true},
		{"fifth in a row gives up", 4, false, down, 5, 0, false},
		{"fifth after data goes on", 4, true, down, 1, 500 * time.Millisecond, true},
		{"503 is transient", 0, false, &Error{Status: 503, Code: "PLATFORM_UNAVAILABLE"}, 1, 500 * time.Millisecond, true},
		{"401 refuses", 0, false, &Error{Status: 401, Code: "AUTH_REQUIRED"}, 0, 0, false},
		{"500 refuses", 0, false, &Error{Status: 500, Code: "INTERNAL"}, 0, 0, false},
		{"wrapped drop", 0, false, fmt.Errorf("read: %w", down), 1, 500 * time.Millisecond, true},
		{"other error", 0, false, errors.New("bad request"), 0, 0, false},
	}
	for _, tc := range cases {
		next, wait, again := streamStep(tc.failures, tc.got, tc.err, 0.5)
		if next != tc.next || wait != tc.wait || again != tc.again {
			t.Errorf("%s: streamStep = %d, %v, %v; want %d, %v, %v", tc.name, next, wait, again, tc.next, tc.wait, tc.again)
		}
	}
}

func TestTailCursor(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	line := func(sec int, text string) LogLine {
		return LogLine{At: t0.Add(time.Duration(sec) * time.Second), Line: text}
	}
	texts := func(ls []LogLine) string {
		out := make([]string, len(ls))
		for i, l := range ls {
			out[i] = l.Line
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name    string
		batches [][]LogLine
		want    []string
		since   string
	}{
		{"nothing yet keeps the asked since", nil, nil, "1h"},
		{"one batch", [][]LogLine{{line(0, "a"), line(1, "b")}}, []string{"a,b"}, "2026-10-07T09:00:01Z"},
		{"the boundary line again is dropped", [][]LogLine{{line(0, "a"), line(1, "b")}, {line(1, "b"), line(2, "c")}}, []string{"a,b", "c"}, "2026-10-07T09:00:02Z"},
		{"a new line at the boundary time is kept", [][]LogLine{{line(1, "b")}, {line(1, "b"), line(1, "b2")}}, []string{"b", "b2"}, "2026-10-07T09:00:01Z"},
		{"same text later is kept", [][]LogLine{{line(1, "x")}, {line(2, "x")}}, []string{"x", "x"}, "2026-10-07T09:00:02Z"},
		{"a whole batch already seen", [][]LogLine{{line(3, "a"), line(3, "b")}, {line(3, "a"), line(3, "b")}}, []string{"a,b", ""}, "2026-10-07T09:00:03Z"},
	}
	for _, tc := range cases {
		cur := tailCursor{}
		var got []string
		for _, b := range tc.batches {
			before := len(cur.seen)
			fresh, next := cur.advance(b)
			if len(cur.seen) != before {
				t.Errorf("%s: advance changed the cursor it was called on", tc.name)
			}
			cur = next
			got = append(got, texts(fresh))
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: delivered %q, want %q", tc.name, got, tc.want)
		}
		if s := cur.since("1h"); s != tc.since {
			t.Errorf("%s: since %q, want %q", tc.name, s, tc.since)
		}
	}
}

// fast is a client whose waits do not wait and record how long they would have.
func fast(c *Client) *[]time.Duration {
	var mu sync.Mutex
	waits := &[]time.Duration{}
	c.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		*waits = append(*waits, d)
		mu.Unlock()
		return ctx.Err()
	}
	c.jitter = func() float64 { return 0.5 }
	return waits
}

// script answers each request with the next status; the last one repeats.
// keys reads the Idempotency-Key of every attempt so far.
func script(t *testing.T, statuses ...int) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(got)
		got = append(got, r.Header.Get("Idempotency-Key"))
		mu.Unlock()
		status := statuses[min(n, len(statuses)-1)]
		if status == 0 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		w.WriteHeader(status)
		if status < 300 {
			_, _ = io.WriteString(w, `{"user":{"id":"u1"}}`)
		} else {
			_, _ = io.WriteString(w, "gateway")
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "whsk_agent_x", "", "test"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, got...)
	}
}

func TestDoRetries(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		statuses []int
		attempts int
		ok       bool
	}{
		{"GET through two 503s", http.MethodGet, "/whoami", []int{503, 502, 200}, 3, true},
		{"GET through a dropped connection", http.MethodGet, "/whoami", []int{0, 200}, 2, true},
		{"GET gives up after three retries", http.MethodGet, "/whoami", []int{504}, 4, false},
		{"GET does not retry 500", http.MethodGet, "/whoami", []int{500, 200}, 1, false},
		{"GET does not retry 404", http.MethodGet, "/whoami", []int{404, 200}, 1, false},
		{"POST with a key retries", http.MethodPost, "/orgs/acme/apps", []int{503, 200}, 2, true},
		{"DELETE with a key retries a drop", http.MethodDelete, "/orgs/acme/apps/crm", []int{0, 204}, 2, true},
		{"a poll is never retried", http.MethodPost, "/device/token", []int{503, 200}, 1, false},
		{"the git password is never retried", http.MethodPost, "/tokens/git", []int{0, 200}, 1, false},
		{"a database query is never retried", http.MethodPost, "/orgs/acme/apps/crm/db/query", []int{503, 200}, 1, false},
		{"a new token is never retried", http.MethodPost, "/orgs/acme/tokens", []int{0, 200}, 1, false},
		{"an invite link is never retried", http.MethodPost, "/orgs/acme/members/m1/invite", []int{502, 200}, 1, false},
		{"reading the schema still retries", http.MethodGet, "/orgs/acme/apps/crm/db/schema", []int{503, 200}, 2, true},
	}
	for _, tc := range cases {
		c, got := script(t, tc.statuses...)
		fast(c)
		var out Whoami
		err := c.Do(context.Background(), tc.method, tc.path, nil, &out)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err %v", tc.name, err)
		}
		attempts := got()
		if len(attempts) != tc.attempts {
			t.Errorf("%s: %d attempts, want %d", tc.name, len(attempts), tc.attempts)
		}
		keys := map[string]bool{}
		for _, k := range attempts {
			keys[k] = true
		}
		if len(keys) != 1 {
			t.Errorf("%s: attempts carried different Idempotency-Keys %v", tc.name, keys)
		}
	}
}

func TestDoHonoursRetryAfter(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	c := New(srv.URL, "t", "", "test")
	waits := fast(c)
	if err := c.Do(context.Background(), http.MethodGet, "/whoami", nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Fatalf("waits %v, want [7s]", *waits)
	}
}

func TestDoStopsWhenTheContextEnds(t *testing.T) {
	c, got := script(t, 503)
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	if err := c.Do(ctx, http.MethodGet, "/whoami", nil, nil); !IsCode(err, "PLATFORM_UNAVAILABLE") {
		t.Fatalf("err %v", err)
	}
	if n := len(got()); n != 1 {
		t.Fatalf("%d attempts after the context ended", n)
	}
}

func TestNonJSONSuccessIsPlatformUnavailable(t *testing.T) {
	rec := &recorder{status: 200, body: "<html>Sign in to the hotel wifi</html>"}
	c := rec.serve(t)
	var out Whoami
	err := c.Do(context.Background(), http.MethodGet, "/whoami", nil, &out)
	var e *Error
	if !errors.As(err, &e) || e.Code != "PLATFORM_UNAVAILABLE" || e.Fix == "" || !strings.Contains(e.Message, "HTTP 200") {
		t.Fatalf("err %#v", err)
	}
}

// sseServer serves each connection to a log stream from the next of conns; a conn is the events
// to send and then whether to hang (send nothing more) instead of ending.
type sseConn struct {
	events []string
	hang   bool
	status int
}

func sseServer(t *testing.T, conns ...sseConn) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(queries)
		queries = append(queries, r.URL.Query().Get("since"))
		mu.Unlock()
		conn := conns[min(n, len(conns)-1)]
		if conn.status != 0 {
			w.WriteHeader(conn.status)
			_, _ = io.WriteString(w, `{"error":{"code":"AUTH_REQUIRED","message":"m","fix":"f"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		for _, ev := range conn.events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
		w.(http.Flusher).Flush()
		if conn.hang {
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "t", "", "test"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, queries...)
	}
}

func TestTailLogsReconnectsFromTheLastLine(t *testing.T) {
	c, queries := sseServer(t,
		sseConn{events: []string{`[{"at":"2026-10-07T09:00:00Z","line":"a"},{"at":"2026-10-07T09:00:01.5Z","line":"b"}]`}},
		sseConn{events: []string{`[{"at":"2026-10-07T09:00:01.5Z","line":"b"},{"at":"2026-10-07T09:00:02Z","line":"c"}]`}, hang: true},
	)
	fast(c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []string
	err := c.TailLogs(ctx, "acme", "crm", LogQuery{Since: "1h"}, func(batch []LogLine) error {
		for _, l := range batch {
			got = append(got, l.Line)
		}
		if len(got) >= 3 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("lines %v", got)
	}
	if q := queries(); len(q) != 2 || q[0] != "1h" || q[1] != "2026-10-07T09:00:01.5Z" {
		t.Fatalf("since on each connection %q", q)
	}
}

func TestTailLogsGivesUpAfterFiveEmptyEnds(t *testing.T) {
	c, queries := sseServer(t, sseConn{})
	waits := fast(c)
	err := c.TailLogs(context.Background(), "acme", "crm", LogQuery{}, func([]LogLine) error { return nil })
	var u *Unavailable
	if !errors.As(err, &u) {
		t.Fatalf("err %v", err)
	}
	if len(queries()) != 5 || len(*waits) != 4 {
		t.Fatalf("%d connections, %d waits", len(queries()), len(*waits))
	}
}

func TestTailLogsStopsOnARefusal(t *testing.T) {
	c, queries := sseServer(t, sseConn{status: 401})
	fast(c)
	err := c.TailPlatformLogs(context.Background(), PlatformLogQuery{}, func([]LogLine) error { return nil })
	if !IsCode(err, "AUTH_REQUIRED") || len(queries()) != 1 {
		t.Fatalf("err %v after %d connections", err, len(queries()))
	}
}

func TestStreamIdleDeadlineReconnects(t *testing.T) {
	c, queries := sseServer(t,
		sseConn{events: []string{`[{"at":"2026-10-07T09:00:00Z","line":"a"}]`}, hang: true},
		sseConn{events: []string{`[{"at":"2026-10-07T09:00:03Z","line":"b"}]`}, hang: true},
	)
	fast(c)
	c.streamIdle = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got []string
	err := c.TailLogs(ctx, "acme", "crm", LogQuery{}, func(batch []LogLine) error {
		for _, l := range batch {
			got = append(got, l.Line)
		}
		if len(got) == 2 {
			cancel()
		}
		return nil
	})
	if err != nil || strings.Join(got, ",") != "a,b" || len(queries()) < 2 {
		t.Fatalf("err %v, lines %v, %d connections", err, got, len(queries()))
	}
}

func TestDeployEventsIdleIsUnavailable(t *testing.T) {
	c, _ := sseServer(t, sseConn{events: []string{`{"deploy_id":"d1","status":"building"}`}, hang: true})
	c.streamIdle = 50 * time.Millisecond
	n := 0
	done, err := c.DeployEvents(context.Background(), "acme", "crm", "d1", func(DeployEvent) error { n++; return nil })
	var u *Unavailable
	if done || n != 1 || !errors.As(err, &u) || !errors.Is(err, errStreamIdle) {
		t.Fatalf("done %v, %d events, err %v", done, n, err)
	}
}
