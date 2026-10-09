// Package run is the one way Whisk's Go programs run background work, call other services and
// run commands (ARCHITECTURE.md, "Running work"). Each helper applies the safety rules once, so
// a feature cannot forget them: every run has a deadline, a crash is recovered and reported, a
// background loop restarts with backoff, a call to another service has a deadline and is retried
// only when the caller says it is safe, and a command's whole process group is killed when it
// runs out of time, with its output capped. `make check` refuses raw goroutines, HTTP clients,
// commands and tickers anywhere else.
package run

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Reporter is told about every failure the helpers catch: a run that returned an error, a run
// that panicked, a run that hit its deadline. Each program sets it once, at start, to its log
// and its reviewed error log.
type Reporter func(name string, err error)

var reporter atomic.Pointer[Reporter]

// SetReporter sets where failures go. Until it is called they are dropped.
func SetReporter(r Reporter) { reporter.Store(&r) }

// Report hands a failure to the reporter.
func Report(name string, err error) {
	if err == nil {
		return
	}
	if r := reporter.Load(); r != nil && *r != nil {
		(*r)(name, err)
	}
}

// Panic is a recovered panic, with the stack where it happened.
type Panic struct {
	Value any
	Stack string
}

func (p Panic) Error() string { return fmt.Sprintf("panic: %v", p.Value) }

// Safely runs fn and answers its error, or the panic it raised as a Panic.
func Safely(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Panic{Value: r, Stack: string(debug.Stack())}
		}
	}()
	return fn(ctx)
}

// once runs fn with a deadline and reports what went wrong.
func once(ctx context.Context, name string, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := Safely(ctx, fn)
	if err != nil && context.Cause(ctx) != context.Canceled {
		Report(name, err)
	}
	return err
}

// Option changes how Every runs.
type Option func(*options)

type options struct{ now bool }

// Now runs the first time at once rather than after the first interval.
func Now() Option { return func(o *options) { o.now = true } }

// Every runs fn every interval until ctx ends, each run within timeout. Runs never overlap: one
// that takes longer than the interval delays the next. A run's error or panic is reported and
// the loop carries on. It returns when ctx ends.
func Every(ctx context.Context, name string, every, timeout time.Duration, fn func(context.Context) error, opts ...Option) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.now && ctx.Err() == nil {
		_ = once(ctx, name, timeout, fn)
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = once(ctx, name, timeout, fn)
		}
	}
}

// EveryInBackground is Every on its own goroutine.
func EveryInBackground(ctx context.Context, name string, every, timeout time.Duration, fn func(context.Context) error, opts ...Option) {
	go Every(ctx, name, every, timeout, fn, opts...)
}

// Backoff is the wait before Go starts a failed loop again: 1 s, doubling to a minute.
func Backoff(failures int) time.Duration {
	if failures < 1 {
		return 0
	}
	d := time.Second << min(failures-1, 6)
	return min(d, time.Minute)
}

// healthy is how long a loop must run before its failures are forgotten.
const healthy = time.Minute

// Go runs fn in the background for as long as ctx lasts: a long-running loop such as a stream
// consumer or a queue worker. When fn returns an error or panics it is reported and started
// again after Backoff; when it returns nil it is done.
func Go(ctx context.Context, name string, fn func(context.Context) error) {
	go loop(ctx, name, fn, time.After)
}

func loop(ctx context.Context, name string, fn func(context.Context) error, after func(time.Duration) <-chan time.Time) {
	failures := 0
	for ctx.Err() == nil {
		start := time.Now()
		err := Safely(ctx, fn)
		if err == nil || ctx.Err() != nil {
			return
		}
		Report(name, err)
		if time.Since(start) >= healthy {
			failures = 0
		}
		failures++
		select {
		case <-ctx.Done():
			return
		case <-after(Backoff(failures)):
		}
	}
}

// Detach runs fn once in the background within timeout, on a context that outlives ctx's
// cancellation but keeps its values: an effect that must finish after the request that started
// it has been answered. A failure is reported.
func Detach(ctx context.Context, name string, timeout time.Duration, fn func(context.Context) error) {
	ctx = context.WithoutCancel(ctx)
	go func() { _ = once(ctx, name, timeout, fn) }()
}

// Spawn runs fn once in the background, bounded only by ctx: work that lasts as long as
// something else does, such as one connection's session. A crash is recovered and reported and
// not started again, since what it served is gone.
func Spawn(ctx context.Context, name string, fn func(context.Context) error) {
	go func() {
		if err := Safely(ctx, fn); err != nil && ctx.Err() == nil {
			Report(name, err)
		}
	}()
}

// All runs each fn at once, each on its own goroutine, and waits for them all. It answers the
// first error (a panic counts as one); the others still run to their end.
func All(ctx context.Context, fns ...func(context.Context) error) error {
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Safely(ctx, fn)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Each runs fn for every item, at most limit at once, and waits for them all. It answers the
// first error by item order; the others still run.
func Each[T any](ctx context.Context, limit int, items []T, fn func(context.Context, T) error) error {
	if limit < 1 {
		limit = 1
	}
	errs := make([]error, len(items))
	slots := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, item := range items {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			errs[i] = Safely(ctx, func(ctx context.Context) error { return fn(ctx, item) })
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Ticker is a ticker for a loop that does more than tick and is itself already run by Go, Every
// or a request: a stream's heartbeat beside its messages, a select over several channels. Plain
// periodic work is Every.
func Ticker(every time.Duration) *time.Ticker { return time.NewTicker(every) }
