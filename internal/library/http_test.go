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

type mockFeatureRepo struct {
	settings library.Settings
	presets  []*library.FilterPreset
	history  []*library.BrowserHistoryEntry
}

func (m *mockFeatureRepo) ListCollections(ctx context.Context) ([]*library.Collection, error) {
	return nil, nil
}
func (m *mockFeatureRepo) CreateCollection(ctx context.Context, name, desc string) (*library.Collection, error) {
	return nil, nil
}
func (m *mockFeatureRepo) GetCollection(ctx context.Context, id string) (*library.Collection, error) {
	return nil, nil
}
func (m *mockFeatureRepo) UpdateCollection(ctx context.Context, id, name, desc string) (*library.Collection, error) {
	return nil, nil
}
func (m *mockFeatureRepo) DeleteCollection(ctx context.Context, id string) error { return nil }
func (m *mockFeatureRepo) ListCollectionMedia(ctx context.Context, id string, limit int, cursor string) ([]*library.Media, string, error) {
	return nil, "", nil
}
func (m *mockFeatureRepo) AddCollectionMedia(ctx context.Context, cid, mid string) error { return nil }
func (m *mockFeatureRepo) RemoveCollectionMedia(ctx context.Context, cid, mid string) error {
	return nil
}
func (m *mockFeatureRepo) ListHistory(ctx context.Context, limit int, cursor string) ([]*library.HistoryEntry, string, error) {
	return nil, "", nil
}
func (m *mockFeatureRepo) UpsertHistory(ctx context.Context, cid string, pos int, prog float64, comp bool) (*library.HistoryEntry, error) {
	return nil, nil
}
func (m *mockFeatureRepo) DeleteHistory(ctx context.Context, id string) error { return nil }
func (m *mockFeatureRepo) ListDownloads(ctx context.Context, limit int, cursor string) ([]*library.Download, string, error) {
	return nil, "", nil
}
func (m *mockFeatureRepo) DeleteDownload(ctx context.Context, id string) ([]string, error) {
	return nil, nil
}

func (m *mockFeatureRepo) ListProfiles(ctx context.Context) ([]*library.Profile, error) {
	return []*library.Profile{{ID: "default", Name: "Default Profile"}}, nil
}
func (m *mockFeatureRepo) GetSettings(ctx context.Context) (*library.Settings, error) {
	return &m.settings, nil
}
func (m *mockFeatureRepo) UpdateSettings(ctx context.Context, s library.Settings) (*library.Settings, error) {
	m.settings = s
	return &m.settings, nil
}
func (m *mockFeatureRepo) ListBrowserHistory(ctx context.Context, limit int, cursor string) ([]*library.BrowserHistoryEntry, string, error) {
	return m.history, "", nil
}
func (m *mockFeatureRepo) ListFilterPresets(ctx context.Context) ([]*library.FilterPreset, error) {
	return m.presets, nil
}
func (m *mockFeatureRepo) CreateFilterPreset(ctx context.Context, name string, filters map[string]any) (*library.FilterPreset, error) {
	preset := &library.FilterPreset{ID: "p1", Name: name, Filters: filters}
	m.presets = append(m.presets, preset)
	return preset, nil
}

func TestLibraryHandler_SystemAndFilterRoutes(t *testing.T) {
	featRepo := &mockFeatureRepo{
		settings: library.Settings{Theme: "dark", AdblockEnabled: true},
		history:  []*library.BrowserHistoryEntry{{ID: "h1", URL: "https://example.com", Title: "Example"}},
	}
	h := library.NewHandler(&mockMediaRepo{}, featRepo)
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	t.Run("GET /profiles", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/profiles", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("GET /settings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("PUT /settings", func(t *testing.T) {
		body, _ := json.Marshal(library.Settings{Theme: "light", AdblockEnabled: false})
		req := httptest.NewRequest(http.MethodPut, "/settings", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("GET /browser/history", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/browser/history", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("GET /filters/presets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/filters/presets", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("POST /filters/presets", func(t *testing.T) {
		input := library.FilterPresetInput{Name: "Manga Action", Filters: map[string]any{"type": "manga"}}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/filters/presets", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201 Created, got %d", rr.Code)
		}
	})
}
