package lmstudio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Model represents a model listed by LM Studio.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// Client is a generic HTTP client to interact with LM Studio local server.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new LM Studio client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{},
	}
}

// ListModels calls GET /v1/models to see what is currently loaded.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var payload struct {
		Data []Model `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return payload.Data, nil
}

// LoadModel instructs LM Studio to load a model into memory.
// It tries to POST to /v1/models with the model name, which is supported by some LM Studio builds.
func (c *Client) LoadModel(ctx context.Context, modelName string) error {
	payload := map[string]any{
		"model":            modelName,
		"context_length":   25000,
		"flash_attention":  true,
		"echo_load_config": true,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/models/load", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()

	// 200 or 201 is fine. If it returns 404 it means the endpoint might not be supported.
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status loading model: %s", resp.Status)
	}

	return nil
}

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InferRequest represents the payload for /v1/chat/completions.
type InferRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// InferResponse represents the OpenAI-compatible response.
type InferResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

// Infer sends a chat completion request to the loaded model.
func (c *Client) Infer(ctx context.Context, req InferRequest) (string, error) {
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status inferring: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var payload InferResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(payload.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}

	return payload.Choices[0].Message.Content, nil
}
