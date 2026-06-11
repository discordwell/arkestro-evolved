package worker

import (
	"context"
	"log"
	"time"
)

// RunProcessor is the slice of the control-plane service the worker needs;
// *service.ControlPlane satisfies it.
type RunProcessor interface {
	ProcessNextRun(ctx context.Context) (bool, error)
}

type Worker struct {
	processor RunProcessor
	poll      time.Duration
	logger    *log.Logger
}

func New(processor RunProcessor, poll time.Duration, logger *log.Logger) *Worker {
	if poll <= 0 {
		poll = time.Second
	}
	return &Worker{processor: processor, poll: poll, logger: logger}
}

// Run processes queued runs until ctx is cancelled. The queue is drained
// without waiting while claims succeed; after an idle poll or an error the
// worker waits for the next tick, so a persistent failure (e.g. the database
// being unreachable) cannot spin the loop hot.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		processed, err := w.processor.ProcessNextRun(ctx)
		if err != nil && ctx.Err() == nil {
			w.logger.Printf("worker: %v", err)
		}
		if err == nil && processed {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
