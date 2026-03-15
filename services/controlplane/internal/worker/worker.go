package worker

import (
	"context"
	"log"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/service"
)

type Worker struct {
	service *service.ControlPlane
	poll    time.Duration
	logger  *log.Logger
}

func New(service *service.ControlPlane, poll time.Duration, logger *log.Logger) *Worker {
	return &Worker{service: service, poll: poll, logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		if processed, err := w.service.ProcessNextRun(ctx); err != nil {
			w.logger.Printf("worker: %v", err)
		} else if !processed {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
