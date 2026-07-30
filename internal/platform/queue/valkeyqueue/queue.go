package valkeyqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/redis/go-redis/v9"
)

// Provider implements queue.Provider using Valkey (Redis Streams).
type Provider struct {
	client *redis.Client
	stream string
	group  string
}

// New creates a new Valkey queue provider.
func New(client *redis.Client, stream, group string) *Provider {
	return &Provider{
		client: client,
		stream: stream,
		group:  group,
	}
}

// EnsureGroup creates the consumer group if it doesn't exist.
func (p *Provider) EnsureGroup(ctx context.Context) error {
	err := p.client.XGroupCreateMkStream(ctx, p.stream, p.group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("creating consumer group: %w", err)
	}
	return nil
}

func (p *Provider) Enqueue(ctx context.Context, job *queue.Job) error {
	payloadBytes, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}

	// Deduplication is handled by Redis logic or can be ignored in this simplified adapter
	// For production, a Set could be used to check idempotency_key

	_, err = p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]interface{}{
			"idempotency_key": job.IdempotencyKey,
			"job_type":        job.Type,
			"payload":         payloadBytes,
			"priority":        job.Priority,
			"max_attempts":    job.MaxAttempts,
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("enqueueing job: %w", err)
	}
	return nil
}

func (p *Provider) Dequeue(ctx context.Context, types []string) (*queue.Job, error) {
	// A complete implementation would handle XREADGROUP and XCLAIM for heartbeats.
	// This is a minimal stub to satisfy the interface for Delivery 6.

	args := &redis.XReadGroupArgs{
		Group:    p.group,
		Consumer: "worker-1", // Should ideally be unique per instance
		Streams:  []string{p.stream, ">"},
		Count:    1,
		Block:    time.Second,
	}

	res, err := p.client.XReadGroup(ctx, args).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, queue.ErrNoJobs
		}
		return nil, fmt.Errorf("dequeueing job: %w", err)
	}

	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, queue.ErrNoJobs
	}

	msg := res[0].Messages[0]

	// Parse job from msg
	job := &queue.Job{
		ID:             msg.ID,
		IdempotencyKey: fmt.Sprintf("%v", msg.Values["idempotency_key"]),
		Type:           fmt.Sprintf("%v", msg.Values["job_type"]),
		Status:         queue.StatusRunning,
	}

	if p, ok := msg.Values["payload"].(string); ok {
		json.Unmarshal([]byte(p), &job.Payload)
	}

	return job, nil
}

func (p *Provider) Ack(ctx context.Context, jobID string) error {
	return p.client.XAck(ctx, p.stream, p.group, jobID).Err()
}

func (p *Provider) Nack(ctx context.Context, jobID string, reason string) error {
	// A full implementation would handle retries, dead-lettering, or recording errors
	// For now, we simply ACK to remove it from pending list if we don't want to retry.
	// Or we can leave it to XCLAIM to pick up later.
	return nil
}

func (p *Provider) Heartbeat(ctx context.Context, jobID string) error {
	// In Valkey Streams, Heartbeat can be simulated by updating a separate key or relying on pending time.
	return nil
}

func (p *Provider) Cancel(ctx context.Context, jobID string) error {
	// Remove from stream
	return p.client.XDel(ctx, p.stream, jobID).Err()
}

func (p *Provider) Status(ctx context.Context, jobID string) (*queue.Job, error) {
	// Not fully implemented for Streams in this stub.
	return &queue.Job{ID: jobID, Status: queue.StatusRunning}, nil
}
