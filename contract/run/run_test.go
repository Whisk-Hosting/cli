package run

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type reports struct {
	mu   sync.Mutex
	errs map[string][]error
}

func capture(t *testing.T) *reports {
	r := &reports{errs: map[string][]error{}}
	SetReporter(func(name string, err error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.errs[name] = append(r.errs[name], err)
	})
	t.Cleanup(func() { SetReporter(nil) })
	return r
}

func (r *reports) of(name string) []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.errs[name]...)
}

func TestSafelyTurnsAPanicIntoAnError(t *testing.T) {
	err := Safely(context.Background(), func(context.Context) error { panic("boom") })
	var p Panic
	if !errors.As(err, &p) || p.Value != "boom" || !strings.Contains(p.Stack, "run_test.go") {
		t.Fatalf("Safely = %v", err)
	}
}

func TestEveryRecoversReportsAndCarriesOn(t *testing.T) {
	r := capture(t)
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		Every(ctx, "loop", time.Millisecond, time.Second, func(context.Context) error {
			if runs.Add(1) == 1 {
				panic("first run")
			}
			if runs.Load() >= 3 {
				cancel()
			}
			return errors.New("second run")
		}, Now())
	}()
	<-done
	if runs.Load() < 3 {
		t.Fatalf("runs = %d: the loop stopped after a failure", runs.Load())
	}
	if got := r.of("loop"); len(got) < 2 {
		t.Fatalf("reported %v", got)
	}
}

func TestEveryGivesEachRunADeadline(t *testing.T) {
	r := capture(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		Every(ctx, "slow", time.Hour, 10*time.Millisecond, func(ctx context.Context) error {
			<-ctx.Done()
			cancel()
			return ctx.Err()
		}, Now())
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a run outlived its deadline")
	}
	if got := r.of("slow"); len(got) != 1 || !errors.Is(got[0], context.DeadlineExceeded) {
		t.Fatalf("reported %v", got)
	}
}

func TestBackoff(t *testing.T) {
	for _, tc := range []struct {
		failures int
		want     time.Duration
	}{{0, 0}, {1, time.Second}, {2, 2 * time.Second}, {6, 32 * time.Second}, {7, time.Minute}, {50, time.Minute}} {
		if got := Backoff(tc.failures); got != tc.want {
			t.Errorf("Backoff(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestGoRestartsAFailedLoopUntilItIsDone(t *testing.T) {
	r := capture(t)
	var waits []time.Duration
	instant := func(d time.Duration) <-chan time.Time {
		waits = append(waits, d)
		c := make(chan time.Time, 1)
		c <- time.Now()
		return c
	}
	runs := 0
	loop(context.Background(), "worker", func(context.Context) error {
		runs++
		switch runs {
		case 1:
			panic("crash")
		case 2:
			return errors.New("lost the connection")
		}
		return nil
	}, instant)
	if runs != 3 || len(r.of("worker")) != 2 {
		t.Fatalf("runs %d, reported %v", runs, r.of("worker"))
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Fatalf("waited %v", waits)
	}
}

func TestDetachOutlivesItsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	Detach(ctx, "after", time.Second, func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		got <- ctx.Err()
		return nil
	})
	cancel()
	if err := <-got; err != nil {
		t.Fatalf("the detached run was cancelled with its request: %v", err)
	}
}

func TestAllAndEachWaitAndAnswerTheFirstError(t *testing.T) {
	var ran atomic.Int32
	err := All(context.Background(),
		func(context.Context) error { ran.Add(1); return nil },
		func(context.Context) error { ran.Add(1); panic("x") },
		func(context.Context) error { ran.Add(1); return errors.New("y") })
	var p Panic
	if ran.Load() != 3 || !errors.As(err, &p) {
		t.Fatalf("ran %d, err %v", ran.Load(), err)
	}
	var most, now atomic.Int32
	err = Each(context.Background(), 2, []int{1, 2, 3, 4, 5}, func(_ context.Context, n int) error {
		if c := now.Add(1); c > most.Load() {
			most.Store(c)
		}
		time.Sleep(5 * time.Millisecond)
		now.Add(-1)
		if n == 4 {
			return errors.New("four")
		}
		return nil
	})
	if err == nil || err.Error() != "four" || most.Load() > 2 {
		t.Fatalf("err %v, at most %d at once", err, most.Load())
	}
}

func TestRetryOnlyWhatIsMarked(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, func(context.Context) error {
		calls++
		return errors.New("not safe to repeat")
	})
	if err == nil || calls != 1 {
		t.Fatalf("an unmarked failure was retried: %d calls", calls)
	}
	calls = 0
	err = Retry(context.Background(), 2, func(context.Context) error {
		calls++
		return Retryable(errors.New("503"))
	})
	if !IsRetryable(err) || calls != 2 {
		t.Fatalf("calls %d err %v", calls, err)
	}
	calls = 0
	if err := Retry(context.Background(), 3, func(context.Context) error {
		calls++
		if calls == 1 {
			return Retryable(errors.New("reset"))
		}
		return nil
	}); err != nil || calls != 2 {
		t.Fatalf("calls %d err %v", calls, err)
	}
}

func TestIdleEndsAQuietStream(t *testing.T) {
	ctx, touch, stop := Idle(context.Background(), 30*time.Millisecond)
	defer stop()
	for range 3 {
		time.Sleep(10 * time.Millisecond)
		touch()
	}
	if ctx.Err() != nil {
		t.Fatal("ended while messages kept coming")
	}
	<-ctx.Done()
	if !errors.Is(context.Cause(ctx), ErrQuiet) {
		t.Fatalf("cause %v", context.Cause(ctx))
	}
}

func TestSpawnReportsACrashOnce(t *testing.T) {
	r := capture(t)
	done := make(chan struct{})
	Spawn(context.Background(), "session", func(context.Context) error {
		defer close(done)
		panic("lost")
	})
	<-done
	time.Sleep(20 * time.Millisecond)
	if got := r.of("session"); len(got) != 1 {
		t.Fatalf("reported %v", got)
	}
}
