// Package dao 负责 rag-service 的 MySQL 持久化访问。
package dao

import (
	"ClaranAIM/internal/rag-service/model"
	"context"
	"errors"
	"sort"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitDB 打开 MySQL 并执行非破坏性迁移。
func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.Document{}, &model.Chunk{}, &model.Entity{}, &model.Relation{}, &model.Community{}); err != nil {
		return nil, err
	}
	return db, nil
}

// SearchFilter 描述 RAG 检索的权限和上下文边界。
type SearchFilter struct {
	ViewerID       int64
	GroupID        int64
	ConversationID int64
	DocumentID     int64
	Limit          int
	Offset         int
}

// Repository 定义 rag-service 所需的持久化能力。
type Repository interface {
	CreateDocumentWithChunks(ctx context.Context, doc *model.Document, chunks []model.Chunk) error
	ListDocuments(ctx context.Context, filter SearchFilter) ([]model.Document, int64, error)
	CountChildChunksByDocumentIDs(ctx context.Context, documentIDs []int64) (map[int64]int64, error)
	ListChunks(ctx context.Context, filter SearchFilter) ([]ChunkWithDocument, error)
	GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error)
	GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error)
	SaveEntity(ctx context.Context, entity *model.Entity) error
	SaveRelation(ctx context.Context, relation *model.Relation) error
	SaveCommunity(ctx context.Context, community *model.Community) error
	ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error)
	ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error
	ListGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error)
	DeleteDocument(ctx context.Context, viewerID, documentID int64) error
	DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error
}

// ChunkWithDocument 把 chunk 和所属文档合并给 service 层做权限、排序和来源展示。
type ChunkWithDocument struct {
	Chunk    model.Chunk
	Document model.Document
}

type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository 创建 GORM 仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

// CreateDocumentWithChunks 在同一事务中写入文档和分块。
func (r *repositoryImpl) CreateDocumentWithChunks(ctx context.Context, doc *model.Document, chunks []model.Chunk) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(doc).Error; err != nil {
			return err
		}
		for i := range chunks {
			chunks[i].DocumentID = doc.ID
			chunks[i].OwnerID = doc.OwnerID
			chunks[i].GroupID = doc.GroupID
			chunks[i].ConversationID = doc.ConversationID
			if err := tx.Create(&chunks[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListDocuments 返回当前用户拥有或公共可见的知识文档。
func (r *repositoryImpl) ListDocuments(ctx context.Context, filter SearchFilter) ([]model.Document, int64, error) {
	query := visibleDocuments(r.db.WithContext(ctx).Table("rag_documents").Where("rag_documents.deleted_at IS NULL"), filter.ViewerID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var docs []model.Document
	err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).Find(&docs).Error
	return docs, total, err
}

// CountChildChunksByDocumentIDs 统计每篇文档真正参与检索的 child chunk 数量。
// 前端展示 chunk 数时使用 child chunk，而不是 parent chunk，避免把层级索引的父块误算进去。
func (r *repositoryImpl) CountChildChunksByDocumentIDs(ctx context.Context, documentIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(documentIDs))
	if len(documentIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		DocumentID int64 `gorm:"column:document_id"`
		Count      int64 `gorm:"column:count"`
	}
	err := r.db.WithContext(ctx).Table("rag_chunks").
		Select("document_id, COUNT(*) AS count").
		Where("deleted_at IS NULL AND chunk_level = ? AND document_id IN ?", model.ChunkLevelChild, documentIDs).
		Group("document_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.DocumentID] = row.Count
	}
	return out, nil
}

// ListChunks 读取可见文档下的候选分块，service 层再做 Hybrid/Rerank 排序。
func (r *repositoryImpl) ListChunks(ctx context.Context, filter SearchFilter) ([]ChunkWithDocument, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if filter.DocumentID > 0 && filter.Limit > 500 && filter.Limit <= 1500 {
		limit = filter.Limit
	}
	var rows []struct {
		model.Chunk
		DocID             int64  `gorm:"column:doc_id"`
		DocOwnerID        int64  `gorm:"column:doc_owner_id"`
		DocTitle          string `gorm:"column:doc_title"`
		DocSource         string `gorm:"column:doc_source"`
		DocSourceType     string `gorm:"column:doc_source_type"`
		DocVisibility     string `gorm:"column:doc_visibility"`
		DocGroupID        int64  `gorm:"column:doc_group_id"`
		DocConversationID int64  `gorm:"column:doc_conversation_id"`
		DocStatus         string `gorm:"column:doc_status"`
	}
	query := r.db.WithContext(ctx).Table("rag_chunks").
		Select("rag_chunks.*, rag_documents.id AS doc_id, rag_documents.owner_id AS doc_owner_id, rag_documents.title AS doc_title, rag_documents.source AS doc_source, rag_documents.source_type AS doc_source_type, rag_documents.visibility AS doc_visibility, rag_documents.group_id AS doc_group_id, rag_documents.conversation_id AS doc_conversation_id, rag_documents.status AS doc_status").
		Joins("JOIN rag_documents ON rag_documents.id = rag_chunks.document_id AND rag_documents.deleted_at IS NULL").
		Where("rag_chunks.deleted_at IS NULL")
	query = visibleDocuments(query, filter.ViewerID)
	if filter.GroupID > 0 {
		query = query.Where("(rag_documents.group_id = ? OR rag_documents.group_id = 0)", filter.GroupID)
	}
	if filter.ConversationID > 0 {
		query = query.Where("(rag_documents.conversation_id = ? OR rag_documents.conversation_id = 0)", filter.ConversationID)
	}
	if filter.DocumentID > 0 {
		query = query.Where("rag_documents.id = ?", filter.DocumentID)
	}
	if err := query.Order("rag_chunks.updated_at DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ChunkWithDocument, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChunkWithDocument{
			Chunk: row.Chunk,
			Document: model.Document{
				ID:             row.DocID,
				OwnerID:        row.DocOwnerID,
				Title:          row.DocTitle,
				Source:         row.DocSource,
				SourceType:     row.DocSourceType,
				Visibility:     row.DocVisibility,
				GroupID:        row.DocGroupID,
				ConversationID: row.DocConversationID,
				Status:         row.DocStatus,
			},
		})
	}
	return out, nil
}

