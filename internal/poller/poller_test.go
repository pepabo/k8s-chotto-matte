package poller_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pepabo/k8s-chotto-matte/internal/poller"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fakeConnector implements connector.Connector by delegating to a
// caller-supplied function.
type fakeConnector struct {
	check func(ctx context.Context) (bool, error)
}

func (f *fakeConnector) Check(ctx context.Context) (bool, error) {
	return f.check(ctx)
}

func alwaysPasses(context.Context) (bool, error) {
	return true, nil
}

var errFakeCheck = errors.New("fake check failure")

func alwaysErrors(context.Context) (bool, error) {
	return false, errFakeCheck
}

func TestRun_ReturnsImmediatelyWhenThresholdIsOne(t *testing.T) {
	t.Parallel()

	conn := &fakeConnector{check: alwaysPasses}
	cfg := poller.Config{Name: "t", Interval: time.Hour, SuccessThreshold: 1, Timeout: time.Second, FailOpen: false}

	err := poller.Run(t.Context(), conn, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRun_WaitsForConsecutiveSuccesses(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	conn := &fakeConnector{check: func(context.Context) (bool, error) {
		calls.Add(1)

		return true, nil
	}}
	cfg := poller.Config{Name: "t", Interval: time.Millisecond, SuccessThreshold: 3, Timeout: time.Second, FailOpen: false}

	err := poller.Run(t.Context(), conn, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestRun_ResetsConsecutiveCountOnFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	conn := &fakeConnector{check: func(context.Context) (bool, error) {
		n := calls.Add(1)
		// Fail once on the 2nd call so the streak must restart.
		if n == 2 {
			return false, nil
		}

		return true, nil
	}}
	cfg := poller.Config{Name: "t", Interval: time.Millisecond, SuccessThreshold: 2, Timeout: time.Second, FailOpen: false}

	err := poller.Run(t.Context(), conn, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Call 1: pass (streak=1). Call 2: fail (streak=0). Calls 3,4: pass,pass (streak=2) -> done.
	if got := calls.Load(); got != 4 {
		t.Errorf("calls = %d, want 4", got)
	}
}

func TestRun_FailOpenCountsErrorAsPass(t *testing.T) {
	t.Parallel()

	conn := &fakeConnector{check: alwaysErrors}
	cfg := poller.Config{Name: "t", Interval: time.Millisecond, SuccessThreshold: 2, Timeout: time.Second, FailOpen: true}

	err := poller.Run(t.Context(), conn, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRun_FailClosedNeverPassesOnError(t *testing.T) {
	t.Parallel()

	conn := &fakeConnector{check: alwaysErrors}
	cfg := poller.Config{Name: "t", Interval: time.Millisecond, SuccessThreshold: 1, Timeout: time.Second, FailOpen: false}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := poller.Run(ctx, conn, cfg, newDiscardLogger())
	if err == nil {
		t.Fatalf("Run() error = nil, want error")
	}

	if !errors.Is(err, poller.ErrCanceled) {
		t.Errorf("Run() error = %v, want wrapping %v", err, poller.ErrCanceled)
	}
}

func TestRun_ChecksAreBoundedByTimeout(t *testing.T) {
	t.Parallel()

	conn := &fakeConnector{check: func(ctx context.Context) (bool, error) {
		<-ctx.Done()

		return false, ctx.Err()
	}}
	cfg := poller.Config{
		Name:             "t",
		Interval:         time.Millisecond,
		SuccessThreshold: 1,
		Timeout:          10 * time.Millisecond,
		FailOpen:         true,
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	start := time.Now()

	err := poller.Run(ctx, conn, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Run() took %v, want it bounded by the per-check timeout", elapsed)
	}
}
