// Package knowledgeclient 定义知识图谱可视化服务的稳定调用契约。
//
// 本包既提供 api-gateway 调用 knowledge-service 的 RPCClient，也提供
// knowledge-service 内部复用的图谱视图模型。这样网关只依赖稳定客户端契约，
// 不需要直接 import knowledge-service 的 internal 包。
package knowledgeclient

import "context"

// GraphQuery 描述知识图谱页面的查询、过滤和邻居扩展条件。
type GraphQuery struct {
	Query           string
	TypeFilters     []string
	RelationFilters []string
	CommunityID     int64
	Hops            int
	Limit           int
}

// GraphInput 是底层 GraphRAG 数据源支持的最小查询条件。
type GraphInput struct {
	Query string
	Limit int
}

// GraphView 是前端可视化画布使用的完整图谱视图。
type GraphView struct {
	Success     bool             `json:"success"`
	Nodes       []GraphNode      `json:"nodes"`
	Edges       []GraphEdge      `json:"edges"`
	Communities []GraphCommunity `json:"communities"`
	Stats       GraphStats       `json:"stats"`
	Msg         string           `json:"msg,omitempty"`
}

// GraphNode 是可视化节点。Size、Color、Degree 是 knowledge-service 计算出的展示属性。
type GraphNode struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Summary     string  `json:"summary"`
	CommunityID int64   `json:"community_id"`
	Score       float64 `json:"score"`
	Degree      int     `json:"degree"`
	Size        float64 `json:"size"`
	Color       string  `json:"color"`
}

// GraphEdge 是可视化关系边。Evidence 来自 GraphRAG 的原文证据片段。
type GraphEdge struct {
	ID          int64   `json:"id"`
	SourceID    int64   `json:"source_id"`
	TargetID    int64   `json:"target_id"`
	Relation    string  `json:"relation"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	Evidence    string  `json:"evidence"`
	Color       string  `json:"color"`
}

// GraphCommunity 是社区摘要，用于图例和社区过滤。
type GraphCommunity struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Level   int64  `json:"level"`
	Color   string `json:"color"`
}

// GraphStats 汇总当前视图规模，供前端顶部状态栏展示。
type GraphStats struct {
	NodeCount      int      `json:"node_count"`
	EdgeCount      int      `json:"edge_count"`
	CommunityCount int      `json:"community_count"`
	Types          []string `json:"types"`
	Relations      []string `json:"relations"`
}

// NodeDetail 是点击节点后的右侧详情面板数据。
type NodeDetail struct {
	Success   bool        `json:"success"`
	Node      GraphNode   `json:"node"`
	Relations []GraphEdge `json:"relations"`
	Neighbors []GraphNode `json:"neighbors"`
	Msg       string      `json:"msg,omitempty"`
}

// EdgeDetail 是点击关系边后的右侧详情面板数据。
type EdgeDetail struct {
	Success bool       `json:"success"`
	Edge    GraphEdge  `json:"edge"`
	Source  *GraphNode `json:"source,omitempty"`
	Target  *GraphNode `json:"target,omitempty"`
	Msg     string     `json:"msg,omitempty"`
}

// PathDetail 是两个实体之间的最短路径结果，用于前端路径高亮和关系解释。
type PathDetail struct {
	Success bool        `json:"success"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
	NodeIDs []int64     `json:"node_ids"`
	EdgeIDs []int64     `json:"edge_ids"`
	Msg     string      `json:"msg,omitempty"`
}

// GraphReviewCandidate 是图谱候选审核工作台展示的数据。
type GraphReviewCandidate struct {
	ID         int64  `json:"id"`
	ItemType   string `json:"item_type"`
	ItemID     int64  `json:"item_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Summary    string `json:"summary"`
	Evidence   string `json:"evidence"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	ReviewNote string `json:"review_note"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	ReviewedAt string `json:"reviewed_at"`
}

// CreateGraphReviewCandidateInput 描述一次候选提交。
type CreateGraphReviewCandidateInput struct {
	ItemType string
	ItemID   int64
	Reason   string
	Query    string
}

// ListGraphReviewCandidatesInput 描述候选审核列表过滤条件。
type ListGraphReviewCandidatesInput struct {
	Status   string
	ItemType string
	Limit    int
	Offset   int
}

// ReviewGraphCandidateInput 描述审核动作。
type ReviewGraphCandidateInput struct {
	CandidateID int64
	Action      string
	Note        string
}

// GraphReviewCandidateList 是审核列表响应。
type GraphReviewCandidateList struct {
	Success    bool                   `json:"success"`
	Candidates []GraphReviewCandidate `json:"candidates"`
	Total      int64                  `json:"total"`
	Msg        string                 `json:"msg,omitempty"`
}

// Service 是 api-gateway 和 knowledge-service RPC 客户端共同遵守的接口。
type Service interface {
	GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error)
	GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error)
	GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error)
	GetNeighborhood(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*GraphView, error)
	GetPath(ctx context.Context, viewerID, sourceID, targetID int64, input GraphQuery) (*PathDetail, error)
	CreateGraphReviewCandidate(ctx context.Context, viewerID int64, input CreateGraphReviewCandidateInput) (*GraphReviewCandidate, error)
	ListGraphReviewCandidates(ctx context.Context, viewerID int64, input ListGraphReviewCandidatesInput) (*GraphReviewCandidateList, error)
	ReviewGraphCandidate(ctx context.Context, viewerID int64, input ReviewGraphCandidateInput) (*GraphReviewCandidate, error)
}
