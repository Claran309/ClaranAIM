// Package handler 实现 knowledge-service 的 Kitex RPC 入口。
package handler

import (
	knowledgesvc "ClaranAIM/internal/knowledge-service/service"
	"ClaranAIM/kitex_gen/knowledge"
	"ClaranAIM/pkg/knowledgeclient"
	"context"
)

// KnowledgeServiceImpl 负责把 Kitex 生成 DTO 转换为 knowledge-service 业务 DTO。
// 图谱读取、过滤、邻居聚合和可视化属性计算都放在 service 层，handler 不直接处理业务规则。
type KnowledgeServiceImpl struct {
	svc knowledgesvc.KnowledgeService
}

// NewKnowledgeServiceImpl 创建 knowledge-service 的 Kitex handler。
func NewKnowledgeServiceImpl(svc knowledgesvc.KnowledgeService) knowledge.KnowledgeService {
	return &KnowledgeServiceImpl{svc: svc}
}

// GetGraphView 返回前端知识图谱画布需要的节点、关系、社区和统计信息。
func (h *KnowledgeServiceImpl) GetGraphView(ctx context.Context, req *knowledge.KnowledgeGraphReq) (*knowledge.KnowledgeGraphResp, error) {
	view, err := h.svc.GetGraphView(ctx, req.GetViewerId(), knowledgeclient.GraphQuery{
		Query:           req.GetQuery(),
		TypeFilters:     req.GetTypeFilters(),
		RelationFilters: req.GetRelationFilters(),
		CommunityID:     req.GetCommunityId(),
		Hops:            int(req.GetHops()),
		Limit:           int(req.GetLimit()),
	})
	if err != nil {
		return &knowledge.KnowledgeGraphResp{Success: false, Msg: err.Error()}, nil
	}
	return toRPCGraphView(view), nil
}

// GetNodeDetail 返回节点详情、相邻节点和相关关系。
func (h *KnowledgeServiceImpl) GetNodeDetail(ctx context.Context, req *knowledge.KnowledgeNodeDetailReq) (*knowledge.KnowledgeNodeDetailResp, error) {
	detail, err := h.svc.GetNodeDetail(ctx, req.GetViewerId(), req.GetNodeId(), knowledgeclient.GraphQuery{
		Query: req.GetQuery(),
		Limit: int(req.GetLimit()),
	})
	if err != nil {
		return &knowledge.KnowledgeNodeDetailResp{Success: false, Msg: err.Error()}, nil
	}
	return toRPCNodeDetail(detail), nil
}

// GetEdgeDetail 返回关系详情、两端节点和证据来源。
func (h *KnowledgeServiceImpl) GetEdgeDetail(ctx context.Context, req *knowledge.KnowledgeEdgeDetailReq) (*knowledge.KnowledgeEdgeDetailResp, error) {
	detail, err := h.svc.GetEdgeDetail(ctx, req.GetViewerId(), req.GetEdgeId(), knowledgeclient.GraphQuery{
		Query: req.GetQuery(),
		Limit: int(req.GetLimit()),
	})
	if err != nil {
		return &knowledge.KnowledgeEdgeDetailResp{Success: false, Msg: err.Error()}, nil
	}
	return toRPCEdgeDetail(detail), nil
}

// GetNeighborhood 返回某个实体的一跳或多跳邻域子图。
func (h *KnowledgeServiceImpl) GetNeighborhood(ctx context.Context, req *knowledge.KnowledgeNeighborhoodReq) (*knowledge.KnowledgeGraphResp, error) {
	view, err := h.svc.GetNeighborhood(ctx, req.GetViewerId(), req.GetNodeId(), knowledgeclient.GraphQuery{
		Query:           req.GetQuery(),
		TypeFilters:     req.GetTypeFilters(),
		RelationFilters: req.GetRelationFilters(),
		CommunityID:     req.GetCommunityId(),
		Hops:            int(req.GetHops()),
		Limit:           int(req.GetLimit()),
	})
	if err != nil {
		return &knowledge.KnowledgeGraphResp{Success: false, Msg: err.Error()}, nil
	}
	return toRPCGraphView(view), nil
}

// GetPath 返回两个实体之间的最短可见路径，用于前端高亮和解释链路。
func (h *KnowledgeServiceImpl) GetPath(ctx context.Context, req *knowledge.KnowledgePathReq) (*knowledge.KnowledgePathResp, error) {
	path, err := h.svc.GetPath(ctx, req.GetViewerId(), req.GetSourceId(), req.GetTargetId(), knowledgeclient.GraphQuery{
		Query: req.GetQuery(),
		Limit: int(req.GetLimit()),
	})
	if err != nil {
		return &knowledge.KnowledgePathResp{Success: false, Msg: err.Error()}, nil
	}
	return toRPCPath(path), nil
}

