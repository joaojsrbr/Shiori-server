package nuextract_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/extraction/nuextract"
	"github.com/joaojsr/shiori-server/internal/platform/ai/lmstudio"
)

// MockAIClient mocks the LM Studio Client.
type MockAIClient struct {
	Response string
	Err      error
	LastReq  *lmstudio.InferRequest
}

func (m *MockAIClient) Infer(ctx context.Context, req lmstudio.InferRequest) (string, error) {
	m.LastReq = &req
	return m.Response, m.Err
}

func createTestTemplates(t *testing.T) string {
	t.Helper()
	content := `{
		"manga": {"type": "manga"}
	}`
	tmp := filepath.Join(t.TempDir(), "templates.json")
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return tmp
}

func TestProvider_Extract_Success(t *testing.T) {
	mockClient := &MockAIClient{
		Response: "```json\n{\n  \"title\": \"One Piece\",\n  \"type\": \"manga\"\n}\n```",
	}

	tmplPath := createTestTemplates(t)
	provider, err := nuextract.New(mockClient, "nuextract-tiny", tmplPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	req := extraction.Request{
		Content: "<html>One Piece manga HTML here</html>",
		Target:  extraction.TargetManga,
	}

	res, err := provider.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res == nil {
		t.Fatal("expected result, got nil")
	}

	expectedJSON := `{\n  "title": "One Piece",\n  "type": "manga"\n}`
	expectedJSON = strings.ReplaceAll(expectedJSON, "\\n", "\n")

	if string(res.RawJSON) != expectedJSON {
		t.Errorf("expected %s, got %s", expectedJSON, res.RawJSON)
	}

	if res.Method != "nuextract-tiny" {
		t.Errorf("expected nuextract-tiny, got %s", res.Method)
	}

	// Check if prompt was built correctly
	if mockClient.LastReq == nil {
		t.Fatal("last req should not be nil")
	}

	prompt := mockClient.LastReq.Messages[0].Content
	if !strings.Contains(prompt, "<|input|>\n<html>One Piece") {
		t.Errorf("prompt missing correct input: %s", prompt)
	}
	if !strings.Contains(prompt, "<|template|>\n{") {
		t.Errorf("prompt missing correct template schema")
	}
}

func TestProvider_Extract_ClientError(t *testing.T) {
	mockClient := &MockAIClient{
		Err: errors.New("timeout"),
	}

	tmplPath := createTestTemplates(t)
	provider, _ := nuextract.New(mockClient, "nuextract-tiny", tmplPath)

	_, err := provider.Extract(context.Background(), extraction.Request{Target: extraction.TargetManga})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, extraction.ErrModelUnavailable) {
		t.Errorf("expected ErrModelUnavailable, got %v", err)
	}
}

func TestProvider_Extract_InvalidJSON(t *testing.T) {
	mockClient := &MockAIClient{
		Response: "I am an AI, I cannot help you with that.",
	}

	tmplPath := createTestTemplates(t)
	provider, _ := nuextract.New(mockClient, "nuextract-tiny", tmplPath)

	_, err := provider.Extract(context.Background(), extraction.Request{Target: extraction.TargetManga})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, extraction.ErrExtractionFailed) {
		t.Errorf("expected ErrExtractionFailed, got %v", err)
	}
}
