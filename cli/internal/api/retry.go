package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retrying (CLI.md §1.4). A call that is safe to repeat (a GET or HEAD, or a mutating call that
// carries an Idempotency-Key the platform deduplicates on) is tried again when the platform
// could not be reached or a proxy in front of it answered 502, 503 or 504. Any other answer,
// 500 included, is the platform's verdict and is returned as it is.
const (
	maxRetries     = 3
	retryBase      = 500 * time.Millisecond
	maxRetryAfter  = 10 * time.Second
	streamIdle     = 45 * time.Second
	streamFailures = 5
	streamMaxWait  = 8 * time.Second
)

// transientStatus is an answer that says nothing about the request: a gateway or the platform
// itself was briefly unavailable.
func transientStatus(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryWait decides whether a failed attempt is made again and after how long. attempt is the
// number of attempts made so far; status is the answer's status, or 0 when the platform could
// not be reached or its answer was cut off. jitter, in [0,1), spreads clients that failed
// together: the wait is 0.5 s, 1 s, 2 s, each ±25 %. A Retry-After the answer carries replaces
// the backoff, capped at 10 s.
func retryWait(attempt int, safe bool, status int, retryAfter string, jitter float64, now time.Time) (time.Duration, bool) {
	if !safe || attempt > maxRetries || (status != 0 && !transientStatus(status)) {
		return 0, false
	}
	if d, ok := parseRetryAfter(retryAfter, now); ok {
		return min(d, maxRetryAfter), true
	}
	base := retryBase << (attempt - 1)
	return time.Duration(float64(base) * (0.75 + 0.5*jitter)), true
}

// parseRetryAfter reads Retry-After as seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0, false
		}
		return time.Duration(min(n, int(maxRetryAfter/time.Second)+1)) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

// streamStep decides what follows a stream that ended while the caller still wanted it: whiskd
// ends every stream when it restarts, and a stream that sent nothing for streamIdle is treated
// as dropped. failures counts the connections in a row that delivered nothing; one that
// delivered data starts the count again. A refusal (any error object other than a transient
// status) ends the follow at once, as does the fifth failure in a row. The wait doubles from
// 0.5 s up to 8 s, ±25 %.
func streamStep(failures int, gotData bool, err error, jitter float64) (next int, wait time.Duration, again bool) {
	var e *Error
	if errors.As(err, &e) && !transientStatus(e.Status) {
		return failures, 0, false
	}
	var u *Unavailable
	if err != nil && e == nil && !errors.As(err, &u) {
		return failures, 0, false
	}
	if gotData {
		failures = 0
	}
	next = failures + 1
	if next >= streamFailures {
		return next, 0, false
	}
	base := min(retryBase<<(next-1), streamMaxWait)
	return next, time.Duration(float64(base) * (0.75 + 0.5*jitter)), true
}

// errStreamIdle is why a stream is dropped when nothing, not even a keepalive, arrived.
var errStreamIdle = errors.New("the stream sent nothing for too long")

// errStreamEnded is what a follow gives up with when every stream ended without an error.
var errStreamEnded = errors.New("the stream kept ending without sending anything")

// stream opens path as server-sent events and hands the body to read. The connection is
// abandoned when no byte arrives for the idle deadline (45 s; the platform sends a keepalive
// every 15 to 20), which covers waiting for the answer's headers too, and that is returned as
// *Unavailable so the caller reconnects.
func (c *Client) stream(ctx context.Context, path string, read func(io.Reader) error) error {
	idle := c.streamIdle
	if idle <= 0 {
		idle = streamIdle
	}
	connCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := time.AfterFunc(idle, func() { cancel(errStreamIdle) })
	defer timer.Stop()
	resp, err := c.open(connCtx, path, "text/event-stream")
	if err == nil {
		defer resp.Body.Close()
		err = read(&idleReader{r: resp.Body, timer: timer, idle: idle})
	}
	if err != nil && ctx.Err() == nil && errors.Is(context.Cause(connCtx), errStreamIdle) {
		return &Unavailable{Err: errStreamIdle}
	}
	return err
}

// idleReader pushes the idle deadline back whenever bytes arrive.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(i.idle)
	}
	return n, err
}

// pause waits d or until the context ends.
func (c *Client) pause(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// tailCursor is where a followed log stream got to: the time of the newest line delivered and
// the lines delivered at exactly that time. A reconnect asks for lines since that time, which
// the platform answers inclusively, so the lines at the boundary come again and are dropped.
type tailCursor struct {
	at   time.Time
	seen map[string]bool
}

func lineKey(l LogLine) string {
	return l.At.UTC().Format(time.RFC3339Nano) + "\x00" + l.Stream + "\x00" + l.Host + "\x00" + l.Unit + "\x00" + l.Line
}

// advance returns the lines of batch not yet delivered and the cursor after them.
func (c tailCursor) advance(batch []LogLine) ([]LogLine, tailCursor) {
	at := c.at
	seen := make(map[string]bool, len(c.seen))
	for k := range c.seen {
		seen[k] = true
	}
	fresh := make([]LogLine, 0, len(batch))
	for _, l := range batch {
		key := lineKey(l)
		if l.At.Equal(at) && seen[key] {
			continue
		}
		if l.At.After(at) {
			at, seen = l.At, map[string]bool{}
		}
		if l.At.Equal(at) {
			seen[key] = true
		}
		fresh = append(fresh, l)
	}
	return fresh, tailCursor{at: at, seen: seen}
}

// since is the query's since for the next connection: the cursor's time once a line arrived,
// else what the caller asked for.
func (c tailCursor) since(asked string) string {
	if c.at.IsZero() {
		return asked
	}
	return c.at.UTC().Format(time.RFC3339Nano)
}

// followLines follows a log stream until the context ends, reconnecting from where it got to
// when the stream ends or drops (streamStep). path builds the stream's path for a since.
func (c *Client) followLines(ctx context.Context, path func(since string) string, since string, fn func([]LogLine) error) error {
	cur := tailCursor{}
	failures := 0
	for {
		got := false
		var fnErr error
		err := c.stream(ctx, path(cur.since(since)), func(r io.Reader) error {
			return readLineEvents(r, func(batch []LogLine) error {
				got = true
				fresh, next := cur.advance(batch)
				cur = next
				if len(fresh) == 0 {
					return nil
				}
				fnErr = fn(fresh)
				return fnErr
			})
		})
		if ctx.Err() != nil {
			return nil
		}
		if fnErr != nil {
			return fnErr
		}
		next, wait, again := streamStep(failures, got, err, c.jitter())
		if !again {
			if err == nil {
				return &Unavailable{Err: errStreamEnded}
			}
			return err
		}
		failures = next
		if c.pause(ctx, wait) != nil {
			return nil
		}
	}
}
