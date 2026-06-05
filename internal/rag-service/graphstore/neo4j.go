package graphstore

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/idgen"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jStore struct {
	driver   neo4j.DriverWithContext
	database string
}

func NewNeo4jStore(driver neo4j.DriverWithContext, database string) *Neo4jStore {
	return &Neo4jStore{driver: driver, database: strings.TrimSpace(database)}
}

func NewNeo4jStoreFromConfig(uri, username, password, database string) (*Neo4jStore, error) {
	driver, err := neo4j.NewDriverWithContext(strings.TrimSpace(uri), neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, err
	}
	return NewNeo4jStore(driver, database), nil
}

func (s *Neo4jStore) Close(ctx context.Context) error {
	if s == nil || s.driver == nil {
		return nil
	}
	return s.driver.Close(ctx)
}

func (s *Neo4jStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE CONSTRAINT entity_id IF NOT EXISTS FOR (e:Entity) REQUIRE e.id IS UNIQUE`,
		`CREATE INDEX entity_owner_key IF NOT EXISTS FOR (e:Entity) ON (e.owner_id, e.canonical_key)`,
		`CREATE INDEX entity_owner_name IF NOT EXISTS FOR (e:Entity) ON (e.owner_id, e.name)`,
		`CREATE INDEX entity_owner_type IF NOT EXISTS FOR (e:Entity) ON (e.owner_id, e.type)`,
		`CREATE CONSTRAINT community_id IF NOT EXISTS FOR (c:Community) REQUIRE c.id IS UNIQUE`,
		`CREATE INDEX community_owner IF NOT EXISTS FOR (c:Community) ON (c.owner_id)`,
		`CREATE INDEX relation_owner_doc IF NOT EXISTS FOR ()-[r:GRAPH_RELATION]-() ON (r.owner_id, r.document_id)`,
	}
	for _, statement := range statements {
		if _, err := s.execute(ctx, statement, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *Neo4jStore) GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error) {
	result, err := s.execute(ctx, `
MATCH (e:Entity {owner_id: $owner_id, name: $name})
RETURN e
ORDER BY e.updated_at DESC
LIMIT 1`, map[string]any{"owner_id": ownerID, "name": name})
	if err != nil || len(result.Records) == 0 {
		return nil, err
	}
	value, _, err := neo4j.GetRecordValue[neo4j.Node](result.Records[0], "e")
	if err != nil {
		return nil, err
	}
	entity := entityFromProps(value.Props)
	return &entity, nil
}

func (s *Neo4jStore) GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error) {
	result, err := s.execute(ctx, `
MATCH (e:Entity {owner_id: $owner_id, canonical_key: $canonical_key})
RETURN e
ORDER BY e.updated_at DESC
LIMIT 1`, map[string]any{"owner_id": ownerID, "canonical_key": canonicalKey})
	if err != nil || len(result.Records) == 0 {
		return nil, err
	}
	value, _, err := neo4j.GetRecordValue[neo4j.Node](result.Records[0], "e")
	if err != nil {
		return nil, err
	}
	entity := entityFromProps(value.Props)
	return &entity, nil
}

func (s *Neo4jStore) SaveEntity(ctx context.Context, entity *model.Entity) error {
	if entity == nil {
		return nil
	}
	if entity.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		entity.ID = id
	}
	now := time.Now()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now
	props := map[string]any{
		"id":            entity.ID,
		"owner_id":      entity.OwnerID,
		"name":          entity.Name,
		"canonical_key": entity.CanonicalKey,
		"type":          entity.Type,
		"summary":       entity.Summary,
		"aliases_json":  entity.AliasesJSON,
		"community_id":  entity.CommunityID,
		"score":         entity.Score,
		"created_at":    entity.CreatedAt.UnixMilli(),
		"updated_at":    entity.UpdatedAt.UnixMilli(),
	}
	_, err := s.execute(ctx, `
MERGE (e:Entity {id: $id})
ON CREATE SET e.created_at = $created_at
SET e.owner_id = $owner_id,
    e.name = $name,
    e.canonical_key = $canonical_key,
    e.type = $type,
    e.summary = $summary,
    e.aliases_json = $aliases_json,
    e.community_id = $community_id,
    e.score = $score,
    e.updated_at = $updated_at`, props)
	return err
}

func (s *Neo4jStore) SaveRelation(ctx context.Context, relation *model.Relation) error {
	if relation == nil || relation.SourceID == 0 || relation.TargetID == 0 {
		return nil
	}
	if relation.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		relation.ID = id
	}
	now := time.Now()
	if relation.CreatedAt.IsZero() {
		relation.CreatedAt = now
	}
	relation.UpdatedAt = now
	_, err := s.execute(ctx, `
MATCH (source:Entity {id: $source_id})
MATCH (target:Entity {id: $target_id})
MERGE (source)-[r:GRAPH_RELATION {id: $id}]->(target)
ON CREATE SET r.created_at = $created_at
SET r.owner_id = $owner_id,
    r.relation = $relation,
    r.description = $description,
    r.weight = $weight,
    r.confidence = $confidence,
    r.evidence = $evidence,
    r.evidence_chunk_id = $evidence_chunk_id,
    r.document_id = $document_id,
    r.updated_at = $updated_at`, map[string]any{
		"id":                relation.ID,
		"owner_id":          relation.OwnerID,
		"source_id":         relation.SourceID,
		"target_id":         relation.TargetID,
		"relation":          relation.Relation,
		"description":       relation.Description,
		"weight":            relation.Weight,
		"confidence":        relation.Confidence,
		"evidence":          relation.Evidence,
		"evidence_chunk_id": relation.EvidenceChunkID,
		"document_id":       relation.DocumentID,
		"created_at":        relation.CreatedAt.UnixMilli(),
		"updated_at":        relation.UpdatedAt.UnixMilli(),
	})
	return err
}

func (s *Neo4jStore) SaveCommunity(ctx context.Context, community *model.Community) error {
	if community == nil {
		return nil
	}
	if community.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		community.ID = id
	}
	now := time.Now()
	if community.CreatedAt.IsZero() {
		community.CreatedAt = now
	}
	community.UpdatedAt = now
	_, err := s.execute(ctx, `
MERGE (c:Community {id: $id})
ON CREATE SET c.created_at = $created_at
SET c.owner_id = $owner_id,
    c.name = $name,
    c.summary = $summary,
    c.key_entities_json = $key_entities_json,
    c.level = $level,
    c.updated_at = $updated_at`, map[string]any{
		"id":                community.ID,
		"owner_id":          community.OwnerID,
		"name":              community.Name,
		"summary":           community.Summary,
		"key_entities_json": community.KeyEntitiesJSON,
		"level":             community.Level,
		"created_at":        community.CreatedAt.UnixMilli(),
		"updated_at":        community.UpdatedAt.UnixMilli(),
	})
	return err
}

func (s *Neo4jStore) ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error) {
	result, err := s.execute(ctx, `
MATCH (e:Entity)
WHERE e.owner_id = $owner_id
WITH collect(DISTINCT e) AS entities
OPTIONAL MATCH (source:Entity)-[r:GRAPH_RELATION]->(target:Entity)
WHERE r.owner_id = $owner_id
RETURN entities, collect(DISTINCT {r: r, source_id: source.id, target_id: target.id}) AS relations`, map[string]any{"owner_id": ownerID})
	if err != nil || len(result.Records) == 0 {
		return nil, nil, err
	}
	entities, err := entitiesFromRecord(result.Records[0], "entities")
	if err != nil {
		return nil, nil, err
	}
	relations, err := relationsFromRecord(result.Records[0], "relations")
	if err != nil {
		return nil, nil, err
	}
	return entities, relations, nil
}

func (s *Neo4jStore) ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error {
	if _, err := s.execute(ctx, `MATCH (c:Community {owner_id: $owner_id}) DETACH DELETE c`, map[string]any{"owner_id": ownerID}); err != nil {
		return err
	}
	for i := range communities {
		communities[i].OwnerID = ownerID
		if err := s.SaveCommunity(ctx, &communities[i]); err != nil {
			return err
		}
	}
	rows := make([]map[string]any, 0, len(entityCommunity))
	for entityID, communityID := range entityCommunity {
		rows = append(rows, map[string]any{"entity_id": entityID, "community_id": communityID})
	}
	_, err := s.execute(ctx, `
MATCH (e:Entity {owner_id: $owner_id})
SET e.community_id = 0
WITH count(e) AS _
UNWIND $rows AS row
MATCH (e:Entity {owner_id: $owner_id, id: row.entity_id})
SET e.community_id = row.community_id`, map[string]any{"owner_id": ownerID, "rows": rows})
	return err
}

func (s *Neo4jStore) ListGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	limit, hops = normalizeGraphLimits(limit, hops)
	if documentID > 0 {
		return s.listDocumentGraph(ctx, viewerID, query, limit, documentID, hops)
	}
	result, err := s.execute(ctx, `
MATCH (seed:Entity)
WHERE (seed.owner_id = $viewer_id OR seed.owner_id = 0)
  AND ($query = '' OR toLower(coalesce(seed.name, '') + ' ' + coalesce(seed.summary, '') + ' ' + coalesce(seed.aliases_json, '') + ' ' + coalesce(seed.canonical_key, '')) CONTAINS $query OR coalesce(seed.canonical_key, '') CONTAINS $canonical_query)
WITH seed ORDER BY seed.score DESC, seed.updated_at DESC LIMIT $limit
OPTIONAL MATCH (seed)-[out:GRAPH_RELATION]-(neighbor:Entity)
WHERE out.owner_id = $viewer_id OR out.owner_id = 0
WITH collect(DISTINCT seed) + collect(DISTINCT neighbor) AS entityRows, collect(DISTINCT {r: out, source_id: startNode(out).id, target_id: endNode(out).id}) AS relationRows
WITH [e IN entityRows WHERE e IS NOT NULL] AS entities, [r IN relationRows WHERE r.r IS NOT NULL] AS relations
RETURN entities, relations`, map[string]any{
		"viewer_id":       viewerID,
		"query":           strings.ToLower(strings.TrimSpace(query)),
		"canonical_query": normalizeGraphQueryKey(query),
		"limit":           limit,
	})
	if err != nil || len(result.Records) == 0 {
		return nil, nil, nil, err
	}
	entities, relations, err := graphRowsFromRecord(result.Records[0])
	if err != nil {
		return nil, nil, nil, err
	}
	communities, err := s.communitiesForEntities(ctx, entities)
	return sortEntities(entities), relations, communities, err
}

func (s *Neo4jStore) DeleteDocumentGraph(ctx context.Context, ownerID, documentID int64) error {
	if _, err := s.execute(ctx, `
MATCH ()-[r:GRAPH_RELATION {owner_id: $owner_id, document_id: $document_id}]->()
DELETE r`, map[string]any{"owner_id": ownerID, "document_id": documentID}); err != nil {
		return err
	}
	_, err := s.execute(ctx, `
MATCH (e:Entity {owner_id: $owner_id})
WHERE NOT (e)--(:Entity)
DETACH DELETE e
WITH count(e) AS _
MATCH (c:Community {owner_id: $owner_id})
DETACH DELETE c`, map[string]any{"owner_id": ownerID})
	return err
}

func (s *Neo4jStore) listDocumentGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	result, err := s.execute(ctx, `
MATCH (source:Entity)-[r:GRAPH_RELATION {document_id: $document_id}]->(target:Entity)
WHERE (r.owner_id = $viewer_id OR r.owner_id = 0)
  AND ($query = '' OR toLower(coalesce(source.name, '') + ' ' + coalesce(source.summary, '') + ' ' + coalesce(source.aliases_json, '') + ' ' + coalesce(source.canonical_key, '') + ' ' + coalesce(target.name, '') + ' ' + coalesce(target.summary, '') + ' ' + coalesce(target.aliases_json, '') + ' ' + coalesce(target.canonical_key, '') + ' ' + coalesce(r.description, '') + ' ' + coalesce(r.evidence, '')) CONTAINS $query OR coalesce(source.canonical_key, '') CONTAINS $canonical_query OR coalesce(target.canonical_key, '') CONTAINS $canonical_query)
WITH collect(DISTINCT source) + collect(DISTINCT target) AS seedEntities,
     collect(DISTINCT {r: r, source_id: source.id, target_id: target.id}) AS seedRelations
WITH seedEntities, seedRelations, [e IN seedEntities | e.id] AS ids
OPTIONAL MATCH (neighborSource:Entity)-[nr:GRAPH_RELATION]-(neighborTarget:Entity)
WHERE $hops > 1
  AND (nr.owner_id = $viewer_id OR nr.owner_id = 0)
  AND nr.document_id <> $document_id
  AND (neighborSource.id IN ids OR neighborTarget.id IN ids)
WITH seedEntities, seedRelations,
     collect(DISTINCT neighborSource) + collect(DISTINCT neighborTarget) AS neighborEntities,
     collect(DISTINCT {r: nr, source_id: startNode(nr).id, target_id: endNode(nr).id}) AS neighborRelations
WITH seedEntities + neighborEntities AS entityRows,
     seedRelations + neighborRelations AS relationRows
WITH [e IN entityRows WHERE e IS NOT NULL] AS entities, [r IN relationRows WHERE r.r IS NOT NULL] AS relations
RETURN entities, relations`, map[string]any{
		"viewer_id":       viewerID,
		"document_id":     documentID,
		"query":           strings.ToLower(strings.TrimSpace(query)),
		"canonical_query": normalizeGraphQueryKey(query),
		"limit":           limit,
		"hops":            hops,
	})
	if err != nil || len(result.Records) == 0 {
		return nil, nil, nil, err
	}
	entities, relations, err := graphRowsFromRecord(result.Records[0])
	if err != nil {
		return nil, nil, nil, err
	}
	if len(relations) > limit*4 {
		relations = relations[:limit*4]
	}
	communities, err := s.communitiesForEntities(ctx, entities)
	return sortEntities(entities), relations, communities, err
}

func (s *Neo4jStore) communitiesForEntities(ctx context.Context, entities []model.Entity) ([]model.Community, error) {
	ids := make([]int64, 0, len(entities))
	seen := map[int64]bool{}
	for _, entity := range entities {
		if entity.CommunityID > 0 && !seen[entity.CommunityID] {
			seen[entity.CommunityID] = true
			ids = append(ids, entity.CommunityID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	result, err := s.execute(ctx, `
MATCH (c:Community)
WHERE c.id IN $ids
RETURN collect(c) AS communities`, map[string]any{"ids": ids})
	if err != nil || len(result.Records) == 0 {
		return nil, err
	}
	return communitiesFromRecord(result.Records[0], "communities")
}

func (s *Neo4jStore) execute(ctx context.Context, cypher string, params map[string]any) (*neo4j.EagerResult, error) {
	if s == nil || s.driver == nil {
		return nil, fmt.Errorf("neo4j driver未配置")
	}
	configurers := []neo4j.ExecuteQueryConfigurationOption{
		neo4j.ExecuteQueryWithReadersRouting(),
	}
	if isWriteQuery(cypher) {
		configurers = []neo4j.ExecuteQueryConfigurationOption{
			neo4j.ExecuteQueryWithWritersRouting(),
		}
	}
	if s.database != "" {
		configurers = append(configurers, neo4j.ExecuteQueryWithDatabase(s.database))
	}
	return neo4j.ExecuteQuery(ctx, s.driver, cypher, params, neo4j.EagerResultTransformer, configurers...)
}

func isWriteQuery(cypher string) bool {
	cypher = strings.TrimSpace(strings.ToUpper(cypher))
	writeTokens := []string{"CREATE", "MERGE", "SET", "DELETE", "DETACH DELETE", "REMOVE", "DROP"}
	for _, token := range writeTokens {
		if strings.HasPrefix(cypher, token+" ") || strings.Contains(cypher, "\n"+token+" ") || strings.Contains(cypher, " "+token+" ") {
			return true
		}
	}
	return false
}

func graphRowsFromRecord(record *neo4j.Record) ([]model.Entity, []model.Relation, error) {
	entities, err := entitiesFromRecord(record, "entities")
	if err != nil {
		return nil, nil, err
	}
	relations, err := relationsFromRecord(record, "relations")
	if err != nil {
		return nil, nil, err
	}
	return uniqueEntities(entities), uniqueRelations(relations), nil
}

func entitiesFromRecord(record *neo4j.Record, key string) ([]model.Entity, error) {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return nil, nil
	}
	nodes, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is %T, want []any", key, value)
	}
	entities := make([]model.Entity, 0, len(nodes))
	for _, item := range nodes {
		node, ok := item.(neo4j.Node)
		if !ok {
			continue
		}
		entities = append(entities, entityFromProps(node.Props))
	}
	return entities, nil
}

func communitiesFromRecord(record *neo4j.Record, key string) ([]model.Community, error) {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return nil, nil
	}
	nodes, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is %T, want []any", key, value)
	}
	communities := make([]model.Community, 0, len(nodes))
	for _, item := range nodes {
		node, ok := item.(neo4j.Node)
		if !ok {
			continue
		}
		communities = append(communities, communityFromProps(node.Props))
	}
	return communities, nil
}

func relationsFromRecord(record *neo4j.Record, key string) ([]model.Relation, error) {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is %T, want []any", key, value)
	}
	relations := make([]model.Relation, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok || row["r"] == nil {
			continue
		}
		rel, ok := row["r"].(neo4j.Relationship)
		if !ok {
			continue
		}
		relation := relationFromProps(rel.Props)
		relation.SourceID = asInt64(row["source_id"])
		relation.TargetID = asInt64(row["target_id"])
		relations = append(relations, relation)
	}
	return relations, nil
}

func entityFromProps(props map[string]any) model.Entity {
	return model.Entity{
		ID:           asInt64(props["id"]),
		OwnerID:      asInt64(props["owner_id"]),
		Name:         asString(props["name"]),
		CanonicalKey: asString(props["canonical_key"]),
		Type:         asString(props["type"]),
		Summary:      asString(props["summary"]),
		AliasesJSON:  asString(props["aliases_json"]),
		CommunityID:  asInt64(props["community_id"]),
		Score:        asFloat64(props["score"]),
		CreatedAt:    timeFromMillis(props["created_at"]),
		UpdatedAt:    timeFromMillis(props["updated_at"]),
	}
}

func relationFromProps(props map[string]any) model.Relation {
	return model.Relation{
		ID:              asInt64(props["id"]),
		OwnerID:         asInt64(props["owner_id"]),
		Relation:        asString(props["relation"]),
		Description:     asString(props["description"]),
		Weight:          asFloat64(props["weight"]),
		Confidence:      asFloat64(props["confidence"]),
		Evidence:        asString(props["evidence"]),
		EvidenceChunkID: asInt64(props["evidence_chunk_id"]),
		DocumentID:      asInt64(props["document_id"]),
		CreatedAt:       timeFromMillis(props["created_at"]),
		UpdatedAt:       timeFromMillis(props["updated_at"]),
	}
}

func communityFromProps(props map[string]any) model.Community {
	return model.Community{
		ID:              asInt64(props["id"]),
		OwnerID:         asInt64(props["owner_id"]),
		Name:            asString(props["name"]),
		Summary:         asString(props["summary"]),
		KeyEntitiesJSON: asString(props["key_entities_json"]),
		Level:           asInt64(props["level"]),
		CreatedAt:       timeFromMillis(props["created_at"]),
		UpdatedAt:       timeFromMillis(props["updated_at"]),
	}
}

func uniqueEntities(in []model.Entity) []model.Entity {
	seen := map[int64]bool{}
	out := make([]model.Entity, 0, len(in))
	for _, entity := range in {
		if entity.ID == 0 || seen[entity.ID] {
			continue
		}
		seen[entity.ID] = true
		out = append(out, entity)
	}
	return out
}

func uniqueRelations(in []model.Relation) []model.Relation {
	seen := map[int64]bool{}
	out := make([]model.Relation, 0, len(in))
	for _, relation := range in {
		if relation.ID == 0 || seen[relation.ID] {
			continue
		}
		seen[relation.ID] = true
		out = append(out, relation)
	}
	return out
}

func sortEntities(in []model.Entity) []model.Entity {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Score == in[j].Score {
			return in[i].UpdatedAt.After(in[j].UpdatedAt)
		}
		return in[i].Score > in[j].Score
	})
	return in
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func asInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func asFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func timeFromMillis(value any) time.Time {
	millis := asInt64(value)
	if millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis)
}
