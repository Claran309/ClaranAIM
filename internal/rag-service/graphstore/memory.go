package graphstore

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/idgen"
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore is a non-MySQL fallback graph store for tests and local disabled-Neo4j runs.
type MemoryStore struct {
	mu          sync.Mutex
	entities    []model.Entity
	relations   []model.Relation
	communities []model.Community
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entities {
		if s.entities[i].OwnerID == ownerID && s.entities[i].Name == name {
			entity := s.entities[i]
			return &entity, nil
		}
	}
	return nil, nil
}

func (s *MemoryStore) GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entities {
		if s.entities[i].OwnerID == ownerID && s.entities[i].CanonicalKey == canonicalKey {
			entity := s.entities[i]
			return &entity, nil
		}
	}
	return nil, nil
}

func (s *MemoryStore) SaveEntity(ctx context.Context, entity *model.Entity) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if entity.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		entity.ID = id
	}
	for i := range s.entities {
		if s.entities[i].ID == entity.ID || (entity.CanonicalKey != "" && s.entities[i].OwnerID == entity.OwnerID && s.entities[i].CanonicalKey == entity.CanonicalKey) {
			entity.ID = s.entities[i].ID
			s.entities[i] = *entity
			return nil
		}
	}
	s.entities = append(s.entities, *entity)
	return nil
}

func (s *MemoryStore) SaveRelation(ctx context.Context, relation *model.Relation) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if relation.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		relation.ID = id
	}
	s.relations = append(s.relations, *relation)
	return nil
}

func (s *MemoryStore) SaveCommunity(ctx context.Context, community *model.Community) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if community.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		community.ID = id
	}
	for i := range s.communities {
		if s.communities[i].ID == community.ID {
			s.communities[i] = *community
			return nil
		}
	}
	s.communities = append(s.communities, *community)
	return nil
}

func (s *MemoryStore) ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	entities := make([]model.Entity, 0, len(s.entities))
	for _, entity := range s.entities {
		if entity.OwnerID == ownerID {
			entities = append(entities, entity)
		}
	}
	relations := make([]model.Relation, 0, len(s.relations))
	for _, relation := range s.relations {
		if relation.OwnerID == ownerID {
			relations = append(relations, relation)
		}
	}
	return entities, relations, nil
}

func (s *MemoryStore) ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]model.Community, 0, len(s.communities)+len(communities))
	for _, community := range s.communities {
		if community.OwnerID != ownerID {
			kept = append(kept, community)
		}
	}
	for i := range communities {
		if communities[i].ID == 0 {
			id, err := idgen.NextID()
			if err != nil {
				return err
			}
			communities[i].ID = id
		}
		communities[i].OwnerID = ownerID
		kept = append(kept, communities[i])
	}
	s.communities = kept
	for i := range s.entities {
		if s.entities[i].OwnerID != ownerID {
			continue
		}
		if communityID, ok := entityCommunity[s.entities[i].ID]; ok {
			s.entities[i].CommunityID = communityID
		} else {
			s.entities[i].CommunityID = 0
		}
	}
	return nil
}

func (s *MemoryStore) ListGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	limit, hops = normalizeGraphLimits(limit, hops)
	query = strings.ToLower(strings.TrimSpace(query))
	canonicalQuery := normalizeGraphQueryKey(query)
	entityByID := map[int64]model.Entity{}
	seedIDs := map[int64]bool{}
	if documentID > 0 {
		relations := s.matchDocumentRelations(viewerID, documentID, query, canonicalQuery, limit)
		return s.expandGraphResultLocked(viewerID, documentID, hops, limit, relations)
	}
	for _, entity := range s.entities {
		if entity.OwnerID != viewerID && entity.OwnerID != 0 {
			continue
		}
		searchText := strings.ToLower(entity.Name + " " + entity.Summary + " " + entity.AliasesJSON + " " + entity.CanonicalKey)
		if query != "" && !strings.Contains(searchText, query) && !strings.Contains(entity.CanonicalKey, canonicalQuery) {
			continue
		}
		entityByID[entity.ID] = entity
		seedIDs[entity.ID] = true
		if len(seedIDs) >= limit {
			break
		}
	}
	relations := make([]model.Relation, 0, limit*3)
	for _, relation := range s.relations {
		if relation.OwnerID != viewerID && relation.OwnerID != 0 {
			continue
		}
		if !seedIDs[relation.SourceID] && !seedIDs[relation.TargetID] {
			continue
		}
		relations = append(relations, relation)
		if entity, ok := s.entityLocked(relation.SourceID); ok {
			entityByID[entity.ID] = entity
		}
		if entity, ok := s.entityLocked(relation.TargetID); ok {
			entityByID[entity.ID] = entity
		}
		if len(relations) >= limit*3 {
			break
		}
	}
	return s.finishGraphResultLocked(entityByID, relations), relations, s.communitiesForEntitiesLocked(entityByID), nil
}

