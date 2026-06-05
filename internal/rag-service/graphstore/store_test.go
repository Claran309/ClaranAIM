package graphstore

import (
	"ClaranAIM/internal/rag-service/model"
	"context"
	"testing"
)

func TestMemoryStoreDeletesDocumentGraphAndOrphans(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	a := &model.Entity{OwnerID: 1001, Name: "agent-manager-service", CanonicalKey: "agentmanagerservice", Type: "Service", Score: 1}
	b := &model.Entity{OwnerID: 1001, Name: "agent-runtime-service", CanonicalKey: "agentruntimeservice", Type: "Service", Score: 1}
	c := &model.Entity{OwnerID: 1001, Name: "event_outbox", CanonicalKey: "eventoutbox", Type: "Table", Score: 1}
	for _, entity := range []*model.Entity{a, b, c} {
		if err := store.SaveEntity(ctx, entity); err != nil {
			t.Fatalf("SaveEntity returned error: %v", err)
		}
	}
	if err := store.SaveRelation(ctx, &model.Relation{OwnerID: 1001, SourceID: a.ID, TargetID: b.ID, Relation: "CALLS", DocumentID: 2001}); err != nil {
		t.Fatalf("SaveRelation returned error: %v", err)
	}
	if err := store.SaveRelation(ctx, &model.Relation{OwnerID: 1001, SourceID: b.ID, TargetID: c.ID, Relation: "WRITES", DocumentID: 2002}); err != nil {
		t.Fatalf("SaveRelation returned error: %v", err)
	}

	if err := store.DeleteDocumentGraph(ctx, 1001, 2001); err != nil {
		t.Fatalf("DeleteDocumentGraph returned error: %v", err)
	}

	entities, relations, err := store.ListOwnerGraph(ctx, 1001)
	if err != nil {
		t.Fatalf("ListOwnerGraph returned error: %v", err)
	}
	if len(relations) != 1 || relations[0].DocumentID != 2002 {
		t.Fatalf("relations=%#v, want only document 2002 relation", relations)
	}
	for _, entity := range entities {
		if entity.ID == a.ID {
			t.Fatalf("orphan entity %#v was not deleted", entity)
		}
	}
}
