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

func (p *Provider) statusKey(jobID string) string { return p.stream + ":status:" + jobID }

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

	messageID, err := p.client.XAdd(ctx, &redis.XAddArgs{
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
	job.ID = messageID
	job.Status = queue.StatusQueued
	now := time.Now().UTC()
	job.CreatedAt, job.UpdatedAt = now, now
	encoded, _ := json.Marshal(job)
	if err := p.client.Set(ctx, p.statusKey(job.ID), encoded, 7*24*time.Hour).Err(); err != nil {
		return fmt.Errorf("recording job status: %w", err)
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
	job.UpdatedAt = time.Now().UTC()
	encoded, _ := json.Marshal(job)
	_ = p.client.Set(ctx, p.statusKey(job.ID), encoded, 7*24*time.Hour).Err()

	return job, nil
}

func (p *Provider) Ack(ctx context.Context, jobID string) error {
	if err := p.client.XAck(ctx, p.stream, p.group, jobID).Err(); err != nil {
		return err
	}
	return p.updateStatus(ctx, jobID, queue.StatusSucceeded, "")
}

func (p *Provider) Nack(ctx context.Context, jobID string, reason string) error {
	// A full implementation would handle retries, dead-lettering, or recording errors
	// For now, we simply ACK to remove it from pending list if we don't want to retry.
	// Or we can leave it to XCLAIM to pick up later.
	return p.updateStatus(ctx, jobID, queue.StatusFailed, reason)
}

func (p *Provider) Heartbeat(ctx context.Context, jobID string) error {
	// In Valkey Streams, Heartbeat can be simulated by updating a separate key or relying on pending time.
	return p.client.Expire(ctx, p.statusKey(jobID), 7*24*time.Hour).Err()
}

func (p *Provider) Cancel(ctx context.Context, jobID string) error {
	if _, err := p.client.XDel(ctx, p.stream, jobID).Result(); err != nil {
		return err
	}
	return p.updateStatus(ctx, jobID, queue.StatusCancelled, "")
}

func (p *Provider) Status(ctx context.Context, jobID string) (*queue.Job, error) {
	value, err := p.client.Get(ctx, p.statusKey(jobID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, queue.ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	var job queue.Job
	if err := json.Unmarshal(value, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (p *Provider) updateStatus(ctx context.Context, jobID string, status queue.JobStatus, reason string) error {
	job, err := p.Status(ctx, jobID)
	if err != nil {
		return err
	}
	job.Status, job.Error, job.UpdatedAt = status, reason, time.Now().UTC()
	encoded, _ := json.Marshal(job)
	return p.client.Set(ctx, p.statusKey(jobID), encoded, 7*24*time.Hour).Err()
}
