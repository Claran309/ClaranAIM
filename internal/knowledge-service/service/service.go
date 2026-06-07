// Package service 实现 knowledge-service 的知识图谱查询与可视化视图逻辑。
//
// 本服务不负责知识入库、embedding、RAG 检索或 GraphRAG indexing；这些仍由 rag-service
// 负责。knowledge-service 只读取底层图谱数据，整理成前端可视化需要的节点、边、社区、
// 详情和过滤 facet。Kitex handler 调用这里的接口，具体 GraphRAG 子图读取通过内部
// GraphSource 适配 rag-service RPC。
package service

import (
	"ClaranAIM/internal/knowledge-service/dao"
	"ClaranAIM/internal/knowledge-service/model"
	"context"
	"errors"
	"strings"
	"time"
)

// KnowledgeService 是 knowledge-service 的业务接口。
type KnowledgeService interface {
	GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error)
	GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error)
	GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error)
	GetNeighborhood(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*GraphView, error)
	GetPath(ctx context.Context, viewerID, sourceID, targetID int64, input GraphQuery) (*PathDetail, error)
	CreateGraphReviewCandidate(ctx context.Context, viewerID int64, input CreateGraphReviewCandidateInput) (*GraphReviewCandidate, error)
	ListGraphReviewCandidates(ctx context.Context, viewerID int64, input ListGraphReviewCandidatesInput) (*GraphReviewCandidateList, error)
	ReviewGraphCandidate(ctx context.Context, viewerID int64, input ReviewGraphCandidateInput) (*GraphReviewCandidate, error)
}

type knowledgeServiceImpl struct {
	graph Service
	repo  dao.Repository
}

// NewKnowledgeService 创建知识图谱视图服务。
func NewKnowledgeService(source GraphSource, repo dao.Repository) KnowledgeService {
	return &knowledgeServiceImpl{graph: NewRAGBackedService(source), repo: repo}
}

func (s *knowledgeServiceImpl) GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error) {
	return s.graph.GetGraphView(ctx, viewerID, input)
}

func (s *knowledgeServiceImpl) GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error) {
	return s.graph.GetNodeDetail(ctx, viewerID, nodeID, input)
}

func (s *knowledgeServiceImpl) GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error) {
	return s.graph.GetEdgeDetail(ctx, viewerID, edgeID, input)
}

func (s *knowledgeServiceImpl) GetNeighborhood(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*GraphView, error) {
	return s.graph.GetNeighborhood(ctx, viewerID, nodeID, input)
}

func (s *knowledgeServiceImpl) GetPath(ctx context.Context, viewerID, sourceID, targetID int64, input GraphQuery) (*PathDetail, error) {
	return s.graph.GetPath(ctx, viewerID, sourceID, targetID, input)
}

func (s *knowledgeServiceImpl) CreateGraphReviewCandidate(ctx context.Context, viewerID int64, input CreateGraphReviewCandidateInput) (*GraphReviewCandidate, error) {
	if s.repo == nil {
		return nil, errors.New("知识图谱审核仓储未初始化")
	}
	if viewerID <= 0 {
		return nil, errors.New("用户未登录")
	}
	itemType := strings.ToLower(strings.TrimSpace(input.ItemType))
	if itemType != "node" && itemType != "edge" {
		return nil, errors.New("item_type只能是node或edge")
	}
	if input.ItemID <= 0 {
		return nil, errors.New("item_id不能为空")
	}
	candidate := &model.GraphReviewCandidate{
		OwnerID:  viewerID,
		ItemType: itemType,
		ItemID:   input.ItemID,
		Reason:   strings.TrimSpace(input.Reason),
		Status:   model.CandidateStatusPending,
	}
	if itemType == "node" {
		detail, err := s.graph.GetNodeDetail(ctx, viewerID, input.ItemID, GraphQuery{Query: input.Query, Limit: 200})
		if err != nil {
			return nil, err
		}
		if detail == nil || !detail.Success {
			return nil, errors.New(defaultMsg(detailMessage(detail), "节点不存在或不可见"))
		}
		candidate.Name = detail.Node.Name
		candidate.Type = detail.Node.Type
		candidate.Summary = detail.Node.Summary
	} else {
		detail, err := s.graph.GetEdgeDetail(ctx, viewerID, input.ItemID, GraphQuery{Query: input.Query, Limit: 200})
		if err != nil {
			return nil, err
		}
		if detail == nil || !detail.Success {
			return nil, errors.New(defaultMsg(edgeDetailMessage(detail), "关系不存在或不可见"))
		}
		candidate.Name = detail.Edge.Relation
		candidate.Type = "Relationship"
		candidate.Summary = detail.Edge.Description
		candidate.Evidence = detail.Edge.Evidence
	}
	if candidate.Reason == "" {
		candidate.Reason = "人工提交审核"
	}
	if err := s.repo.SaveCandidate(ctx, candidate); err != nil {
		return nil, err
	}
	return candidateToDTO(candidate), nil
}