func toRPCGraphView(view *knowledgeclient.GraphView) *knowledge.KnowledgeGraphResp {
	if view == nil {
		return &knowledge.KnowledgeGraphResp{Success: false, Msg: "knowledge-service返回空图谱"}
	}
	return &knowledge.KnowledgeGraphResp{
		Success:     view.Success,
		Nodes:       toRPCNodes(view.Nodes),
		Edges:       toRPCEdges(view.Edges),
		Communities: toRPCCommunities(view.Communities),
		Stats:       toRPCStats(view.Stats),
		Msg:         view.Msg,
	}
}

func toRPCNodeDetail(detail *knowledgeclient.NodeDetail) *knowledge.KnowledgeNodeDetailResp {
	if detail == nil {
		return &knowledge.KnowledgeNodeDetailResp{Success: false, Msg: "knowledge-service返回空节点详情"}
	}
	return &knowledge.KnowledgeNodeDetailResp{
		Success:   detail.Success,
		Node:      toRPCNode(detail.Node),
		Relations: toRPCEdges(detail.Relations),
		Neighbors: toRPCNodes(detail.Neighbors),
		Msg:       detail.Msg,
	}
}

func toRPCEdgeDetail(detail *knowledgeclient.EdgeDetail) *knowledge.KnowledgeEdgeDetailResp {
	if detail == nil {
		return &knowledge.KnowledgeEdgeDetailResp{Success: false, Msg: "knowledge-service返回空关系详情"}
	}
	return &knowledge.KnowledgeEdgeDetailResp{
		Success: detail.Success,
		Edge:    toRPCEdge(detail.Edge),
		Source:  toRPCNodePtr(detail.Source),
		Target:  toRPCNodePtr(detail.Target),
		Msg:     detail.Msg,
	}
}

func toRPCPath(path *knowledgeclient.PathDetail) *knowledge.KnowledgePathResp {
	if path == nil {
		return &knowledge.KnowledgePathResp{Success: false, Msg: "knowledge-service返回空路径"}
	}
	return &knowledge.KnowledgePathResp{
		Success: path.Success,
		Nodes:   toRPCNodes(path.Nodes),
		Edges:   toRPCEdges(path.Edges),
		NodeIds: path.NodeIDs,
		EdgeIds: path.EdgeIDs,
		Msg:     path.Msg,
	}
}

func toRPCNodes(nodes []knowledgeclient.GraphNode) []*knowledge.KnowledgeGraphNode {
	out := make([]*knowledge.KnowledgeGraphNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, toRPCNode(nodes[i]))
	}
	return out
}

func toRPCNodePtr(node *knowledgeclient.GraphNode) *knowledge.KnowledgeGraphNode {
	if node == nil {
		return nil
	}
	return toRPCNode(*node)
}

func toRPCNode(node knowledgeclient.GraphNode) *knowledge.KnowledgeGraphNode {
	return &knowledge.KnowledgeGraphNode{
		Id:          node.ID,
		Name:        node.Name,
		Type:        node.Type,
		Summary:     node.Summary,
		CommunityId: node.CommunityID,
		Score:       node.Score,
		Degree:      int64(node.Degree),
		Size:        node.Size,
		Color:       node.Color,
	}
}

func toRPCEdges(edges []knowledgeclient.GraphEdge) []*knowledge.KnowledgeGraphEdge {
	out := make([]*knowledge.KnowledgeGraphEdge, 0, len(edges))
	for i := range edges {
		out = append(out, toRPCEdge(edges[i]))
	}
	return out
}

func toRPCEdge(edge knowledgeclient.GraphEdge) *knowledge.KnowledgeGraphEdge {
	return &knowledge.KnowledgeGraphEdge{
		Id:          edge.ID,
		SourceId:    edge.SourceID,
		TargetId:    edge.TargetID,
		Relation:    edge.Relation,
		Description: edge.Description,
		Weight:      edge.Weight,
		Evidence:    edge.Evidence,
		Color:       edge.Color,
	}
}

func toRPCCommunities(communities []knowledgeclient.GraphCommunity) []*knowledge.KnowledgeGraphCommunity {
	out := make([]*knowledge.KnowledgeGraphCommunity, 0, len(communities))
	for i := range communities {
		community := communities[i]
		out = append(out, &knowledge.KnowledgeGraphCommunity{
			Id:      community.ID,
			Name:    community.Name,
			Summary: community.Summary,
			Level:   community.Level,
			Color:   community.Color,
		})
	}
	return out
}

func toRPCStats(stats knowledgeclient.GraphStats) *knowledge.KnowledgeGraphStats {
	return &knowledge.KnowledgeGraphStats{
		NodeCount:      int64(stats.NodeCount),
		EdgeCount:      int64(stats.EdgeCount),
		CommunityCount: int64(stats.CommunityCount),
		Types:          stats.Types,
		Relations:      stats.Relations,
	}
}
