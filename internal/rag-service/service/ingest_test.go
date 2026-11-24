package service

import (
	"strings"
	"testing"
)

func TestPlanMarkdownChunksBuildsParentChildPlans(t *testing.T) {
	content := "# Project Doc\n\n## Architecture\n\nClaranAIM uses services.\n\n## Retrieval\n\nHybrid retrieval combines BM25 and vectors."
	plans := planMarkdownChunks(content)
	if len(plans) < 2 {
		t.Fatalf("plans = %d, want at least 2", len(plans))
	}
	joined := ""
	for _, plan := range plans {
		joined += plan.parentTitle + " " + plan.parentContent + " "
	}
	if !strings.Contains(joined, "Architecture") || !strings.Contains(joined, "Retrieval") {
		t.Fatalf("markdown plans lost section titles: %q", joined)
	}
}

func TestSplitLongParagraphKeepsContent(t *testing.T) {
	text := strings.Repeat("ClaranAIM retrieval paragraph. ", 40)
	parts := splitLongParagraph(text, 120)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want multiple chunks", len(parts))
	}
	for _, part := range parts {
		if len([]rune(part)) > 160 {
			t.Fatalf("part too long: %d runes", len([]rune(part)))
		}
	}
}

func TestSplitIntoChunksHandlesChinese(t *testing.T) {
	parts := splitIntoChunks(strings.Repeat("知识库检索与重排。", 20), 50)
	if len(parts) == 0 {
		t.Fatal("splitIntoChunks returned no parts")
	}
	for _, part := range parts {
		if strings.ContainsRune(part, '\uFFFD') {
			t.Fatalf("invalid UTF-8 replacement rune in %q", part)
		}
	}
}
