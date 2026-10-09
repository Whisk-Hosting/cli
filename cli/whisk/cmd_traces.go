package whisk

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// tracesCmd is where the app's time went, request by request (CLI.md §5.5): the traces its
// OpenTelemetry SDK sent, one line each, and one trace as a waterfall.
func tracesCmd(s *session) *cobra.Command {
	var since, until, env, q, minDur string
	var slowest bool
	var limit int
	c := &cobra.Command{
		Use:   "traces",
		Short: "Where the app's time went, request by request",
		Long: `Lists the traces the app sent: when, how long, the status code, the request, how many spans
and how many of them failed, and the request id. The platform points the app's OpenTelemetry SDK
at Whisk; a trace's id is the request's X-Whisk-Request-Id, so a log line's request_id opens it
with whisk traces show. --since and --until take a duration (30m, 6h, 7d) or an RFC 3339 time.
--slowest sorts by duration, --min keeps traces at least that long (500ms, 2s), --q matches the
request's name or path. Traces are kept 7 days.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			minMS, err := parseMinMS(minDur)
			if err != nil {
				return err
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			query := api.TraceQuery{Env: env, Since: since, Until: until, Q: q, MinMS: minMS, Sort: "recent", Limit: limit}
			if slowest {
				query.Sort = "slowest"
			}
			list, err := client.Traces(s.ctx, org, app, query)
			if err != nil {
				return wrap(err)
			}
			items := list.Items
			if items == nil {
				items = []api.TraceSummary{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "traces": items, "retention_days": list.RetentionDays, "usage": list.Usage}, func(w io.Writer) {
				if len(items) == 0 {
					fmt.Fprintln(w, "No traces in that window. The platform sets OTEL_EXPORTER_OTLP_TRACES_ENDPOINT for the app; once the app starts OpenTelemetry with it, every request it serves appears here.")
				} else {
					rows := make([][]string, len(items))
					for i, t := range items {
						rows[i] = traceRow(t)
					}
					s.printer.Table(w, []string{"TIME", "DURATION", "STATUS", "REQUEST", "SPANS", "ERRORS", "REQUEST ID"}, rows)
				}
				if line := usageNote(list.Usage); line != "" {
					fmt.Fprintln(w, s.printer.Warn(line))
				}
			})
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&since, "since", "1h", "how far back to read: 30m, 6h, 7d, or an RFC 3339 time")
	f.StringVar(&until, "until", "", "the end of the window: a duration ago or an RFC 3339 time")
	f.StringVar(&env, "env", "", "one environment: production or preview:<branch>")
	f.BoolVar(&slowest, "slowest", false, "the slowest traces first, instead of the most recent")
	f.StringVar(&minDur, "min", "", "only traces at least this long: 500ms, 2s")
	f.StringVar(&q, "q", "", "text the request's name or path contains")
	f.IntVar(&limit, "limit", 50, "how many traces to show, at most 200")
	c.AddCommand(traceShowCmd(s))
	return c
}

// traceShowCmd prints one trace as a waterfall.
func traceShowCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "show <trace-id-or-request-id>",
		Short: "One trace as a waterfall of its spans",
		Long: `Prints every span of one trace, indented under its parent, with when it started from the
trace's start, how long it took and a bar to scale. A failed span is marked x and followed by its
status message and the message of any exception it recorded. The id is the 32 hex character trace
id or the request's 26 character X-Whisk-Request-Id, as a log line's request_id prints it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := normalizeTraceID(args[0])
			if err != nil {
				return err
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			t, err := client.Trace(s.ctx, org, app, id)
			if err != nil {
				return wrap(err)
			}
			if t.Spans == nil {
				t.Spans = []api.Span{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "trace": t}, func(w io.Writer) {
				for _, line := range renderTrace(t, waterfallWidth) {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
}

// parseMinMS reads --min as whole milliseconds: a Go duration (500ms, 2s) or a bare number of
// milliseconds. Empty is no minimum.
func parseMinMS(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	bad := output.New("INVALID_REQUEST", fmt.Sprintf("--min takes a duration such as 500ms or 2s; %q is not one.", v),
		"Pass --min with a number and a unit (ms, s, m), such as --min 500ms.", map[string]any{"min": v})
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, bad
		}
		return int64(math.Ceil(n)), nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, bad
	}
	return int64(math.Ceil(float64(d) / float64(time.Millisecond))), nil
}

// normalizeTraceID checks an id is a trace id (32 hex, written lower case) or a request id
// (a 26 character ULID, written upper case), so a typo is caught before the request.
func normalizeTraceID(id string) (string, error) {
	switch {
	case len(id) == 32 && strings.Trim(strings.ToLower(id), "0123456789abcdef") == "":
		return strings.ToLower(id), nil
	case len(id) == 26 && strings.Trim(strings.ToUpper(id), "0123456789ABCDEFGHJKMNPQRSTVWXYZ") == "":
		return strings.ToUpper(id), nil
	}
	return "", output.New("INVALID_REQUEST", fmt.Sprintf("%q is neither a trace id nor a request id.", id),
		"Pass the 32 hex character trace id from whisk traces, or the 26 character request id from a log line's request_id or the X-Whisk-Request-Id header.", map[string]any{"id": id})
}

// traceRow is one trace as a row of the list.
func traceRow(t api.TraceSummary) []string {
	status := "-"
	if t.StatusCode > 0 {
		status = strconv.Itoa(t.StatusCode)
	}
	return []string{
		t.StartedAt.Local().Format("01-02 15:04:05"),
		formatMS(t.DurationMS),
		status,
		truncate(traceRequest(t), 60),
		strconv.Itoa(t.Spans),
		strconv.Itoa(t.Errors),
		t.RequestID,
	}
}

// traceRequest is how a trace is named in the list: method and path when the entry span is a
// request, otherwise the entry span's name.
func traceRequest(t api.TraceSummary) string {
	switch {
	case t.Method != "" && t.Path != "":
		return t.Method + " " + t.Path
	case t.Path != "":
		return t.Path
	case t.Name != "":
		return t.Name
	}
	return "-"
}

// usageNote is the sentence for a person when the app has reached its daily span limit.
func usageNote(u api.TraceUsage) string {
	if u.Limit <= 0 || u.SpansToday < u.Limit {
		return ""
	}
	return fmt.Sprintf("The app has sent %s spans today, its daily limit of %s; spans past it are dropped until midnight UTC.",
		thousands(int(u.SpansToday)), thousands(int(u.Limit)))
}

// thousands writes n with comma separators.
func thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// formatMS is a duration in milliseconds as a person reads it: 0.42ms, 12.5ms, 340ms, 1.25s.
func formatMS(ms float64) string {
	switch {
	case ms < 0:
		return "0ms"
	case ms < 10:
		return strconv.FormatFloat(roundTo(ms, 2), 'f', -1, 64) + "ms"
	case ms < 100:
		return strconv.FormatFloat(roundTo(ms, 1), 'f', -1, 64) + "ms"
	case ms < 1000:
		return strconv.FormatFloat(math.Round(ms), 'f', -1, 64) + "ms"
	}
	return strconv.FormatFloat(roundTo(ms/1000, 2), 'f', -1, 64) + "s"
}

func roundTo(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// waterfallWidth is how many cells a span's bar is drawn across.
const waterfallWidth = 24

// spanRow is one span placed in the tree: how deep it sits under its root.
type spanRow struct {
	Span  api.Span
	Depth int
}

// spanTree orders spans for the waterfall: each root (no parent, or a parent that is not among
// the spans) followed by its descendants depth first, siblings in the order given (start time).
// A span caught in a parent cycle is reached from no root; it is shown as a root of its own so
// nothing the API returned is hidden.
func spanTree(spans []api.Span) []spanRow {
	known := make(map[string]bool, len(spans))
	for _, sp := range spans {
		known[sp.SpanID] = true
	}
	children := map[string][]int{}
	var roots []int
	for i, sp := range spans {
		if sp.ParentSpanID == "" || sp.ParentSpanID == sp.SpanID || !known[sp.ParentSpanID] {
			roots = append(roots, i)
			continue
		}
		children[sp.ParentSpanID] = append(children[sp.ParentSpanID], i)
	}
	seen := make([]bool, len(spans))
	rows := make([]spanRow, 0, len(spans))
	var visit func(i, depth int)
	visit = func(i, depth int) {
		if seen[i] {
			return
		}
		seen[i] = true
		rows = append(rows, spanRow{Span: spans[i], Depth: depth})
		for _, c := range children[spans[i].SpanID] {
			visit(c, depth+1)
		}
	}
	for _, r := range roots {
		visit(r, 0)
	}
	for i := range spans {
		visit(i, 0)
	}
	return rows
}

// spanBar draws a span to scale across width cells: where it starts and how long it lasts within
// a trace of total milliseconds. Every span gets at least one cell so a short one is still seen.
func spanBar(offsetMS, durationMS, totalMS float64, width int) string {
	if width <= 0 {
		return ""
	}
	start, length := 0, 1
	if totalMS > 0 {
		start = int(math.Floor(offsetMS / totalMS * float64(width)))
		length = int(math.Round(durationMS / totalMS * float64(width)))
	}
	start = max(0, min(start, width-1))
	length = max(1, min(length, width-start))
	return "[" + strings.Repeat(" ", start) + strings.Repeat("=", length) + strings.Repeat(" ", width-start-length) + "]"
}

// traceTotalMS is the scale of the waterfall: the trace's duration, or the end of its last span
// when that is later (a span that outlived the entry span).
func traceTotalMS(t api.Trace) float64 {
	total := t.DurationMS
	for _, sp := range t.Spans {
		total = math.Max(total, sp.OffsetMS+sp.DurationMS)
	}
	return total
}

// spanProblems are the lines under a failed span: its status message and each exception's
// type and message.
func spanProblems(sp api.Span) []string {
	var out []string
	if sp.StatusMessage != "" {
		out = append(out, "error: "+oneLine(sp.StatusMessage))
	}
	for _, ev := range sp.Events {
		if ev.Name != "exception" {
			continue
		}
		msg, _ := ev.Attributes["exception.message"].(string)
		typ, _ := ev.Attributes["exception.type"].(string)
		switch {
		case typ != "" && msg != "":
			out = append(out, "exception: "+oneLine(typ+": "+msg))
		case msg != "" || typ != "":
			out = append(out, "exception: "+oneLine(msg+typ))
		}
	}
	return out
}

// oneLine keeps a message on one line and short enough to read.
func oneLine(s string) string {
	return truncate(strings.Join(strings.Fields(s), " "), 200)
}

// renderTrace is the whole waterfall as lines: a heading, then one line per span with its offset,
// duration, bar, an x when it failed, and its name indented under its parent; a failed span is
// followed by what went wrong. The service is named on a span only when the trace crosses several.
func renderTrace(t api.Trace, width int) []string {
	errs, services := 0, map[string]bool{}
	for _, sp := range t.Spans {
		if sp.Status == "error" {
			errs++
		}
		services[sp.Service] = true
	}
	lines := []string{
		fmt.Sprintf("Trace %s  request %s  %s", t.TraceID, t.RequestID, t.Env),
		fmt.Sprintf("%s  %s  %d spans, %d errors", t.StartedAt.Local().Format("2006-01-02 15:04:05"), formatMS(t.DurationMS), len(t.Spans), errs),
	}
	if len(t.Spans) == 0 {
		return append(lines, "", "No spans of this trace are kept.")
	}
	lines = append(lines, "")
	total := traceTotalMS(t)
	for _, r := range spanTree(t.Spans) {
		sp := r.Span
		mark := " "
		if sp.Status == "error" {
			mark = "x"
		}
		indent := strings.Repeat("  ", r.Depth)
		name := sp.Name
		if len(services) > 1 && sp.Service != "" {
			name += "  (" + sp.Service + ")"
		}
		head := fmt.Sprintf("%9s %9s  %s %s %s", "+"+formatMS(sp.OffsetMS), formatMS(sp.DurationMS), spanBar(sp.OffsetMS, sp.DurationMS, total, width), mark, indent)
		lines = append(lines, head+name)
		if sp.Status == "error" {
			pad := strings.Repeat(" ", len(head)+2)
			for _, p := range spanProblems(sp) {
				lines = append(lines, pad+p)
			}
		}
	}
	if t.Truncated {
		lines = append(lines, "", "The trace has more spans than are shown; the first 2000 by start time are kept.")
	}
	return lines
}
