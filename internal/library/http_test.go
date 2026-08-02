package library_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/joaojsr/shiori-server/internal/library"
)

type mockMediaRepo struct {
	Media       []library.Media
	DeletedKeys []string
}

type mockStorage struct{ deleted []string }

func (m *mockStorage) Put(context.Context, string, io.Reader) error { return nil }
func (m *mockStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockStorage) Delete(_ context.Context, key string) error {
	m.deleted = append(m.deleted, key)
	return nil
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

func (m *mockMediaRepo) Delete(ctx context.Context, id string) ([]string, bool, error) {
	for index, media := range m.Media {
		if media.ID == id {
			m.Media = append(m.Media[:index], m.Media[index+1:]...)
			return m.DeletedKeys, true, nil
		}
	}
	return nil, false, nil
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

func TestLibraryHandler_DeleteMedia(t *testing.T) {
	repo := &mockMediaRepo{
		Media:       []library.Media{{ID: "media-1", Title: "Test"}},
		DeletedKeys: []string{"media/media-1/chapter/page.jpg"},
	}
	files := &mockStorage{}
	h := library.NewHandler(repo, files)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/media/media-1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rr.Code)
	}
	if len(repo.Media) != 0 {
		t.Fatalf("expected media to be removed, got %d entries", len(repo.Media))
	}
	if len(files.deleted) != 1 || files.deleted[0] != repo.DeletedKeys[0] {
		t.Fatalf("expected stored image to be deleted, got %v", files.deleted)
	}
}

func TestLibraryHandler_DeleteMediaNotFound(t *testing.T) {
	h := library.NewHandler(&mockMediaRepo{})
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodDelete, "/media/missing", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rr.Code)
	}
}
