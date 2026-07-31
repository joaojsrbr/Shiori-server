package nuextract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	client          AIClient
	modelName       string
	templates       map[string]json.RawMessage
	maxContentBytes int
}

// New Creates a new NuExtract provider.
func New(client AIClient, modelName string, templatePath string, maxContentBytes int) (*Provider, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		// Fallback: try relative to the executable path (useful when double-clicking .exe or running from bin/)
		if exe, e := os.Executable(); e == nil {
			exeDir := filepath.Dir(exe)
			if fallbackData, e2 := os.ReadFile(filepath.Join(exeDir, templatePath)); e2 == nil {
				data = fallbackData
				err = nil
			} else if fallbackData2, e3 := os.ReadFile(filepath.Join(exeDir, "..", templatePath)); e3 == nil {
				data = fallbackData2
				err = nil
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading templates file: %w", err)
	}

	var templates map[string]json.RawMessage
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("parsing templates JSON: %w", err)
	}

	return &Provider{
		client:          client,
		modelName:       modelName,
		templates:       templates,
		maxContentBytes: maxContentBytes,
	}, nil
}

// getTemplateForTarget returns the JSON template for NuExtract to fill.
func (p *Provider) getTemplateForTarget(target extraction.TargetType) (string, error) {
	tmpl, ok := p.templates[string(target)]
	if !ok {
		return "", fmt.Errorf("unknown target type or missing template: %s", target)
	}
	return string(tmpl), nil
}

// Extract invokes NuExtract logic via inference.
func (p *Provider) Extract(ctx context.Context, req extraction.Request) (*extraction.Result, error) {
	schema, err := p.getTemplateForTarget(req.Target)
	if err != nil {
		return nil, fmt.Errorf("getting schema: %w", err)
	}

	// Truncate content to stay within the model's context window.
	// UTF-8 safe: truncate on rune boundary.
	content := req.Content
	if p.maxContentBytes > 0 && len(content) > p.maxContentBytes {
		// Walk runes until we exceed the limit
		truncated := content[:p.maxContentBytes]
		// Walk backward to find a valid UTF-8 boundary
		for len(truncated) > 0 {
			if truncated[len(truncated)-1]&0xC0 != 0x80 {
				break
			}
			truncated = truncated[:len(truncated)-1]
		}
		content = truncated
	}

	// Format NuExtract prompt
	// <|input|>\n{CONTENT}\n<|template|>\n{SCHEMA}
	prompt := fmt.Sprintf("<|input|>\n%s\n<|template|>\n%s", content, schema)

	inferReq := lmstudio.InferRequest{
		Model: p.modelName,
		Messages: []lmstudio.ChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.0,
		// Keep MaxTokens moderate: enough for the schema output but won't exhaust
		// the context budget left after the (potentially large) input prompt.
		MaxTokens: 4096,
	}

	inferOutput, err := p.client.Infer(ctx, inferReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", extraction.ErrModelUnavailable, err)
	}

	// Try to find raw JSON object in the output.
	// LLMs may add conversational padding or markdown fences around the JSON.
	start := strings.Index(inferOutput, "{")
	if start == -1 {
		return nil, fmt.Errorf("%w: no JSON object found in output", extraction.ErrExtractionFailed)
	}

	rawCandidate := inferOutput[start:]

	// If the JSON is truncated (model ran out of tokens), attempt to repair it
	// by balancing unclosed braces/brackets before validating.
	jsonString, repaired := repairJSON(rawCandidate)

	var dummy interface{}
	if err := json.Unmarshal([]byte(jsonString), &dummy); err != nil {
		return nil, fmt.Errorf("%w: output is not valid json: %v", extraction.ErrExtractionFailed, err)
	}

	var warnings []string
	if repaired {
		warnings = append(warnings, "LLM output was truncated; JSON was auto-repaired and may be incomplete")
	}

	return &extraction.Result{
		RawJSON:    json.RawMessage(jsonString),
		Confidence: 0.85,
		Method:     p.modelName,
		Warnings:   warnings,
	}, nil
}

// repairJSON attempts to close any unclosed JSON braces/brackets in a
// potentially truncated LLM output. Returns the (possibly repaired) string
// and a boolean indicating whether any repair was applied.
func repairJSON(s string) (string, bool) {
	// First try the string as-is (find last closing brace)
	end := strings.LastIndex(s, "}")
	if end != -1 {
		candidate := s[:end+1]
		var dummy interface{}
		if json.Unmarshal([]byte(candidate), &dummy) == nil {
			return candidate, false // already valid
		}
	}

	// Output is truncated. Walk the string tracking open structures,
	// then append the necessary closing characters.
	var stack []byte
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == ch {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if len(stack) == 0 {
		// Nothing to close; the JSON is just malformed in an unrecoverable way
		return s, false
	}

	// Close unclosed string if we're still inside one
	repaired := s
	if inString {
		repaired += "\""
	}
	// Close structures in reverse order
	for i := len(stack) - 1; i >= 0; i-- {
		repaired += string(stack[i])
	}

	return repaired, true
}
