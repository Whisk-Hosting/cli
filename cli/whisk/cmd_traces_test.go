package whisk

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

const (
	testTraceID   = "01923f7a8b9c0d1e2f30415263748596"
	testRequestID = "01J9F7N2WC1MGJYC54CHS3J8MP"
)

func TestSpanTree(t *testing.T) {
	sp := func(id, parent string) api.Span { return api.Span{SpanID: id, ParentSpanID: parent, Name: id} }
	type row struct {
		ID    string
		Depth int
	}
	cases := []struct {
		name  string
		spans []api.Span
		want  []row
	}{
		{"empty", nil, []row{}},
		{"one root", []api.Span{sp("a", "")}, []row{{"a", 0}}},
		{"edge parent is absent, so the entry span is a root", []api.Span{sp("a", "ffffffffffffffff")}, []row{{"a", 0}}},
		{"children under parents, siblings in start order",
			[]api.Span{sp("a", ""), sp("b", "a"), sp("c", "a"), sp("d", "b"), sp("e", "c")},
			[]row{{"a", 0}, {"b", 1}, {"d", 2}, {"c", 1}, {"e", 2}}},
		{"a child listed before its parent still sits under it",
			[]api.Span{sp("b", "a"), sp("a", "")},
			[]row{{"a", 0}, {"b", 1}}},
		{"two roots", []api.Span{sp("a", ""), sp("x", "gone"), sp("b", "a"), sp("y", "x")},
			[]row{{"a", 0}, {"b", 1}, {"x", 0}, {"y", 1}}},
		{"own parent is a root", []api.Span{sp("a", "a")}, []row{{"a", 0}}},
		{"a parent cycle is still shown", []api.Span{sp("r", ""), sp("a", "b"), sp("b", "a")},
			[]row{{"r", 0}, {"a", 0}, {"b", 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := []row{}
			for _, r := range spanTree(c.spans) {
				got = append(got, row{r.Span.SpanID, r.Depth})
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestSpanBar(t *testing.T) {
	cases := []struct {
		offset, dur, total float64
		width              int
		want               string
	}{
		{0, 100, 100, 10, "[==========]"},
		{0, 50, 100, 10, "[=====     ]"},
		{50, 50, 100, 10, "[     =====]"},
		{25, 10, 100, 10, "[  =       ]"},
		{99, 0.1, 100, 10, "[         =]"},
		{0, 0.001, 100, 10, "[=         ]"},
		{90, 50, 100, 10, "[         =]"},
		{150, 10, 100, 10, "[         =]"},
		{0, 0, 0, 4, "[=   ]"},
		{-5, 10, 100, 4, "[=   ]"},
		{0, 10, 10, 0, ""},
	}
	for _, c := range cases {
		if got := spanBar(c.offset, c.dur, c.total, c.width); got != c.want {
			t.Errorf("spanBar(%v, %v, %v, %d) = %q, want %q", c.offset, c.dur, c.total, c.width, got, c.want)
		}
	}
}

func TestFormatMS(t *testing.T) {
	cases := map[float64]string{0: "0ms", 0.4234: "0.42ms", 3.2: "3.2ms", 12.54: "12.5ms", 340.4: "340ms", 999.6: "1000ms", 1250: "1.25s", 61000: "61s", -1: "0ms"}
	for in, want := range cases {
		if got := formatMS(in); got != want {
			t.Errorf("formatMS(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseMinMS(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"", 0, true}, {"500ms", 500, true}, {"2s", 2000, true}, {"1.5s", 1500, true}, {"250", 250, true},
		{"0.5ms", 1, true}, {"1m", 60000, true}, {"fast", 0, false}, {"-1s", 0, false}, {"-3", 0, false}, {"NaN", 0, false},
	}
	for _, c := range cases {
		got, err := parseMinMS(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("parseMinMS(%q) = %d, %v", c.in, got, err)
		}
	}
}

func TestNormalizeTraceID(t *testing.T) {
	cases := []struct{ in, want string }{
		{testTraceID, testTraceID},
		{strings.ToUpper(testTraceID), testTraceID},
		{testRequestID, testRequestID},
		{strings.ToLower(testRequestID), testRequestID},
		{"nope", ""},
		{"01923f7a8b9c0d1e2f30415263748zzz", ""},
		{"01J9F7N2WC1MGJYC54CHS3J8MU", ""}, // U is not in the ULID alphabet
		{"../../../../etc/passwd/xxxxxxxx", ""},
	}
	for _, c := range cases {
		got, err := normalizeTraceID(c.in)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("normalizeTraceID(%q) = %q, %v", c.in, got, err)
		}
	}
}

func TestUsageNote(t *testing.T) {
	cases := []struct {
		u    api.TraceUsage
		want string
	}{
		{api.TraceUsage{SpansToday: 1234, Limit: 100000}, ""},
		{api.TraceUsage{}, ""},
		{api.TraceUsage{SpansToday: 100000, Limit: 100000}, "The app has sent 100,000 spans today, its daily limit of 100,000; spans past it are dropped until midnight UTC."},
		{api.TraceUsage{SpansToday: 1234567, Limit: 100000}, "The app has sent 1,234,567 spans today, its daily limit of 100,000; spans past it are dropped until midnight UTC."},
	}
	for _, c := range cases {
		if got := usageNote(c.u); got != c.want {
			t.Errorf("usageNote(%+v) = %q", c.u, got)
		}
	}
}

func TestTraceRequest(t *testing.T) {
	cases := []struct {
		t    api.TraceSummary
		want string
	}{
		{api.TraceSummary{Name: "GET /notes/{id}", Method: "GET", Path: "/notes/7"}, "GET /notes/7"},
		{api.TraceSummary{Name: "nightly-sync"}, "nightly-sync"},
		{api.TraceSummary{Path: "/health"}, "/health"},
		{api.TraceSummary{}, "-"},
	}
	for _, c := range cases {
		if got := traceRequest(c.t); got != c.want {
			t.Errorf("traceRequest(%+v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestSpanProblems(t *testing.T) {
	cases := []struct {
		sp   api.Span
		want []string
	}{
		{api.Span{Status: "error"}, nil},
		{api.Span{Status: "error", StatusMessage: "card\ndeclined"}, []string{"error: card declined"}},
		{api.Span{Status: "error", Events: []api.SpanEvent{
			{Name: "log", Attributes: map[string]any{"exception.message": "not an exception"}},
			{Name: "exception", Attributes: map[string]any{"exception.type": "TypeError", "exception.message": "x is undefined"}},
			{Name: "exception", Attributes: map[string]any{"exception.message": "only a message"}},
			{Name: "exception", Attributes: map[string]any{"exception.message": 42}},
		}}, []string{"exception: TypeError: x is undefined", "exception: only a message"}},
	}
	for _, c := range cases {
		if got := spanProblems(c.sp); !reflect.DeepEqual(got, c.want) {
			t.Errorf("spanProblems(%+v) = %q, want %q", c.sp, got, c.want)
		}
	}
}

func testTrace() api.Trace {
	return api.Trace{
		TraceID: testTraceID, RequestID: testRequestID, Env: "production", DurationMS: 100,
		Spans: []api.Span{
			{SpanID: "a", ParentSpanID: "1111111111111111", Name: "POST /orders", Service: "crm", OffsetMS: 0, DurationMS: 100, Status: "error", StatusMessage: "500"},
			{SpanID: "b", ParentSpanID: "a", Name: "SELECT orders", Service: "crm", OffsetMS: 10, DurationMS: 20, Status: "unset"},
			{SpanID: "c", ParentSpanID: "a", Name: "POST api.stripe.com", Service: "crm", OffsetMS: 50, DurationMS: 50, Status: "error",
				Events: []api.SpanEvent{{Name: "exception", OffsetMS: 99, Attributes: map[string]any{"exception.type": "CardError", "exception.message": "card declined"}}}},
		},
	}
}

func TestRenderTrace(t *testing.T) {
	lines := renderTrace(testTrace(), 10)
	want := []string{
		"Trace " + testTraceID + "  request " + testRequestID + "  production",
		"",
		"     +0ms     100ms  [==========] x POST /orders",
		"                                      error: 500",
		"    +10ms      20ms  [ ==       ]     SELECT orders",
		"    +50ms      50ms  [     =====] x   POST api.stripe.com",
		"                                        exception: CardError: card declined",
	}
	// The second line carries a local time; compare the rest exactly.
	if len(lines) != len(want)+1 || !strings.HasSuffix(lines[1], "100ms  3 spans, 2 errors") {
		t.Fatalf("lines: %q", lines)
	}
	got := append([]string{lines[0]}, lines[2:]...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("waterfall:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	several := testTrace()
	several.Spans[1].Service = "billing"
	several.Truncated = true
	lines = renderTrace(several, 10)
	if !strings.Contains(strings.Join(lines, "\n"), "SELECT orders  (billing)") || !strings.Contains(lines[len(lines)-1], "first 2000") {
		t.Fatalf("services and truncation: %q", lines)
	}
	empty := renderTrace(api.Trace{TraceID: testTraceID}, 10)
	if !strings.Contains(empty[len(empty)-1], "No spans") {
		t.Fatalf("empty: %q", empty)
	}
}

// tracesAPI answers the trace routes for acme/crm and records the last list query.
func tracesAPI(items []map[string]any, usage map[string]any, query *string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/traces", func(w http.ResponseWriter, r *http.Request) {
		*query = r.URL.RawQuery
		writeJSON(w, 200, map[string]any{"items": items, "retention_days": 7, "usage": usage})
	})
	mux.HandleFunc("GET /v1/orgs/acme/apps/crm/traces/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != testTraceID && id != testRequestID {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "The trace was not found.", "fix": "Traces are kept 7 days; list them with whisk traces."}})
			return
		}
		writeJSON(w, 200, testTrace())
	})
	return httptest.NewServer(mux)
}

func TestTracesList(t *testing.T) {
	var query string
	items := []map[string]any{
		{"trace_id": testTraceID, "request_id": testRequestID, "env": "production", "name": "POST /orders", "method": "POST", "path": "/orders",
			"status_code": 500, "started_at": "2026-10-05T19:00:00.123456Z", "duration_ms": 1250.0, "spans": 3, "errors": 2},
		{"trace_id": "0192", "request_id": "01J9F7N2WC1MGJYC54CHS3J8MQ", "env": "production", "name": "nightly-sync",
			"started_at": "2026-10-05T18:00:00Z", "duration_ms": 3.2, "spans": 1, "errors": 0},
	}
	srv := tracesAPI(items, map[string]any{"spans_today": 1234, "limit": 100000}, &query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "traces")
	for _, want := range []string{"TIME", "REQUEST ID", "POST /orders", "1.25s", "500", testRequestID, "nightly-sync", "3.2ms"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("list lacks %q: %+v", want, r)
		}
	}
	if strings.Contains(r.Stdout, "daily limit") {
		t.Errorf("usage under the limit is not a note: %s", r.Stdout)
	}
	if query != "limit=50&since=1h&sort=recent" {
		t.Errorf("default query: %q", query)
	}

	r = runRemote(t, dir, srv.URL, nil, "traces", "--slowest", "--min", "500ms", "--q", "orders", "--env", "production", "--since", "6h", "--until", "1h", "--limit", "10", "--json")
	if r.Code != 0 || len(r.JSON["traces"].([]any)) != 2 || r.JSON["retention_days"] != float64(7) || r.JSON["usage"].(map[string]any)["spans_today"] != float64(1234) {
		t.Fatalf("json: %+v", r)
	}
	if query != "env=production&limit=10&min_ms=500&q=orders&since=6h&sort=slowest&until=1h" {
		t.Errorf("query: %q", query)
	}

	r = runRemote(t, dir, srv.URL, nil, "traces", "--min", "soon", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "INVALID_REQUEST" || e["fix"] == "" {
		t.Fatalf("bad --min: %+v", r)
	}
}

func TestTracesEmptyAndAtLimit(t *testing.T) {
	var query string
	srv := tracesAPI([]map[string]any{}, map[string]any{"spans_today": 100000, "limit": 100000}, &query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	r := runRemote(t, dir, srv.URL, nil, "traces")
	if r.Code != 0 || !strings.Contains(r.Stdout, "No traces in that window") || !strings.Contains(r.Stdout, "100,000 spans today") {
		t.Fatalf("empty at the limit: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "traces", "--json")
	if r.Code != 0 || r.JSON["traces"] == nil || len(r.JSON["traces"].([]any)) != 0 {
		t.Fatalf("empty json: %+v", r)
	}
}

func TestTracesShow(t *testing.T) {
	var query string
	srv := tracesAPI(nil, nil, &query)
	defer srv.Close()
	dir := boundDir(t, srv.URL)

	r := runRemote(t, dir, srv.URL, nil, "traces", "show", testRequestID)
	for _, want := range []string{"Trace " + testTraceID, "3 spans, 2 errors", "x POST /orders", "    SELECT orders", "error: 500", "exception: CardError: card declined"} {
		if r.Code != 0 || !strings.Contains(r.Stdout, want) {
			t.Errorf("show lacks %q: %+v", want, r)
		}
	}
	r = runRemote(t, dir, srv.URL, nil, "traces", "show", strings.ToUpper(testTraceID), "--json")
	tr, _ := r.JSON["trace"].(map[string]any)
	if r.Code != 0 || tr["trace_id"] != testTraceID || len(tr["spans"].([]any)) != 3 {
		t.Fatalf("show json: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "traces", "show", "01J9F7N2WC1MGJYC54CHS3J8MQ", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "NOT_FOUND" {
		t.Fatalf("unknown trace: %+v", r)
	}
	r = runRemote(t, dir, srv.URL, nil, "traces", "show", "nope", "--json")
	if e, _ := r.JSON["error"].(map[string]any); r.Code == 0 || e["code"] != "INVALID_REQUEST" {
		t.Fatalf("malformed id: %+v", r)
	}
}

func TestTracesAreTools(t *testing.T) {
	names := map[string]bool{}
	for _, tl := range mcpTools(newRoot(&session{})) {
		names[tl.Name] = true
	}
	for _, want := range []string{"traces", "traces_show", "logs", "logs_forwarding", "logs_forwarding_test"} {
		if !names[want] {
			t.Errorf("no tool %s", want)
		}
	}
}
