package service

import "testing"

func TestDedupeAndSortEdgesKeepsSameRelationWithDifferentEvidence(t *testing.T) {
	edges := []GraphEdge{
		{ID: 1, SourceID: 10, TargetID: 20, Relation: "生成", Weight: 0.8, Evidence: "第一段证据", DocumentID: 100},
		{ID: 2, SourceID: 10, TargetID: 20, Relation: "生成", Weight: 0.7, Evidence: "第二段证据", DocumentID: 100},
		{ID: 3, SourceID: 10, TargetID: 20, Relation: "生成", Weight: 0.9, Evidence: "第一段证据", DocumentID: 101},
	}

	got := dedupeAndSortEdges(edges)

	if len(got) != 3 {
		t.Fatalf("expected all evidence/document-distinct edges to remain, got %d: %+v", len(got), got)
	}
}

func TestNormalizeRelationKeepsFreeSpecificRelation(t *testing.T) {
	if got := normalizeRelation("面向"); got != "面向" {
		t.Fatalf("expected free relation to remain unchanged, got %q", got)
	}
}
