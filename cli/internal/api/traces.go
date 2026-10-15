package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/whisk-run/contract/apitypes"
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

func (q TraceQuery) values() url.Values {
	v := url.Values{}
	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	set("env", q.Env)
	set("since", q.Since)
	set("until", q.Until)
	set("q", q.Q)
	set("sort", q.Sort)
	if q.MinMS > 0 {
		v.Set("min_ms", fmt.Sprint(q.MinMS))
	}
	if q.Limit > 0 {
		v.Set("limit", fmt.Sprint(q.Limit))
	}
	return v
}

// Traces lists the app's traces in a window.
func (c *Client) Traces(ctx context.Context, org, app string, q TraceQuery) (TraceList, error) {
	var out TraceList
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/traces?"+q.values().Encode(), nil, &out)
}

// Trace reads one trace by its 32 hex trace id or its 26 character request id.
func (c *Client) Trace(ctx context.Context, org, app, id string) (Trace, error) {
	var out Trace
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/traces/"+pathSeg(id), nil, &out)
}
