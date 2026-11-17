package whisk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/whisk-run/contract/run"

	"github.com/whisk-run/cli/internal/output"
)

// whisk call --batch sends many requests in one run: the address is found once, and every
// request goes over the same connection, so an import of hundreds of records pays for one
// connection rather than one per record.

// callBatchParallelMax is the most requests --batch sends at once.
const callBatchParallelMax = 8

// callBatchLineMax is the longest request line --batch reads: the edge's limit on a body.
const callBatchLineMax = 32 << 20

// callBatchRequest is one line of a batch file.
type callBatchRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Data    json.RawMessage   `json:"data,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// callBatchItem is a line read and checked: what to send, or why it cannot be.
type callBatchItem struct {
	Line   int
	Method string
	Path   string
	Header http.Header
	Body   []byte
	Err    error
}

// parseCallBatchLine reads one line of a batch file as a request, checking it as whisk call
// checks its arguments. Pure.
func parseCallBatchLine(n int, raw []byte) callBatchItem {
	it := callBatchItem{Line: n}
	var r callBatchRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		it.Err = output.New("INVALID_REQUEST", fmt.Sprintf("Line %d is not a request: %s.", n, err.Error()),
			`Write each line as {"method": "POST", "path": "/api/...", "data": {...}, "headers": {"Name": "value"}}.`, map[string]any{"line": n})
		return it
	}
	it.Method, it.Path = strings.ToUpper(r.Method), r.Path
	if !callMethods[it.Method] {
		it.Err = output.New("INVALID_REQUEST", fmt.Sprintf("Line %d: %q is not a method whisk call sends.", n, r.Method), "Use GET, POST, PUT, PATCH, DELETE, HEAD or OPTIONS.", map[string]any{"line": n, "method": r.Method})
		return it
	}
	names := make([]string, 0, len(r.Headers))
	for name := range r.Headers {
		names = append(names, name)
	}
	slices.Sort(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, name+": "+r.Headers[name])
	}
	header, err := callHeaders(lines)
	if err != nil {
		it.Err = err
		return it
	}
	it.Header = header
	if len(r.Data) == 0 || string(r.Data) == "null" {
		return it
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json")
	}
	var text string
	if !isJSONType(header.Get("Content-Type")) && json.Unmarshal(r.Data, &text) == nil {
		it.Body = []byte(text)
		return it
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, r.Data); err != nil {
		it.Err = output.New("INVALID_REQUEST", fmt.Sprintf("Line %d's data is not valid JSON.", n), "Fix the JSON on that line.", map[string]any{"line": n})
		return it
	}
	it.Body = compact.Bytes()
	return it
}

// callBatchResult is what one request came to, as the JSON line printed for it. Pure.
func callBatchResult(it callBatchItem, a callAnswer, err error) map[string]any {
	out := map[string]any{"line": it.Line, "method": it.Method, "path": it.Path}
	if err == nil {
		out["status"] = a.Status
		out["body"] = a.bodyValue()
		if a.Truncated {
			out["truncated"] = true
		}
		if a.RequestID != "" {
			out["request_id"] = a.RequestID
		}
		return out
	}
	var e *output.Error
	if !errors.As(err, &e) {
		e = &output.Error{Code: "CLI_ERROR", Message: err.Error(), Fix: "Read the message; if it names a platform problem, retry with backoff."}
	}
	body := map[string]any{"code": e.Code, "message": e.Message, "fix": e.Fix}
	if e.Docs != "" {
		body["docs"] = e.Docs
	}
	if e.Details != nil {
		body["details"] = e.Details
	}
	out["error"] = body
	return out
}

// readCallBatch reads a batch file, or stdin for -, into its requests. Blank lines are skipped
// but counted, so line numbers are the file's own.
func (s *session) readCallBatch(from string) ([]callBatchItem, error) {
	var r io.Reader
	if from == "-" {
		r = s.env.Stdin
	} else {
		path := from
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.env.Dir, path)
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, output.New("INVALID_REQUEST", "The batch file could not be read: "+err.Error(), "Pass the path of a file of requests, relative to the working directory, or - for stdin.", map[string]any{"batch": from})
		}
		defer f.Close()
		r = f
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), callBatchLineMax)
	var items []callBatchItem
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		items = append(items, parseCallBatchLine(n, line))
	}
	if err := sc.Err(); err != nil {
		return nil, output.New("INVALID_REQUEST", "The batch could not be read: "+err.Error(), fmt.Sprintf("Keep each request on one line of at most %d MB.", callBatchLineMax>>20), map[string]any{"batch": from})
	}
	if len(items) == 0 {
		return nil, output.New("INVALID_REQUEST", "The batch has no requests.", `Write one request per line, such as {"method": "GET", "path": "/api/me"}.`, map[string]any{"batch": from})
	}
	return items, nil
}

// callBatch sends every request of a batch, up to parallel at once, printing one JSON line per
// request in the batch's order as soon as it and every request before it have answered.
func (s *session) callBatch(from, environment string, timeout time.Duration, parallel int) error {
	if timeout <= 0 || timeout > 10*time.Minute {
		return output.New("INVALID_REQUEST", "--timeout must be more than 0 and at most 10m.", "Pass a duration such as 60s.", map[string]any{"timeout": timeout.String()})
	}
	if parallel < 1 || parallel > callBatchParallelMax {
		return output.New("INVALID_REQUEST", fmt.Sprintf("--parallel must be 1 to %d.", callBatchParallelMax), "Pass --parallel 4, or leave it at 1 when later requests need earlier ones made first.", map[string]any{"parallel": parallel})
	}
	items, err := s.readCallBatch(from)
	if err != nil {
		return err
	}
	token, key, err := s.callCredential()
	if err != nil {
		return err
	}
	client, org, app, err := s.appClient()
	if err != nil {
		return err
	}
	addr := s.callResolver(client, org, app, environment)
	if _, _, _, err := addr.address(); err != nil {
		return err
	}
	hc := callHTTPClient(timeout)
	started := time.Now()

	results := make([]map[string]any, len(items))
	done := make([]chan struct{}, len(items))
	for i := range done {
		done[i] = make(chan struct{})
	}
	indexes := make([]int, len(items))
	for i := range indexes {
		indexes[i] = i
	}
	finished := make(chan struct{})
	run.Spawn(s.ctx, "whisk call --batch", func(ctx context.Context) error {
		defer close(finished)
		return run.Each(ctx, parallel, indexes, func(_ context.Context, i int) error {
			defer close(done[i])
			it := items[i]
			if it.Err != nil {
				results[i] = callBatchResult(it, callAnswer{}, it.Err)
				return nil
			}
			u, _, a, err := s.callOnce(addr, hc, it.Method, it.Path, it.Header, it.Body, token, key, timeout)
			if err == nil {
				err = callOutcome(it.Method, u, a)
			}
			results[i] = callBatchResult(it, a, err)
			return nil
		})
	})

	enc := json.NewEncoder(s.env.Stdout)
	enc.SetEscapeHTML(false)
	var failed []int
	for i := range items {
		<-done[i]
		_ = enc.Encode(results[i])
		if _, bad := results[i]["error"]; bad {
			failed = append(failed, items[i].Line)
		}
	}
	<-finished

	took := time.Since(started).Round(time.Millisecond)
	if !s.printer.Quiet {
		fmt.Fprintf(s.env.Stderr, "%d requests to %s/%s in %s: %d answered 2xx, %d failed\n", len(items), org, app, took, len(items)-len(failed), len(failed))
	}
	if len(failed) > 0 {
		shown := failed[:min(len(failed), 50)]
		return &output.Error{Code: "CALL_FAILED", Message: fmt.Sprintf("%d of %d requests in the batch failed.", len(failed), len(items)),
			Fix:     "Read each failed line's error above, correct those requests and send only them again; the others were made.",
			Details: map[string]any{"requests": len(items), "failed": len(failed), "lines": shown}}
	}
	if s.printer.JSON {
		s.printer.Result(map[string]any{"org": org, "app": app, "requests": len(items), "failed": 0, "ms": took.Milliseconds()}, nil)
	}
	return nil
}
