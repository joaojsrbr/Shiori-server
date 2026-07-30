package sqlitequeue_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
	"github.com/joaojsr/shiori-server/internal/platform/queue/sqlitequeue"
)

func TestProvider_Enqueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	provider := sqlitequeue.New(db)

	job := &queue.Job{
		IdempotencyKey: "idem-123",
		Type:           "fetch_page",
		Payload:        []byte(`{"url":"https://lycantoons.com/series/defensor-da-dungeon"}`),
	}

	// SQLiteQueue's Enqueue uses QueryRowContext with RETURNING id
	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)

	mock.ExpectQuery("INSERT INTO job_queue").
		WithArgs(
			job.IdempotencyKey,
			job.Type,
			string(job.Payload),
			queue.StatusQueued,
			0,                // priority
			3,                // max_attempts
			sqlmock.AnyArg(), // scheduled_at
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnRows(rows)

	err = provider.Enqueue(context.Background(), job)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if job.ID != "1" {
		t.Errorf("expected job ID to be '1', got %s", job.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}
