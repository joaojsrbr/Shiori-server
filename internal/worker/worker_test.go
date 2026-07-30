package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/joaojsr/shiori-server/internal/worker"
)

// MockProvider mocks the queue.Provider for testing
type MockProvider struct {
	DequeueFunc func(ctx context.Context, types []string) (*queue.Job, error)
	AckFunc     func(ctx context.Context, jobID string) error
	NackFunc    func(ctx context.Context, jobID string, reason string) error

	mu    sync.Mutex
	Acks  []string
	Nacks []string
}

func (m *MockProvider) Enqueue(ctx context.Context, job *queue.Job) error { return nil }
func (m *MockProvider) Dequeue(ctx context.Context, types []string) (*queue.Job, error) {
	if m.DequeueFunc != nil {
		return m.DequeueFunc(ctx, types)
	}
	return nil, queue.ErrNoJobs
}
func (m *MockProvider) Ack(ctx context.Context, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Acks = append(m.Acks, jobID)
	if m.AckFunc != nil {
		return m.AckFunc(ctx, jobID)
	}
	return nil
}
func (m *MockProvider) Nack(ctx context.Context, jobID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nacks = append(m.Nacks, jobID)
	if m.NackFunc != nil {
		return m.NackFunc(ctx, jobID, reason)
	}
	return nil
}
func (m *MockProvider) Heartbeat(ctx context.Context, jobID string) error { return nil }
func (m *MockProvider) Cancel(ctx context.Context, jobID string) error    { return nil }
func (m *MockProvider) Status(ctx context.Context, jobID string) (*queue.Job, error) {
	return nil, nil
}

func TestPool_ProcessJobs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mockQueue := &MockProvider{}

	pool := worker.New(mockQueue, logger, 1)

	// We'll queue exactly two jobs: one that succeeds, one that fails. Then we'll return ErrNoJobs.
	var dequeueCount int
	var mu sync.Mutex

	mockQueue.DequeueFunc = func(ctx context.Context, types []string) (*queue.Job, error) {
		mu.Lock()
		defer mu.Unlock()
		dequeueCount++

		if dequeueCount == 1 {
			return &queue.Job{ID: "job-1", Type: "test_ok"}, nil
		}
		if dequeueCount == 2 {
			return &queue.Job{ID: "job-2", Type: "test_fail"}, nil
		}
		return nil, queue.ErrNoJobs
	}

	var handlerOkCalled bool
	pool.Register("test_ok", func(ctx context.Context, job *queue.Job) error {
		handlerOkCalled = true
		return nil
	})

	var handlerFailCalled bool
	pool.Register("test_fail", func(ctx context.Context, job *queue.Job) error {
		handlerFailCalled = true
		return errors.New("simulated failure")
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start pool in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Start(ctx)
	}()

	// Wait a bit for the pool to poll and process our 2 jobs
	time.Sleep(100 * time.Millisecond)
	cancel()
	wg.Wait()

	if !handlerOkCalled {
		t.Error("expected test_ok handler to be called")
	}
	if !handlerFailCalled {
		t.Error("expected test_fail handler to be called")
	}

	mockQueue.mu.Lock()
	defer mockQueue.mu.Unlock()

	if len(mockQueue.Acks) != 1 || mockQueue.Acks[0] != "job-1" {
		t.Errorf("expected job-1 to be acked, got %v", mockQueue.Acks)
	}

	if len(mockQueue.Nacks) != 1 || mockQueue.Nacks[0] != "job-2" {
		t.Errorf("expected job-2 to be nacked, got %v", mockQueue.Nacks)
	}
}
