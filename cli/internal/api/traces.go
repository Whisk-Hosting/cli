package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// ---- traces (CLI.md §5.5) -----------------------------------------------------------------------

// TraceSummary is one trace in a list: its entry span and how big it is.
type TraceSummary = apitypes.TraceSummary

// TraceUsage is how many spans the app has sent today against its daily limit.
type TraceUsage = apitypes.TraceUsage

// TraceList is a page of traces with the retention and today's usage.
type TraceList = apitypes.TraceList

// SpanEvent is something a span recorded at a moment, such as an exception.
type SpanEvent = apitypes.TraceEvent

// Span is one operation of a trace. OffsetMS is from the trace's start.
type Span = apitypes.TraceSpan

// Trace is one trace with its spans, ordered by start time.
type Trace = apitypes.TraceDetail

// TraceQuery is what the caller is asking for. Sort is recent or slowest.
type TraceQuery struct {
	Env   string
	Since string
	Until string
	Q     string
	MinMS int64
	Sort  string
	Limit int
}

// Traces lists the app's traces in a window.
func (c *Client) Traces(ctx context.Context, org, app string, q TraceQuery) (TraceList, error) {
	return Send(ctx, c, endpoints.ListTraces, endpoints.ListTracesQuery{Env: q.Env, Since: q.Since, Until: q.Until, Q: q.Q,
		MinMS: int(max(q.MinMS, 0)), Sort: q.Sort, Limit: max(q.Limit, 0)}, org, app)
}
