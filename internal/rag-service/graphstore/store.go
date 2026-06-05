// Package graphstore defines GraphRAG persistence independent from MySQL document storage.
package graphstore

import (
	"ClaranAIM/internal/rag-service/model"
	"context"
)

// GraphStore is the persistence boundary for GraphRAG entities, relations, and communities.
type GraphStore interface {
	GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error)
	GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error)
	SaveEntity(ctx context.Context, entity *model.Entity) error
	SaveRelation(ctx context.Context, relation *model.Relation) error
	SaveCommunity(ctx context.Context, community *model.Community) error
	ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error)
	ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error
	ListGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error)
	DeleteDocumentGraph(ctx context.Context, ownerID, documentID int64) error
}
