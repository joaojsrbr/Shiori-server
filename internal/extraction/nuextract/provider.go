package nuextract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/joaojsr/shiori-server/internal/extraction"
	"github.com/joaojsr/shiori-server/internal/platform/ai/lmstudio"
)

// AIClient defines the dependencies we need from an AI HTTP Client (like lmstudio.Client)
type AIClient interface {
	Infer(ctx context.Context, req lmstudio.InferRequest) (string, error)
}

// Provider implements the extraction.Provider using NuExtract through LM Studio.
type Provider struct {
	client    AIClient
	modelName string
}

// New Creates a new NuExtract provider.
func New(client AIClient, modelName string) *Provider {
	return &Provider{
		client:    client,
		modelName: modelName,
	}
}

// getTemplateForTarget returns the JSON template for NuExtract to fill.
func getTemplateForTarget(target extraction.TargetType) (string, error) {
	switch target {
	case extraction.TargetMedia:
		return `{
			"type": "",
			"title": "",
			"alternative_titles": [],
			"description": "",
			"cover_url": "",
			"authors": [],
			"artists": [],
			"status": "",
			"genres": []
		}`, nil
	case extraction.TargetChapterList:
		return `{
			"chapters": [
				{
					"number": "",
					"title": "",
					"url": "",
					"date": ""
				}
			]
		}`, nil
	default:
		return "", fmt.Errorf("unknown target type: %s", target)
	}
}

// Extract invokes NuExtract logic via inference.
func (p *Provider) Extract(ctx context.Context, req extraction.Request) (*extraction.Result, error) {
	schema, err := getTemplateForTarget(req.Target)
	if err != nil {
		return nil, fmt.Errorf("getting schema: %w", err)
	}

	// Format NuExtract prompt
	// <|input|>\n{HTML}\n<|template|>\n{SCHEMA}
	prompt := fmt.Sprintf("<|input|>\n%s\n<|template|>\n%s", req.Content, schema)

	inferReq := lmstudio.InferRequest{
		Model: p.modelName,
		Messages: []lmstudio.ChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.0,
		MaxTokens:   4000,
	}

	content, err := p.client.Infer(ctx, inferReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", extraction.ErrModelUnavailable, err)
	}

	// Try to find raw JSON brackets since the LLM might add conversational padding
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")

	if start == -1 || end == -1 || start > end {
		return nil, fmt.Errorf("%w: failed to parse JSON brackets from output", extraction.ErrExtractionFailed)
	}

	jsonString := content[start : end+1]

	// Basic validation just to ensure it parses as JSON
	var dummy interface{}
	if err := json.Unmarshal([]byte(jsonString), &dummy); err != nil {
		return nil, fmt.Errorf("%w: output is not valid json: %v", extraction.ErrExtractionFailed, err)
	}

	return &extraction.Result{
		RawJSON:    json.RawMessage(jsonString),
		Confidence: 0.85, // NuExtract is quite reliable for structured schemas, but heuristic validation later confirms it
		Method:     p.modelName,
		Warnings:   nil,
	}, nil
}
