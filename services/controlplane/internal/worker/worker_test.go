package worker_test

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/worker"
)

type fakeProcessor struct {
	mu sync.Mutex
	// fn receives the 1-based call number and decides the result.
	fn    func(call int) (bool, error)
	calls int
}

func (f *fakeProcessor) ProcessNextRun(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.fn(f.calls)
}

func (f *fakeProcessor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// A persistent claim error (e.g. the database being down) must not spin the
// loop hot: the worker has to wait one poll interval between attempts.
func TestRunWaitsBetweenPollsOnPersistentError(t *testing.T) {
	proc := &fakeProcessor{fn: func(int) (bool, error) {
		return false, errors.New("db down")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()

	err := worker.New(proc, 50*time.Millisecond, discardLogger()).Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	// 180ms at a 50ms poll allows the immediate attempt plus ~3 ticks. A hot
	// loop would rack up thousands of calls.
	if calls := proc.callCount(); calls < 2 || calls > 6 {
		t.Fatalf("expected throttled retries (2..6 calls), got %d", calls)
	}
}

// Queued work is drained back-to-back without waiting for the ticker.
func TestRunDrainsQueueWithoutWaiting(t *testing.T) {
	proc := &fakeProcessor{fn: func(call int) (bool, error) {
		return call <= 3, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// A one-hour poll proves draining never touches the ticker.
	err := worker.New(proc, time.Hour, discardLogger()).Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if calls := proc.callCount(); calls != 4 {
		t.Fatalf("expected 3 processed runs + 1 idle poll, got %d calls", calls)
	}
}

// Cancellation interrupts the idle wait promptly instead of blocking until
// the next tick.
func TestRunStopsPromptlyOnCancelWhileIdle(t *testing.T) {
	proc := &fakeProcessor{fn: func(int) (bool, error) {
		return false, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- worker.New(proc, time.Hour, discardLogger()).Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop after cancellation")
	}
}
