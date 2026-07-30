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
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := sqlite.NewRepository(db)
	nowStr := time.Now().Format(time.RFC3339)

	req := library.MediaCreateRequest{
		SourceURL:         "https://lycantoons.com/series/defensor-da-dungeon",
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

	mock.ExpectQuery("INSERT INTO media").
		WithArgs(
			sqlmock.AnyArg(),
			"https://lycantoons.com/series/defensor-da-dungeon",
			library.MediaTypeManga,
			"Defensor da Dungeon",
			sqlmock.AnyArg(),
			"Teste via sqlmock SQLite",
			"https://lycantoons.com/series/defensor-da-dungeon",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			library.StatusOngoing,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("test-id", nowStr))

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
	nowStr := time.Now().Format(time.RFC3339)

	rows := sqlmock.NewRows([]string{
		"id", "source_url", "type", "title", "alternative_titles", "description", "cover_url",
		"authors", "artists", "status", "genres", "created_at", "updated_at",
	}).AddRow(
		"test-id", "https://lycantoons.com", "manga", "Defensor da Dungeon", "[]", "Teste via sqlmock SQLite", "cover",
		"[]", "[]", "ongoing", "[]", nowStr, nowStr,
	)

	// SQLite uses ? instead of $1
	mock.ExpectQuery("SELECT (.+) FROM media WHERE id = \\?").
		WithArgs("test-id").
		WillReturnRows(rows)

	media, err := repo.GetByID(context.Background(), "test-id")
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
