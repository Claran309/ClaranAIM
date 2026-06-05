package graphstore

import (
	"ClaranAIM/internal/rag-service/model"
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestNeo4jStoreIntegration(t *testing.T) {
	uri := os.Getenv("NEO4J_TEST_URI")
	password := os.Getenv("NEO4J_TEST_PASSWORD")
	if uri == "" || password == "" {
		t.Skip("set NEO4J_TEST_URI and NEO4J_TEST_PASSWORD to run Neo4j integration tests")
	}
	username := os.Getenv("NEO4J_TEST_USERNAME")
	if username == "" {
		username = "neo4j"
	}
	database := os.Getenv("NEO4J_TEST_DATABASE")
	if database == "" {
		database = "neo4j"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewNeo4jStoreFromConfig(uri, username, password, database)
	if err != nil {
		t.Fatalf("NewNeo4jStoreFromConfig returned error: %v", err)
	}
	defer store.Close(context.Background())
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	ownerID := time.Now().UnixNano()
	cleanupNeo4jOwner(t, ctx, store, ownerID)
	defer cleanupNeo4jOwner(t, context.Background(), store, ownerID)

	source := &model.Entity{OwnerID: ownerID, Name: "agent-manager-service", CanonicalKey: "agentmanagerservice", Type: "Service", Score: 1}
	target := &model.Entity{OwnerID: ownerID, Name: "agent-runtime-service", CanonicalKey: "agentruntimeservice", Type: "Service", Score: 1}
	if err := store.SaveEntity(ctx, source); err != nil {
		t.Fatalf("SaveEntity source returned error: %v", err)
	}
	if err := store.SaveEntity(ctx, target); err != nil {
		t.Fatalf("SaveEntity target returned error: %v", err)
	}
	duplicate := &model.Entity{OwnerID: ownerID, Name: "agent-manager-service", CanonicalKey: "agentmanagerservice", Type: "Service", Score: 2}
	duplicate.ID = source.ID
	if err := store.SaveEntity(ctx, duplicate); err != nil {
		t.Fatalf("SaveEntity duplicate returned error: %v", err)
	}
	documentID := ownerID + 1
	if err := store.SaveRelation(ctx, &model.Relation{OwnerID: ownerID, SourceID: source.ID, TargetID: target.ID, Relation: "CALLS", Weight: 0.9, Confidence: 0.9, DocumentID: documentID}); err != nil {
		t.Fatalf("SaveRelation returned error: %v", err)
	}

	entities, relations, _, err := store.ListGraph(ctx, ownerID, "agent", 20, documentID, 1)
	if err != nil {
		t.Fatalf("ListGraph returned error: %v", err)
	}
	if len(entities) != 2 || len(relations) != 1 {
		t.Fatalf("graph entities=%#v relations=%#v, want 2 entities and 1 relation", entities, relations)
	}
	if err := store.DeleteDocumentGraph(ctx, ownerID, documentID); err != nil {
		t.Fatalf("DeleteDocumentGraph returned error: %v", err)
	}
	entities, relations, err = store.ListOwnerGraph(ctx, ownerID)
	if err != nil {
		t.Fatalf("ListOwnerGraph returned error: %v", err)
	}
	if len(entities) != 0 || len(relations) != 0 {
		t.Fatalf("owner graph after delete entities=%#v relations=%#v, want empty", entities, relations)
	}
}

func cleanupNeo4jOwner(t *testing.T, ctx context.Context, store *Neo4jStore, ownerID int64) {
	t.Helper()
	_, err := neo4j.ExecuteQuery(ctx, store.driver, `
MATCH (n)
WHERE n.owner_id = $owner_id
DETACH DELETE n`, map[string]any{"owner_id": ownerID}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(store.database), neo4j.ExecuteQueryWithWritersRouting())
	if err != nil && os.Getenv("NEO4J_TEST_STRICT_CLEANUP") == "true" {
		t.Fatalf("cleanup owner %s failed: %v", strconv.FormatInt(ownerID, 10), err)
	}
}
