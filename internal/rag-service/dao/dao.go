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
	Limit          int
	Offset         int
}

// Repository 定义 rag-service 所需的持久化能力。
type Repository interface {
	CreateDocumentWithChunks(ctx context.Context, doc *model.Document, chunks []model.Chunk) error
	ListDocuments(ctx context.Context, filter SearchFilter) ([]model.Document, int64, error)
	ListChunks(ctx context.Context, filter SearchFilter) ([]ChunkWithDocument, error)
	GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error)
	GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error)
	SaveEntity(ctx context.Context, entity *model.Entity) error
	SaveRelation(ctx context.Context, relation *model.Relation) error
	SaveCommunity(ctx context.Context, community *model.Community) error
	ListGraph(ctx context.Context, viewerID int64, query string, limit int) ([]model.Entity, []model.Relation, []model.Community, error)
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

// ListChunks 读取可见文档下的候选分块，service 层再做 Hybrid/Rerank 排序。
func (r *repositoryImpl) ListChunks(ctx context.Context, filter SearchFilter) ([]ChunkWithDocument, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
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

// ListGraph 返回当前用户可见的轻量知识图谱。
func (r *repositoryImpl) ListGraph(ctx context.Context, viewerID int64, queryText string, limit int) ([]model.Entity, []model.Relation, []model.Community, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
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
