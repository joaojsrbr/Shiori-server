package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// Handler processes a specific type of job.
type Handler func(ctx context.Context, job *queue.Job) error

// Pool manages background job processing.
type Pool struct {
	q        queue.Provider
	logger   *slog.Logger
	handlers map[string]Handler
	types    []string

	// Concurrency settings
	concurrency int
	wg          sync.WaitGroup
}

// New Creates a new worker pool.
func New(q queue.Provider, logger *slog.Logger, concurrency int) *Pool {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Pool{
		q:           q,
		logger:      logger,
		handlers:    make(map[string]Handler),
		concurrency: concurrency,
	}
}

// Register maps a job type to its handler.
func (p *Pool) Register(jobType string, h Handler) {
	p.handlers[jobType] = h
	p.types = append(p.types, jobType)
}

// Start launches the worker goroutines. It blocks until the context is canceled.
func (p *Pool) Start(ctx context.Context) {
	if len(p.handlers) == 0 {
		p.logger.Warn("starting worker pool with no registered handlers")
		return
	}

	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}

	// Wait for all workers to finish when ctx is canceled
	<-ctx.Done()
	p.logger.Info("shutting down worker pool...")
	p.wg.Wait()
	p.logger.Info("worker pool stopped")
}

func (p *Pool) workerLoop(ctx context.Context, workerID int) {
	defer p.wg.Done()

	p.logger.Debug("worker started", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Attempt to dequeue
			job, err := p.q.Dequeue(ctx, p.types)
			if err != nil {
				if errors.Is(err, queue.ErrNoJobs) || errors.Is(err, queue.ErrConflict) {
					// Sleep briefly before polling again
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
						return
					}
					continue
				}

				// Avoid logging scary errors if we are shutting down
				if ctx.Err() != nil {
					return
				}

				// Other errors (e.g. DB connection) might require logging and backing off
				p.logger.Error("failed to dequeue job", "error", err, "worker_id", workerID)
				select {
				case <-time.After(5 * time.Second):
				case <-ctx.Done():
					return
				}
				continue
			}

			// We have a job. Process it.
			p.processJob(ctx, job, workerID)
		}
	}
}

func (p *Pool) processJob(ctx context.Context, job *queue.Job, workerID int) {
	log := p.logger.With("job_id", job.ID, "job_type", job.Type, "worker_id", workerID)
	log.Info("processing job")

	handler, ok := p.handlers[job.Type]
	if !ok {
		log.Error("no handler registered for job type")
		// Use a detached context for Ack/Nack to guarantee execution even if shutting down
		_ = p.q.Nack(context.Background(), job.ID, "no handler registered")
		return
	}

	jobCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				current, err := p.q.Status(context.Background(), job.ID)
				if err == nil && current.Status == queue.StatusCancelled {
					cancel()
					return
				}
				_ = p.q.Heartbeat(context.Background(), job.ID)
			}
		}
	}()
	err := handler(jobCtx, job)
	close(done)
	cancel()
	current, statusErr := p.q.Status(context.Background(), job.ID)
	if statusErr == nil && current.Status == queue.StatusCancelled {
		log.Info("job cancelled")
		return
	}
	if err != nil {
		log.Error("job failed", "error", err)
		_ = p.q.Nack(context.Background(), job.ID, err.Error())
		return
	}

	log.Info("job succeeded")
	_ = p.q.Ack(context.Background(), job.ID)
}
