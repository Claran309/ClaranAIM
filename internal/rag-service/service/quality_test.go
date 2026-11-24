package service

import (
	"strings"
	"testing"

	"ClaranAIM/internal/rag-service/model"
)

func TestSynthesizeAnswerFallbackExplainsMissingSources(t *testing.T) {
	answer := synthesizeAnswerFallback("what is ClaranAIM", "hybrid", "web_fallback", nil, nil)
	if !strings.Contains(answer, "没有在内部知识库中找到") || !strings.Contains(answer, "联网增强") {
		t.Fatalf("fallback answer = %q", answer)
	}
}

func TestSynthesizeAnswerFallbackCitesSources(t *testing.T) {
	chunks := []rankedChunk{
		{Chunk: model.Chunk{Content: "ClaranAIM is an AIM system."}, Document: model.Document{Title: "Architecture"}, Score: 1},
	}
	answer := synthesizeAnswerFallback("What is ClaranAIM?", "hybrid", "correct", chunks, nil)
	if !strings.Contains(answer, "Architecture") || !strings.Contains(answer, "ClaranAIM") {
		t.Fatalf("fallback answer = %q", answer)
	}
}
