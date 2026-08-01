package nuextract

import (
	"crypto/sha256"
	"strings"
	"unicode/utf8"
)

const (
	defaultContextTokens      = 8192
	defaultOutputTokens       = 2048
	contextOverheadTokens     = 512
	conservativeBytesPerToken = 3
)

// contextBudget derives an input byte ceiling and output-token ceiling from
// the model context. The byte estimate is deliberately conservative because
// LM Studio does not expose the tokenizer through the OpenAI-compatible API.
func contextBudget(contextTokens, configuredMaxBytes, schemaBytes int) (int, int) {
	if contextTokens <= 0 {
		contextTokens = defaultContextTokens
	}

	outputTokens := defaultOutputTokens
	if quarter := contextTokens / 4; quarter < outputTokens {
		outputTokens = quarter
	}
	if outputTokens < 512 {
		outputTokens = 512
	}

	schemaTokens := (schemaBytes + conservativeBytesPerToken - 1) / conservativeBytesPerToken
	inputTokens := contextTokens - outputTokens - contextOverheadTokens - schemaTokens
	if inputTokens < 256 {
		inputTokens = 256
	}
	maxBytes := inputTokens * conservativeBytesPerToken
	if configuredMaxBytes > 0 && configuredMaxBytes < maxBytes {
		maxBytes = configuredMaxBytes
	}
	return maxBytes, outputTokens
}

// splitSemanticMarkdown packs complete Markdown blocks up to maxBytes. A
// heading becomes breadcrumb context for following chunks instead of copying a
// blind character overlap. Oversized blocks are split by lines and finally at
// UTF-8 boundaries, so the hard limit is always respected.
func splitSemanticMarkdown(content string, maxBytes int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return []string{""}
	}
	if maxBytes <= 0 || len(content) <= maxBytes {
		return []string{content}
	}

	blocks := markdownBlocks(content)
	chunks := make([]string, 0, len(blocks))
	seen := make(map[[32]byte]struct{}, len(blocks))
	var current strings.Builder
	var heading string

	flush := func() {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		current.Reset()
	}

	appendPart := func(part string) {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		prefix := ""
		if heading != "" && part != heading {
			prefix = heading + "\n\n"
		}
		separator := ""
		if current.Len() > 0 {
			separator = "\n\n"
		}
		if current.Len()+len(separator)+len(part) > maxBytes {
			flush()
			if prefix != "" && len(prefix)+len(part) <= maxBytes {
				current.WriteString(prefix)
			}
		}
		current.WriteString(part)
	}

	for _, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			continue
		}
		hash := sha256.Sum256([]byte(trimmed))
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}

		if isHeading(trimmed) {
			heading = firstLine(trimmed)
		}
		partLimit := maxBytes
		if heading != "" && trimmed != heading && len(heading)+2 < maxBytes {
			partLimit -= len(heading) + 2
		}
		for _, part := range splitOversizedBlock(trimmed, partLimit) {
			appendPart(part)
		}
	}
	flush()
	return chunks
}

func markdownBlocks(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var blocks []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, strings.Join(current, "\n"))
			current = nil
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "#") && len(current) > 0 {
			flush()
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func splitOversizedBlock(block string, maxBytes int) []string {
	if len(block) <= maxBytes {
		return []string{block}
	}
	var parts []string
	var current strings.Builder
	for _, line := range strings.Split(block, "\n") {
		lineParts := splitUTF8(line, maxBytes)
		for _, part := range lineParts {
			separator := ""
			if current.Len() > 0 {
				separator = "\n"
			}
			if current.Len()+len(separator)+len(part) > maxBytes {
				parts = append(parts, current.String())
				current.Reset()
				separator = ""
			}
			current.WriteString(separator)
			current.WriteString(part)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func splitUTF8(value string, maxBytes int) []string {
	if len(value) <= maxBytes {
		return []string{value}
	}
	var parts []string
	for len(value) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(value[cut]) {
			cut--
		}
		if cut == 0 {
			_, size := utf8.DecodeRuneInString(value)
			cut = size
		}
		parts = append(parts, value[:cut])
		value = value[cut:]
	}
	if value != "" {
		parts = append(parts, value)
	}
	return parts
}

func isHeading(block string) bool {
	line := firstLine(block)
	return strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ")
}

func firstLine(value string) string {
	if i := strings.IndexByte(value, '\n'); i >= 0 {
		return value[:i]
	}
	return value
}
