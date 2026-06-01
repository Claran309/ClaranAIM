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

// Service 是 api-gateway 和 knowledge-service RPC 客户端共同遵守的接口。
type Service interface {
	GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error)
	GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error)
	GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error)
}
