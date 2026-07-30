package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/library/sqlite"
)

func TestRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	repo := sqlite.NewRepository(db)

	req := library.MediaCreateRequest{
		Type:              "manga",
		Title:             "Defensor da Dungeon",
		AlternativeTitles: []string{},
		Description:       "Teste via sqlmock SQLite",
		CoverURL:          "https://lycantoons.com/series/defensor-da-dungeon",
		Authors:           []string{},
		Artists:           []string{},
		Status:            "ongoing",
		Genres:            []string{},
	}

	mock.ExpectExec("INSERT INTO media").
		WithArgs(
			sqlmock.AnyArg(), // id
			req.Type,
			req.Title,
			"[]",
			req.Description,
			req.CoverURL,
			"[]",
			"[]",
			req.Status,
			"[]",
			sqlmock.AnyArg(), // created_at
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	media, err := repo.Create(context.Background(), req)
	if err != nil {
		t.Errorf("error was not expected while inserting: %s", err)
	}

	if media == nil || media.Title != "Defensor da Dungeon" {
		t.Errorf("expected media title 'Defensor da Dungeon'")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	repo := sqlite.NewRepository(db)

	rows := sqlmock.NewRows([]string{
		"id", "type", "title", "alternative_titles", "description", "cover_url",
		"authors", "artists", "status", "genres", "created_at", "updated_at",
	}).AddRow(
		"ulid-123", "manga", "Defensor da Dungeon", "[]", "Teste SQLite", "https://lycantoons.com/series/defensor-da-dungeon",
		"[]", "[]", "ongoing", "[]", time.Now(), time.Now(),
	)

	// SQLite uses ? instead of $1
	mock.ExpectQuery("SELECT (.+) FROM media WHERE id = \\?").
		WithArgs("ulid-123").
		WillReturnRows(rows)

	media, err := repo.GetByID(context.Background(), "ulid-123")
	if err != nil {
		t.Errorf("error was not expected: %s", err)
	}

	if media == nil || media.Title != "Defensor da Dungeon" {
		t.Errorf("expected media 'Defensor da Dungeon', got %v", media)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}
