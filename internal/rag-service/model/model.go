// Package model 定义 RAG 文档、检索分块和 GraphRAG 轻量知识图谱模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

const (
	VisibilityPrivate = "private"
	VisibilityShared  = "shared"
	VisibilityPublic  = "public"

	DocumentStatusReady = "ready"

	ChunkLevelParent = "parent"
	ChunkLevelChild  = "child"
)

// Document 保存一份可被检索的知识源。
type Document struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID        int64          `json:"owner_id" gorm:"index;not null"`
	Title          string         `json:"title" gorm:"size:255;not null"`
	Source         string         `json:"source" gorm:"size:500"`
	SourceType     string         `json:"source_type" gorm:"size:64;index"`
	Visibility     string         `json:"visibility" gorm:"size:32;index;not null;default:private"`
	GroupID        int64          `json:"group_id" gorm:"index"`
	ConversationID int64          `json:"conversation_id" gorm:"index"`
	Status         string         `json:"status" gorm:"size:32;index;not null;default:ready"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Document) TableName() string { return "rag_documents" }

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	if d.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	d.ID = id
	return err
}

// Chunk 是 Document 的检索单元，同时保存稀疏关键词和 embedding 引用。
type Chunk struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	DocumentID     int64          `json:"document_id" gorm:"index;not null"`
	ParentChunkID  int64          `json:"parent_chunk_id" gorm:"index;not null;default:0"`
	ChunkLevel     string         `json:"chunk_level" gorm:"size:32;index;not null;default:child"`
	OwnerID        int64          `json:"owner_id" gorm:"index;not null"`
	GroupID        int64          `json:"group_id" gorm:"index"`
	ConversationID int64          `json:"conversation_id" gorm:"index"`
	ChunkIndex     int            `json:"chunk_index" gorm:"index"`
	Content        string         `json:"content" gorm:"type:mediumtext;not null"`
	Summary        string         `json:"summary" gorm:"type:text"`
	Keywords       string         `json:"keywords" gorm:"type:text"`
	EmbeddingRef   string         `json:"embedding_ref" gorm:"size:255;index"`
	QualityScore   float64        `json:"quality_score" gorm:"not null;default:0"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Chunk) TableName() string { return "rag_chunks" }

func (c *Chunk) BeforeCreate(tx *gorm.DB) error {
	if c.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	c.ID = id
	return err
}

// Entity 是 GraphRAG 中的知识图谱节点。
// 当前使用 MySQL 事实表保存实体、别名、类型、摘要和社区归属；在线查询由 rag-service/knowledge-service 读取，
// 后续如果需要更复杂的图遍历，可以把这组事实表迁移到专用图数据库。
type Entity struct {
	ID           int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID      int64          `json:"owner_id" gorm:"index;not null"`
	Name         string         `json:"name" gorm:"size:255;index;not null"`
	CanonicalKey string         `json:"canonical_key" gorm:"size:255;index"`
	Type         string         `json:"type" gorm:"size:64;index"`
	Summary      string         `json:"summary" gorm:"type:text"`
	AliasesJSON  string         `json:"aliases_json" gorm:"type:text"`
	CommunityID  int64          `json:"community_id" gorm:"index"`
	Score        float64        `json:"score" gorm:"not null;default:0"`
	CreatedAt    time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Entity) TableName() string { return "rag_entities" }

func (e *Entity) BeforeCreate(tx *gorm.DB) error {
	if e.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	e.ID = id
	return err
}

// Relation 保存两个实体之间的有向关系和证据文本。
type Relation struct {
	ID              int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID         int64          `json:"owner_id" gorm:"index;not null"`
	SourceID        int64          `json:"source_id" gorm:"index;not null"`
	TargetID        int64          `json:"target_id" gorm:"index;not null"`
	Relation        string         `json:"relation" gorm:"size:128;index;not null"`
	Description     string         `json:"description" gorm:"type:text"`
	Weight          float64        `json:"weight" gorm:"not null;default:1"`
	Confidence      float64        `json:"confidence" gorm:"not null;default:0"`
	Evidence        string         `json:"evidence" gorm:"type:text"`
	EvidenceChunkID int64          `json:"evidence_chunk_id" gorm:"index"`
	DocumentID      int64          `json:"document_id" gorm:"index"`
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Relation) TableName() string { return "rag_relations" }

func (r *Relation) BeforeCreate(tx *gorm.DB) error {
	if r.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	r.ID = id
	return err
}

// Community 是 GraphRAG 社区摘要。
// rag-service 会基于实体关系做 Leiden 风格社区划分，并用 LLM 或本地规则生成社区标题与摘要。
type Community struct {
	ID              int64          `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OwnerID         int64          `json:"owner_id" gorm:"index;not null"`
	Name            string         `json:"name" gorm:"size:255;not null"`
	Summary         string         `json:"summary" gorm:"type:text"`
	KeyEntitiesJSON string         `json:"key_entities_json" gorm:"type:text"`
	Level           int64          `json:"level" gorm:"not null;default:1"`
	CreatedAt       time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Community) TableName() string { return "rag_communities" }

func (c *Community) BeforeCreate(tx *gorm.DB) error {
	if c.ID != 0 {
		return nil
	}
	id, err := idgen.NextID()
	c.ID = id
	return err
}
