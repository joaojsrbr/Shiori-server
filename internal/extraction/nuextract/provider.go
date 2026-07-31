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
		// Keep MaxTokens moderate but large enough for hundreds of chapters.
		MaxTokens: 16384,
	}

	inferOutput, err := p.client.Infer(ctx, inferReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", extraction.ErrModelUnavailable, err)
	}

	// Extract the first complete JSON object from the output, repairing it if needed.
	// Handles: LLM markdown fences, trailing commas, and truncated output.
	jsonString, repaired, ok := extractAndRepairJSON(inferOutput)
	if !ok {
		return nil, fmt.Errorf("%w: no valid JSON object found in output", extraction.ErrExtractionFailed)
	}

	var warnings []string
	if repaired {
		warnings = append(warnings, "LLM output required repair (trailing commas or truncation); result may be incomplete")
	}

	return &extraction.Result{
		RawJSON:    json.RawMessage(jsonString),
		Confidence: 0.85,
		Method:     p.modelName,
		Warnings:   warnings,
	}, nil
}

// extractAndRepairJSON extracts the first complete JSON object from s, repairing it if needed.
// It handles:
//   - LLM padding/markdown fences before/after the JSON
//   - Trailing commas (e.g. `{"a": 1,}` or `[1, 2,]`)
//   - Truncated output (model ran out of context budget)
//
// Returns (json string, wasRepaired, ok).
func extractAndRepairJSON(s string) (string, bool, bool) {
	start := strings.Index(s, "{")
	if start == -1 {
		return "", false, false
	}
	s = s[start:]

	// Walk the JSON tracking brace depth to find end of the first complete object.
	depth := 0
	inString := false
	escaped := false
	end := -1

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
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if depth == 0 && end != -1 {
			break
		}
	}

	var candidate string
	repaired := false

	if end != -1 {
		// Complete (or apparently complete) JSON found
		candidate = s[:end+1]
	} else {
		// Truncated: repair by closing open structures
		repaired = true
		candidate = repairTruncated(s)
	}

	// Strip trailing commas (very common LLM quirk: `"key": "val",}`)
	cleaned, stripped := stripTrailingCommas(candidate)
	if stripped {
		repaired = true
	}

	var dummy interface{}
	if err := json.Unmarshal([]byte(cleaned), &dummy); err != nil {
		return "", false, false
	}

	return cleaned, repaired, true
}

// stripTrailingCommas removes trailing commas before `}` and `]` from JSON strings.
// e.g. `{"a": 1,}` → `{"a": 1}`
func stripTrailingCommas(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	changed := false
	runes := []rune(s)
	inString := false
	escaped := false

	for i, r := range runes {
		if escaped {
			escaped = false
			b.WriteRune(r)
			continue
		}
		if r == '\\' && inString {
			escaped = true
			b.WriteRune(r)
			continue
		}
		if r == '"' {
			inString = !inString
			b.WriteRune(r)
			continue
		}
		if inString {
			b.WriteRune(r)
			continue
		}
		if r == ',' {
			// Look ahead for the next non-whitespace character
			next := ' '
			for j := i + 1; j < len(runes); j++ {
				if runes[j] != ' ' && runes[j] != '\t' && runes[j] != '\n' && runes[j] != '\r' {
					next = runes[j]
					break
				}
			}
			if next == '}' || next == ']' {
				changed = true
				continue // skip the trailing comma
			}
		}
		b.WriteRune(r)
	}
	return b.String(), changed
}

// repairTruncated closes unclosed braces/brackets in truncated JSON output.
func repairTruncated(s string) string {
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

	result := s
	if inString {
		result += "\""
	}
	for i := len(stack) - 1; i >= 0; i-- {
		result += string(stack[i])
	}
	return result
}
