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

// splitIntoChunks splits content by lines into chunks up to maxSize bytes.
func splitIntoChunks(content string, maxSize int) []string {
	if len(content) <= maxSize || maxSize <= 0 {
		return []string{content}
	}
	var chunks []string
	lines := strings.Split(content, "\n")
	var currentChunk strings.Builder

	for _, line := range lines {
		// If a single line is larger than maxSize, we still have to add it.
		if currentChunk.Len()+len(line)+1 > maxSize && currentChunk.Len() > 0 {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}
		currentChunk.WriteString(line)
		currentChunk.WriteString("\n")
	}
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}
	return chunks
}

// Extract invokes NuExtract logic via inference.
func (p *Provider) Extract(ctx context.Context, req extraction.Request) (*extraction.Result, error) {
	schema, err := p.getTemplateForTarget(req.Target)
	if err != nil {
		return nil, fmt.Errorf("getting schema: %w", err)
	}

	// For chunking, we limit each chunk to ~20000 bytes (approx 5k tokens),
	// leaving plenty of room for 16k context models.
	chunkSize := 20000
	if p.maxContentBytes > 0 && p.maxContentBytes < chunkSize {
		chunkSize = p.maxContentBytes
	}
	chunks := splitIntoChunks(req.Content, chunkSize)

	var baseJSON map[string]interface{}
	var allWarnings []string
	var finalRepaired bool

	for i, chunk := range chunks {
		if req.OnProgress != nil {
			req.OnProgress(fmt.Sprintf("Running AI extraction (chunk %d of %d)...", i+1, len(chunks)))
		}

		prompt := fmt.Sprintf("<|input|>\n%s\n<|template|>\n%s", chunk, schema)

		inferReq := lmstudio.InferRequest{
			Model: p.modelName,
			Messages: []lmstudio.ChatMessage{
				{
					Role:    "user",
					Content: prompt,
				},
			},
			Temperature: 0.0,
			MaxTokens:   16384,
		}

		inferOutput, err := p.client.Infer(ctx, inferReq)
		if err != nil {
			return nil, fmt.Errorf("%w: chunk %d: %v", extraction.ErrModelUnavailable, i+1, err)
		}

		jsonString, repaired, ok := extractAndRepairJSON(inferOutput)
		if !ok {
			if i == 0 {
				return nil, fmt.Errorf("%w: chunk 1: no valid JSON object found in output", extraction.ErrExtractionFailed)
			}
			allWarnings = append(allWarnings, fmt.Sprintf("chunk %d: no valid JSON object found, skipped", i+1))
			continue
		}
		if repaired {
			finalRepaired = true
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(jsonString), &parsed); err != nil {
			if i == 0 {
				return nil, fmt.Errorf("%w: chunk 1: invalid json: %v", extraction.ErrExtractionFailed, err)
			}
			allWarnings = append(allWarnings, fmt.Sprintf("chunk %d: invalid json, skipped", i+1))
			continue
		}

		if i == 0 {
			baseJSON = parsed
		} else {
			// Merge lists (arrays) from parsed into baseJSON
			for k, v := range parsed {
				if parsedList, ok := v.([]interface{}); ok {
					if baseList, ok := baseJSON[k].([]interface{}); ok {
						baseJSON[k] = append(baseList, parsedList...)
					} else if baseJSON[k] == nil {
						baseJSON[k] = parsedList
					}
				}
			}
		}
	}

	finalBytes, _ := json.MarshalIndent(baseJSON, "", "  ")

	if finalRepaired {
		allWarnings = append(allWarnings, "LLM output required repair (trailing commas or truncation); result may be incomplete")
	}

	return &extraction.Result{
		RawJSON:    json.RawMessage(finalBytes),
		Confidence: 0.85,
		Method:     p.modelName,
		Warnings:   allWarnings,
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
