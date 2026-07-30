package library_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/library"
)

type mockMediaRepo struct {
	Media []library.Media
}

func (m *mockMediaRepo) Create(ctx context.Context, req library.MediaCreateRequest) (*library.Media, error) {
	newMedia := library.Media{
		ID:    library.NewULID(),
		Title: req.Title,
		Type:  req.Type,
	}
	m.Media = append(m.Media, newMedia)
	return &newMedia, nil
}

func (m *mockMediaRepo) GetByID(ctx context.Context, id string) (*library.Media, error) {
	for _, media := range m.Media {
		if media.ID == id {
			return &media, nil
		}
	}
	return nil, errors.New("media not found")
}

func (m *mockMediaRepo) List(ctx context.Context) ([]*library.Media, error) {
	// Not needed for this test, but satisfies interface
	return nil, nil
}

func (m *mockMediaRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func TestLibraryHandler_ListMedia(t *testing.T) {
	repo := &mockMediaRepo{
		Media: []library.Media{{ID: "1", Title: "Test"}},
	}
	h := library.NewHandler(repo)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/media", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}
}

func TestLibraryHandler_CreateMedia(t *testing.T) {
	repo := &mockMediaRepo{}
	h := library.NewHandler(repo)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	payload := library.MediaCreateRequest{
		Title:  "New Manga",
		Type:   "manga",
		Status: "ongoing",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/media", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rr.Code)
	}

	if len(repo.Media) != 1 {
		t.Errorf("expected 1 media in repo, got %d", len(repo.Media))
	}
}
