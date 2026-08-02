package nuextract

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestContextBudgetReservesTemplateAndOutput(t *testing.T) {
	maxBytes, outputTokens := contextBudget(8192, 12000, 900)
	if maxBytes > 12000 {
		t.Fatalf("maxBytes = %d, exceeds configured hard limit", maxBytes)
	}
	if outputTokens != 8192 {
		t.Fatalf("outputTokens = %d, want 8192", outputTokens)
	}
}

func TestSplitSemanticMarkdownPreservesLimitAndUTF8(t *testing.T) {
	content := "# Manga\n\n" + strings.Repeat("capítulo muito longo ", 30) + "\n\n## Chapters\n\n- 1\n- 2\n- 3"
	chunks := splitSemanticMarkdown(content, 96)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want multiple chunks", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk) > 96 {
			t.Errorf("chunk %d has %d bytes, hard limit is 96", i, len(chunk))
		}
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}
	}
}

func TestSplitSemanticMarkdownCarriesHeadingAndDeduplicates(t *testing.T) {
	content := "## Chapters\n\n- Chapter 1\n\n- Chapter 1\n\n- Chapter 2 has a longer descriptive title"
	chunks := splitSemanticMarkdown(content, 52)
	joined := strings.Join(chunks, "\n---\n")
	if strings.Count(joined, "- Chapter 1") != 1 {
		t.Fatalf("duplicate block was not removed: %q", joined)
	}
	if len(chunks) > 1 && !strings.Contains(chunks[1], "## Chapters") {
		t.Fatalf("heading context not carried into next chunk: %q", chunks[1])
	}
}
