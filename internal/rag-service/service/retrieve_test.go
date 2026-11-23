package service

import (
	"testing"

	"ClaranAIM/internal/rag-service/model"
)

func TestBM25TokensAddsChineseBigrams(t *testing.T) {
	tokens := bm25Tokens("ClaranAIM 知识库检索")
	if len(tokens) < 4 {
		t.Fatalf("tokens = %#v, want words and Chinese bigrams", tokens)
	}
	found := false
	for _, token := range tokens {
		if token == "知识" || token == "识库" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tokens missing Chinese bigram: %#v", tokens)
	}
}

func TestReciprocalRankFusionRewardsBothRanks(t *testing.T) {
	both := reciprocalRankFusionScore(1, 1)
	denseOnly := reciprocalRankFusionScore(1, 0)
	weak := reciprocalRankFusionScore(10, 10)
	if both <= weak || weak <= denseOnly {
		t.Fatalf("RRF ordering broken: both=%f dense=%f weak=%f", both, denseOnly, weak)
	}
}

func TestRerankAndRetrievalQuality(t *testing.T) {
	chunks := []rankedChunk{
		{Chunk: model.Chunk{Content: "unrelated"}, Document: model.Document{Title: "A"}, Score: 1},
		{Chunk: model.Chunk{Content: "ClaranAIM retrieval"}, Document: model.Document{Title: "Retrieval"}, Score: 1},
	}
	reranked := localRerank("ClaranAIM retrieval", chunks)
	if len(reranked) != 2 || reranked[0].Score < reranked[1].Score {
		t.Fatalf("localRerank order = %#v", reranked)
	}
	if quality := retrievalQuality(nil); quality != 0 {
		t.Fatalf("empty retrieval quality = %f, want 0", quality)
	}
	if quality := retrievalQuality(reranked); quality <= 0 {
		t.Fatalf("retrieval quality = %f, want positive", quality)
	}
}
