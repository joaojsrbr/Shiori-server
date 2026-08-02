package lmstudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Model represents a model listed by LM Studio.
type Model struct {
	ID              string `json:"id,omitempty"`
	Key             string `json:"key,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	LoadedInstances []struct {
		ID string `json:"id"`
	} `json:"loaded_instances,omitempty"`
}

// Client is a generic HTTP client to interact with LM Studio local server.
type Client struct {
	baseURL     string
	token       string
	httpClient  *http.Client // for quick operations (list models, load model)
	inferClient *http.Client // for inference: no timeout (rely on ctx deadline)
}

// NewClient creates a new LM Studio client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		inferClient: &http.Client{
			// No built-in timeout: inference can take minutes.
			// The caller must set a deadline on the context.
		},
	}
}

// ListModels calls LM Studio's native model-management API. Unlike the
// OpenAI-compatible list endpoint, it identifies loaded instances explicitly.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/models", nil)
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
		Models []Model `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return payload.Models, nil
}

// UnloadModel releases one loaded LM Studio model instance from memory.
func (c *Client) UnloadModel(ctx context.Context, instanceID string) error {
	body, _ := json.Marshal(map[string]string{"instance_id": instanceID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/models/unload", bytes.NewReader(body))
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
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status unloading model: %s, body: %s", resp.Status, data)
	}
	return nil
}

// LoadModel instructs LM Studio to load a model into memory.
// It tries to POST to /v1/models with the model name, which is supported by some LM Studio builds.
func (c *Client) LoadModel(ctx context.Context, modelName string, contextLength int) error {
	payload := map[string]any{
		"model":            modelName,
		"context_length":   contextLength,
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

type InferProgress struct {
	GeneratedTokens int
	TokensPerSecond float64
	Elapsed         time.Duration
	Done            bool
	Estimated       bool
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

	resp, err := c.inferClient.Do(httpReq)
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

// InferStream uses the OpenAI-compatible SSE response. LM Studio does not
// expose a reliable completion percentage, but streaming provides live token
// generation and final usage when supported by the loaded runtime.
func (c *Client) InferStream(ctx context.Context, req InferRequest, onProgress func(InferProgress)) (string, error) {
	payload := struct {
		InferRequest
		Stream        bool            `json:"stream"`
		StreamOptions map[string]bool `json:"stream_options,omitempty"`
	}{InferRequest: req, Stream: true, StreamOptions: map[string]bool{"include_usage": true}}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.inferClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status inferring: %s, body: %s", resp.Status, string(bodyBytes))
	}

	type streamChunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Usage *struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage,omitempty"`
	}
	started := time.Now()
	generated := 0
	usageSeen := false
	var output strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				output.WriteString(choice.Delta.Content)
				generated++
			}
		}
		if chunk.Usage != nil && chunk.Usage.CompletionTokens > 0 {
			generated = chunk.Usage.CompletionTokens
			usageSeen = true
		}
		elapsed := time.Since(started)
		rate := 0.0
		if elapsed > 0 {
			rate = float64(generated) / elapsed.Seconds()
		}
		if onProgress != nil {
			onProgress(InferProgress{GeneratedTokens: generated, TokensPerSecond: rate, Elapsed: elapsed, Estimated: !usageSeen})
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading streamed response: %w", err)
	}
	if onProgress != nil {
		elapsed := time.Since(started)
		rate := 0.0
		if elapsed > 0 {
			rate = float64(generated) / elapsed.Seconds()
		}
		onProgress(InferProgress{GeneratedTokens: generated, TokensPerSecond: rate, Elapsed: elapsed, Done: true, Estimated: !usageSeen})
	}
	if output.Len() == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return output.String(), nil
}