// GetEntityByName 在同一 owner 下按名称读取实体。
func (r *repositoryImpl) GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error) {
	var entity model.Entity
	err := r.db.WithContext(ctx).Where("owner_id = ? AND name = ?", ownerID, name).First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &entity, err
}

// GetEntityByCanonicalKey 使用归一化 key 合并同义实体，避免 msg-core-service/msg core service 变成多个节点。
func (r *repositoryImpl) GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error) {
	var entity model.Entity
	err := r.db.WithContext(ctx).Where("owner_id = ? AND canonical_key = ?", ownerID, canonicalKey).First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &entity, err
}

func (r *repositoryImpl) SaveEntity(ctx context.Context, entity *model.Entity) error {
	return r.db.WithContext(ctx).Save(entity).Error
}

func (r *repositoryImpl) SaveRelation(ctx context.Context, relation *model.Relation) error {
	return r.db.WithContext(ctx).Create(relation).Error
}

func (r *repositoryImpl) SaveCommunity(ctx context.Context, community *model.Community) error {
	return r.db.WithContext(ctx).Save(community).Error
}

// ListOwnerGraph 读取某个 owner 的完整图谱，用于 GraphRAG 社区划分。
func (r *repositoryImpl) ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error) {
	var entities []model.Entity
	if err := r.db.WithContext(ctx).Where("owner_id = ?", ownerID).Find(&entities).Error; err != nil {
		return nil, nil, err
	}
	var relations []model.Relation
	if err := r.db.WithContext(ctx).Where("owner_id = ?", ownerID).Find(&relations).Error; err != nil {
		return nil, nil, err
	}
	return entities, relations, nil
}

