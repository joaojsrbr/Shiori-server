package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/library/postgres"
)

func TestRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	repo := postgres.NewRepository(db)

	req := library.MediaCreateRequest{
		SourceURL:         "https://lycantoons.com/series/defensor-da-dungeon",
		Type:              "manga",
		Title:             "Defensor da Dungeon",
		AlternativeTitles: []string{},
		Description:       "Teste via sqlmock",
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
			"Teste via sqlmock",
			"https://lycantoons.com/series/defensor-da-dungeon",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			library.StatusOngoing,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("test-id", time.Now().UTC()))

	media, err := repo.Create(context.Background(), req)
	if err != nil {
		t.Errorf("error was not expected while inserting: %s", err)
	}

	if media == nil || media.Title != "Defensor da Dungeon" {
		t.Errorf("expected media title 'Defensor da Dungeon', got %v", media)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestRepository_GetByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	repo := postgres.NewRepository(db)

	rows := sqlmock.NewRows([]string{
		"id", "source_url", "type", "title", "alternative_titles", "description", "cover_url",
		"authors", "artists", "status", "genres", "created_at", "updated_at",
	}).AddRow(
		"test-id", "https://lycantoons.com", "manga", "Defensor da Dungeon", []byte("[]"), "Teste via sqlmock", "cover",
		[]byte("[]"), []byte("[]"), "ongoing", []byte("[]"), time.Now(), time.Now(),
	)

	mock.ExpectQuery("SELECT (.+) FROM media WHERE id = \\$1").
		WithArgs("test-id").
		WillReturnRows(rows)

	media, err := repo.GetByID(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("error was not expected: %v", err)
	}
	if media == nil || media.Title != "Defensor da Dungeon" {
		t.Errorf("expected media 'Defensor da Dungeon', got %v", media)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}
