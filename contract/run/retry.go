package run

import (
	"context"
	"errors"
	"time"
)

// retryable marks an error a caller has judged safe to try again.
type retryable struct{ err error }

func (r retryable) Error() string { return r.err.Error() }
func (r retryable) Unwrap() error { return r.err }

// Retryable marks err as safe to try again: the call is idempotent and failed in a way that may
// pass (a connection reset, a 502, 503 or 504, a 429).
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return retryable{err}
}

// IsRetryable reports whether err was marked Retryable.
func IsRetryable(err error) bool {
	var r retryable
	return errors.As(err, &r)
}

// Retry calls fn up to attempts times while it fails with a Retryable error, waiting Backoff
// between tries. Only an idempotent call may be retried; the caller says which by marking.
func Retry(ctx context.Context, attempts int, fn func(context.Context) error) error {
	var err error
	for try := 1; try <= attempts; try++ {
		if err = Safely(ctx, fn); err == nil || !IsRetryable(err) || try == attempts {
			return err
		}
		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-time.After(Backoff(try)):
		}
	}
	return err
}

// Idle watches a stream: it answers a context that ends when touch has not been called for
// quiet, and touch, to call on every message or heartbeat. Read the stream under that context
// and a stream that silently stops is ended rather than waited on for ever.
func Idle(ctx context.Context, quiet time.Duration) (context.Context, func(), context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(quiet, func() { cancel(ErrQuiet) })
	touch := func() { timer.Reset(quiet) }
	return ctx, touch, func() { timer.Stop(); cancel(context.Canceled) }
}

// ErrQuiet is the cause of a stream ended by Idle.
var ErrQuiet = errors.New("the stream went quiet")