// ReplaceOwnerCommunities 用一次事务替换 owner 当前的社区摘要和实体归属。
func (r *repositoryImpl) ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner_id = ?", ownerID).Delete(&model.Community{}).Error; err != nil {
			return err
		}
		for i := range communities {
			communities[i].OwnerID = ownerID
			if err := tx.Save(&communities[i]).Error; err != nil {
				return err
			}
		}
		for entityID, communityID := range entityCommunity {
			if err := tx.Model(&model.Entity{}).Where("id = ? AND owner_id = ?", entityID, ownerID).Update("community_id", communityID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListGraph 返回当前用户可见的轻量知识图谱。
func (r *repositoryImpl) ListGraph(ctx context.Context, viewerID int64, queryText string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	if hops <= 0 {
		hops = 1
	}
	if hops > 2 {
		hops = 2
	}
	if documentID > 0 {
		return r.listDocumentScopedGraph(ctx, viewerID, documentID, queryText, limit, hops)
	}
	queryText = strings.TrimSpace(queryText)
	canonical := normalizeGraphQueryKey(queryText)
	query := r.db.WithContext(ctx).Model(&model.Entity{}).Where("owner_id = ? OR owner_id = 0", viewerID)
	if queryText != "" {
		like := "%" + queryText + "%"
		canonicalLike := "%" + canonical + "%"
		query = query.Where("name LIKE ? OR summary LIKE ? OR aliases_json LIKE ? OR canonical_key LIKE ?", like, like, like, canonicalLike)
	}
	var seedEntities []model.Entity
	if err := query.Order("score DESC, updated_at DESC").Limit(limit).Find(&seedEntities).Error; err != nil {
		return nil, nil, nil, err
	}
	seedIDs := make([]int64, 0, len(seedEntities))
	entityByID := make(map[int64]model.Entity, len(seedEntities))
	for _, entity := range seedEntities {
		seedIDs = append(seedIDs, entity.ID)
		entityByID[entity.ID] = entity
	}
	var relations []model.Relation
	if len(seedIDs) > 0 {
		if err := r.db.WithContext(ctx).
			Where("(owner_id = ? OR owner_id = 0) AND (source_id IN ? OR target_id IN ?)", viewerID, seedIDs, seedIDs).
			Limit(limit * 3).
			Find(&relations).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	neighborIDs := make([]int64, 0, len(relations)*2)
	seenNeighbor := map[int64]bool{}
	for _, relation := range relations {
		if _, ok := entityByID[relation.SourceID]; !ok && !seenNeighbor[relation.SourceID] {
			seenNeighbor[relation.SourceID] = true
			neighborIDs = append(neighborIDs, relation.SourceID)
		}
		if _, ok := entityByID[relation.TargetID]; !ok && !seenNeighbor[relation.TargetID] {
			seenNeighbor[relation.TargetID] = true
			neighborIDs = append(neighborIDs, relation.TargetID)
		}
	}
	if len(neighborIDs) > 0 {
		var neighbors []model.Entity
		if err := r.db.WithContext(ctx).Where("id IN ?", neighborIDs).Find(&neighbors).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, entity := range neighbors {
			entityByID[entity.ID] = entity
		}
	}
	entities := make([]model.Entity, 0, len(entityByID))
	communityIDs := make([]int64, 0, len(entityByID))
	for _, entity := range entityByID {
		entities = append(entities, entity)
		if entity.CommunityID > 0 {
			communityIDs = append(communityIDs, entity.CommunityID)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Score == entities[j].Score {
			return entities[i].UpdatedAt.After(entities[j].UpdatedAt)
		}
		return entities[i].Score > entities[j].Score
	})
	var communities []model.Community
	if len(communityIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", communityIDs).Find(&communities).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	return entities, relations, communities, nil
}

// DeleteDocument 删除当前用户拥有的知识文档，同时清理它的分块、图谱关系和孤立实体。
// 这里要求 owner_id 精确匹配 viewerID，避免公共文档被普通可见用户删除。
func (r *repositoryImpl) DeleteDocument(ctx context.Context, viewerID, documentID int64) error {
	if viewerID <= 0 || documentID <= 0 {
		return errors.New("无效的用户或文档ID")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc model.Document
		if err := tx.Where("id = ? AND owner_id = ? AND deleted_at IS NULL", documentID, viewerID).First(&doc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("文档不存在或无权删除")
			}
			return err
		}
		if err := tx.Where("document_id = ? AND owner_id = ?", documentID, viewerID).Delete(&model.Relation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("document_id = ? AND owner_id = ?", documentID, viewerID).Delete(&model.Chunk{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&doc).Error; err != nil {
			return err
		}
		return cleanupOrphanGraphRows(ctx, tx, viewerID)
	})
}

// DeleteDocumentGraph 只删除某篇文档贡献的图谱关系，文档和检索 chunk 保留。
func (r *repositoryImpl) DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error {
	if viewerID <= 0 || documentID <= 0 {
		return errors.New("无效的用户或文档ID")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc model.Document
		if err := tx.Where("id = ? AND owner_id = ? AND deleted_at IS NULL", documentID, viewerID).First(&doc).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("文档不存在或无权删除图谱")
			}
			return err
		}
		if err := tx.Where("document_id = ? AND owner_id = ?", documentID, viewerID).Delete(&model.Relation{}).Error; err != nil {
			return err
		}
		return cleanupOrphanGraphRows(ctx, tx, viewerID)
	})
}

func cleanupOrphanGraphRows(ctx context.Context, tx *gorm.DB, ownerID int64) error {
	var usedRows []struct {
		ID int64 `gorm:"column:id"`
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT source_id AS id FROM rag_relations WHERE owner_id = ? AND deleted_at IS NULL
		UNION
		SELECT target_id AS id FROM rag_relations WHERE owner_id = ? AND deleted_at IS NULL
	`, ownerID, ownerID).Scan(&usedRows).Error; err != nil {
		return err
	}
	usedIDs := make([]int64, 0, len(usedRows))
	for _, row := range usedRows {
		if row.ID > 0 {
			usedIDs = append(usedIDs, row.ID)
		}
	}
	query := tx.WithContext(ctx).Where("owner_id = ?", ownerID)
	if len(usedIDs) > 0 {
		query = query.Where("id NOT IN ?", usedIDs)
	}
	if err := query.Delete(&model.Entity{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Where("owner_id = ?", ownerID).Delete(&model.Community{}).Error
}

func (r *repositoryImpl) listDocumentScopedGraph(ctx context.Context, viewerID, documentID int64, queryText string, limit, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	queryText = strings.TrimSpace(queryText)
	canonical := normalizeGraphQueryKey(queryText)
	relationQuery := r.db.WithContext(ctx).Table("rag_relations").
		Select("rag_relations.*").
		Joins("JOIN rag_documents ON rag_documents.id = rag_relations.document_id AND rag_documents.deleted_at IS NULL").
		Where("rag_relations.deleted_at IS NULL AND rag_relations.document_id = ?", documentID)
	relationQuery = visibleDocuments(relationQuery, viewerID)
	if queryText != "" {
		like := "%" + queryText + "%"
		canonicalLike := "%" + canonical + "%"
		relationQuery = relationQuery.Joins("LEFT JOIN rag_entities src ON src.id = rag_relations.source_id").
			Joins("LEFT JOIN rag_entities dst ON dst.id = rag_relations.target_id").
			Where("src.name LIKE ? OR src.summary LIKE ? OR src.aliases_json LIKE ? OR src.canonical_key LIKE ? OR dst.name LIKE ? OR dst.summary LIKE ? OR dst.aliases_json LIKE ? OR dst.canonical_key LIKE ? OR rag_relations.description LIKE ? OR rag_relations.evidence LIKE ?",
				like, like, like, canonicalLike, like, like, like, canonicalLike, like, like)
	}
	var seedRelations []model.Relation
	if err := relationQuery.Order("rag_relations.weight DESC, rag_relations.updated_at DESC").Limit(limit * 3).Find(&seedRelations).Error; err != nil {
		return nil, nil, nil, err
	}
	entityIDs := map[int64]bool{}
	for _, relation := range seedRelations {
		entityIDs[relation.SourceID] = true
		entityIDs[relation.TargetID] = true
	}
	if len(entityIDs) == 0 {
		return nil, nil, nil, nil
	}
	allRelations := append([]model.Relation{}, seedRelations...)
	if hops > 1 && len(entityIDs) > 0 {
		ids := int64SetToSlice(entityIDs)
		var neighborRelations []model.Relation
		if err := r.db.WithContext(ctx).Where("(owner_id = ? OR owner_id = 0) AND document_id <> ? AND (source_id IN ? OR target_id IN ?)", viewerID, documentID, ids, ids).
			Order("weight DESC, updated_at DESC").
			Limit(limit).
			Find(&neighborRelations).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, relation := range neighborRelations {
			allRelations = append(allRelations, relation)
			entityIDs[relation.SourceID] = true
			entityIDs[relation.TargetID] = true
		}
	}
	if len(entityIDs) == 0 {
		return nil, nil, nil, nil
	}
	var entities []model.Entity
	if err := r.db.WithContext(ctx).Where("id IN ?", int64SetToSlice(entityIDs)).Find(&entities).Error; err != nil {
		return nil, nil, nil, err
	}
	communityIDs := make([]int64, 0, len(entities))
	for _, entity := range entities {
		if entity.CommunityID > 0 {
			communityIDs = append(communityIDs, entity.CommunityID)
		}
	}
	var communities []model.Community
	if len(communityIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", communityIDs).Find(&communities).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Score == entities[j].Score {
			return entities[i].UpdatedAt.After(entities[j].UpdatedAt)
		}
		return entities[i].Score > entities[j].Score
	})
	return entities, allRelations, communities, nil
}

func int64SetToSlice(values map[int64]bool) []int64 {
	out := make([]int64, 0, len(values))
	for value := range values {
		if value > 0 {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func visibleDocuments(query *gorm.DB, viewerID int64) *gorm.DB {
	return query.Where("(rag_documents.owner_id = ? OR rag_documents.visibility = ?)", viewerID, model.VisibilityPublic)
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