func (s *knowledgeServiceImpl) ListGraphReviewCandidates(ctx context.Context, viewerID int64, input ListGraphReviewCandidatesInput) (*GraphReviewCandidateList, error) {
	if s.repo == nil {
		return &GraphReviewCandidateList{Success: false, Msg: "知识图谱审核仓储未初始化"}, nil
	}
	ownerID := viewerID
	if viewerID < 0 {
		return &GraphReviewCandidateList{Success: false, Msg: "用户未登录"}, nil
	}
	rows, total, err := s.repo.ListCandidates(ctx, dao.CandidateFilter{
		OwnerID:  ownerID,
		Status:   strings.TrimSpace(input.Status),
		ItemType: strings.ToLower(strings.TrimSpace(input.ItemType)),
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]GraphReviewCandidate, 0, len(rows))
	for i := range rows {
		out = append(out, *candidateToDTO(&rows[i]))
	}
	return &GraphReviewCandidateList{Success: true, Candidates: out, Total: total}, nil
}

func (s *knowledgeServiceImpl) ReviewGraphCandidate(ctx context.Context, viewerID int64, input ReviewGraphCandidateInput) (*GraphReviewCandidate, error) {
	if s.repo == nil {
		return nil, errors.New("知识图谱审核仓储未初始化")
	}
	if viewerID == 0 {
		return nil, errors.New("用户未登录")
	}
	adminReview := viewerID < 0
	reviewerID := viewerID
	if adminReview {
		reviewerID = -viewerID
	}
	candidate, err := s.repo.GetCandidate(ctx, input.CandidateID)
	if err != nil {
		return nil, err
	}
	if candidate == nil || (!adminReview && candidate.OwnerID != viewerID) {
		return nil, errors.New("候选不存在或不可见")
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	status := ""
	switch action {
	case "approve", "approved":
		status = model.CandidateStatusApproved
	case "reject", "rejected":
		status = model.CandidateStatusRejected
	default:
		return nil, errors.New("action只能是approve或reject")
	}
	now := time.Now()
	if err := s.repo.UpdateCandidateStatus(ctx, candidate.ID, reviewerID, status, strings.TrimSpace(input.Note), now); err != nil {
		return nil, err
	}
	candidate.Status = status
	candidate.ReviewerID = reviewerID
	candidate.ReviewNote = strings.TrimSpace(input.Note)
	candidate.ReviewedAt = &now
	return candidateToDTO(candidate), nil
}

func candidateToDTO(candidate *model.GraphReviewCandidate) *GraphReviewCandidate {
	if candidate == nil {
		return nil
	}
	return &GraphReviewCandidate{
		ID:         candidate.ID,
		ItemType:   candidate.ItemType,
		ItemID:     candidate.ItemID,
		Name:       candidate.Name,
		Type:       candidate.Type,
		Summary:    candidate.Summary,
		Evidence:   candidate.Evidence,
		Reason:     candidate.Reason,
		Status:     candidate.Status,
		ReviewNote: candidate.ReviewNote,
		CreatedAt:  formatTime(candidate.CreatedAt),
		UpdatedAt:  formatTime(candidate.UpdatedAt),
		ReviewedAt: formatOptionalTime(candidate.ReviewedAt),
	}
}

func detailMessage(detail *NodeDetail) string {
	if detail == nil {
		return ""
	}
	return detail.Msg
}

func edgeDetailMessage(detail *EdgeDetail) string {
	if detail == nil {
		return ""
	}
	return detail.Msg
}

func defaultMsg(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
