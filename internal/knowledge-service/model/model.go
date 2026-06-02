// Package model 保存 knowledge-service 自己拥有的治理数据模型。
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

const (
	CandidateStatusPending  = "pending"
	CandidateStatusApproved = "approved"
	CandidateStatusRejected = "rejected"
)

// GraphReviewCandidate 是知识图谱候选审核记录。
//
// GraphRAG 的实体和关系事实仍由 rag-service 维护；这里保存的是“某个用户认为某个
// 节点或关系需要被审核”的治理视图。这样可以先形成审核工作台闭环，后续再把审核
// 结果接入实体合并、关系删除或人工修订流程。
type GraphReviewCandidate struct {
	ID          int64      `json:"id" gorm:"primaryKey"`
	OwnerID     int64      `json:"owner_id" gorm:"index;not null"`
	ItemType    string     `json:"item_type" gorm:"size:32;index;not null"`
	ItemID      int64      `json:"item_id" gorm:"index;not null"`
	Name        string     `json:"name" gorm:"size:255"`
	Type        string     `json:"type" gorm:"size:64"`
	Summary     string     `json:"summary" gorm:"type:text"`
	Evidence    string     `json:"evidence" gorm:"type:text"`
	Reason      string     `json:"reason" gorm:"type:text"`
	Status      string     `json:"status" gorm:"size:32;index;not null;default:pending"`
	ReviewerID  int64      `json:"reviewer_id" gorm:"index"`
	ReviewNote  string     `json:"review_note" gorm:"type:text"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (GraphReviewCandidate) TableName() string {
	return "knowledge_graph_review_candidates"
}

func (c *GraphReviewCandidate) BeforeCreate(tx *gorm.DB) error {
	if c.ID == 0 {
		id, err := idgen.NextID()
		if err != nil {
			return err
		}
		c.ID = id
	}
	if c.Status == "" {
		c.Status = CandidateStatusPending
	}
	return nil
}