func (s *MemoryStore) DeleteDocumentGraph(ctx context.Context, ownerID, documentID int64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	relations := make([]model.Relation, 0, len(s.relations))
	used := map[int64]bool{}
	for _, relation := range s.relations {
		if relation.DocumentID == documentID && relation.OwnerID == ownerID {
			continue
		}
		relations = append(relations, relation)
		if relation.OwnerID == ownerID {
			used[relation.SourceID] = true
			used[relation.TargetID] = true
		}
	}
	s.relations = relations
	entities := make([]model.Entity, 0, len(s.entities))
	for _, entity := range s.entities {
		if entity.OwnerID == ownerID && !used[entity.ID] {
			continue
		}
		entities = append(entities, entity)
	}
	s.entities = entities
	communities := make([]model.Community, 0, len(s.communities))
	for _, community := range s.communities {
		if community.OwnerID != ownerID {
			communities = append(communities, community)
		}
	}
	s.communities = communities
	return nil
}

func (s *MemoryStore) matchDocumentRelations(viewerID, documentID int64, query, canonicalQuery string, limit int) []model.Relation {
	relations := make([]model.Relation, 0, limit*3)
	for _, relation := range s.relations {
		if relation.DocumentID != documentID || (relation.OwnerID != viewerID && relation.OwnerID != 0) {
			continue
		}
		if query != "" {
			src, _ := s.entityLocked(relation.SourceID)
			dst, _ := s.entityLocked(relation.TargetID)
			searchText := strings.ToLower(src.Name + " " + src.Summary + " " + src.AliasesJSON + " " + src.CanonicalKey + " " + dst.Name + " " + dst.Summary + " " + dst.AliasesJSON + " " + dst.CanonicalKey + " " + relation.Description + " " + relation.Evidence)
			if !strings.Contains(searchText, query) && !strings.Contains(src.CanonicalKey, canonicalQuery) && !strings.Contains(dst.CanonicalKey, canonicalQuery) {
				continue
			}
		}
		relations = append(relations, relation)
		if len(relations) >= limit*3 {
			break
		}
	}
	return relations
}

func (s *MemoryStore) expandGraphResultLocked(viewerID, documentID int64, hops, limit int, relations []model.Relation) ([]model.Entity, []model.Relation, []model.Community, error) {
	entityByID := map[int64]model.Entity{}
	seenRelation := map[int64]bool{}
	allRelations := make([]model.Relation, 0, len(relations)+limit)
	for _, relation := range relations {
		allRelations = append(allRelations, relation)
		seenRelation[relation.ID] = true
		if entity, ok := s.entityLocked(relation.SourceID); ok {
			entityByID[entity.ID] = entity
		}
		if entity, ok := s.entityLocked(relation.TargetID); ok {
			entityByID[entity.ID] = entity
		}
	}
	if hops > 1 && len(entityByID) > 0 {
		for _, relation := range s.relations {
			if seenRelation[relation.ID] || relation.DocumentID == documentID || (relation.OwnerID != viewerID && relation.OwnerID != 0) {
				continue
			}
			if _, ok := entityByID[relation.SourceID]; !ok {
				if _, ok := entityByID[relation.TargetID]; !ok {
					continue
				}
			}
			allRelations = append(allRelations, relation)
			if entity, ok := s.entityLocked(relation.SourceID); ok {
				entityByID[entity.ID] = entity
			}
			if entity, ok := s.entityLocked(relation.TargetID); ok {
				entityByID[entity.ID] = entity
			}
			if len(allRelations) >= limit*4 {
				break
			}
		}
	}
	return s.finishGraphResultLocked(entityByID, allRelations), allRelations, s.communitiesForEntitiesLocked(entityByID), nil
}

func (s *MemoryStore) finishGraphResultLocked(entityByID map[int64]model.Entity, relations []model.Relation) []model.Entity {
	entities := make([]model.Entity, 0, len(entityByID))
	for _, entity := range entityByID {
		entities = append(entities, entity)
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Score == entities[j].Score {
			return entities[i].UpdatedAt.After(entities[j].UpdatedAt)
		}
		return entities[i].Score > entities[j].Score
	})
	_ = relations
	return entities
}

func (s *MemoryStore) communitiesForEntitiesLocked(entityByID map[int64]model.Entity) []model.Community {
	communityIDs := map[int64]bool{}
	for _, entity := range entityByID {
		if entity.CommunityID > 0 {
			communityIDs[entity.CommunityID] = true
		}
	}
	communities := make([]model.Community, 0, len(communityIDs))
	for _, community := range s.communities {
		if communityIDs[community.ID] {
			communities = append(communities, community)
		}
	}
	return communities
}

func (s *MemoryStore) entityLocked(id int64) (model.Entity, bool) {
	for _, entity := range s.entities {
		if entity.ID == id {
			return entity, true
		}
	}
	return model.Entity{}, false
}

func normalizeGraphLimits(limit, hops int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	if hops <= 0 {
		hops = 1
	}
	if hops > 2 {
		hops = 2
	}
	return limit, hops
}

func normalizeGraphQueryKey(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
