package knowledgeclient

import (
	"ClaranAIM/kitex_gen/knowledge"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"context"
	"errors"
)

// RPCClient 使用 Kitex 调用 knowledge-service。
// api-gateway 只依赖本包接口，不直接访问 knowledge-service 的 internal 代码。
type RPCClient struct {
	client knowledgeservice.Client
}

// NewRPCClient 包装已由 api-gateway 启动逻辑创建好的 Kitex 客户端。
func NewRPCClient(client knowledgeservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// GetGraphView 查询前端图谱画布视图。
func (c *RPCClient) GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error) {
	resp, err := c.client.GetGraphView(ctx, &knowledge.KnowledgeGraphReq{
		ViewerId:        viewerID,
		Query:           input.Query,
		TypeFilters:     input.TypeFilters,
		RelationFilters: input.RelationFilters,
		CommunityId:     input.CommunityID,
		Hops:            int64(input.Hops),
		Limit:           int64(input.Limit),
		DocumentId:      input.DocumentID,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return &GraphView{Success: false, Msg: err.Error()}, nil
	}
	return fromRPCGraphView(resp), nil
}

// GetNodeDetail 查询节点详情和邻居关系。
func (c *RPCClient) GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error) {
	resp, err := c.client.GetNodeDetail(ctx, &knowledge.KnowledgeNodeDetailReq{
		ViewerId:   viewerID,
		NodeId:     nodeID,
		Query:      input.Query,
		Limit:      int64(input.Limit),
		DocumentId: input.DocumentID,
		Hops:       int64(input.Hops),
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return &NodeDetail{Success: false, Msg: defaultRPCMsg(resp.GetMsg())}, nil
	}
	return fromRPCNodeDetail(resp), nil
}

// GetEdgeDetail 查询关系详情和两端节点。
func (c *RPCClient) GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error) {
	resp, err := c.client.GetEdgeDetail(ctx, &knowledge.KnowledgeEdgeDetailReq{
		ViewerId:   viewerID,
		EdgeId:     edgeID,
		Query:      input.Query,
		Limit:      int64(input.Limit),
		DocumentId: input.DocumentID,
		Hops:       int64(input.Hops),
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return &EdgeDetail{Success: false, Msg: defaultRPCMsg(resp.GetMsg())}, nil
	}
	return fromRPCEdgeDetail(resp), nil
}

// GetNeighborhood 查询某个节点的一跳或多跳邻域子图。
func (c *RPCClient) GetNeighborhood(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*GraphView, error) {
	resp, err := c.client.GetNeighborhood(ctx, &knowledge.KnowledgeNeighborhoodReq{
		ViewerId:        viewerID,
		NodeId:          nodeID,
		Query:           input.Query,
		TypeFilters:     input.TypeFilters,
		RelationFilters: input.RelationFilters,
		CommunityId:     input.CommunityID,
		Hops:            int64(input.Hops),
		Limit:           int64(input.Limit),
		DocumentId:      input.DocumentID,
	})
	if err != nil {
		return nil, err
	}
	if err := rpcStatus(resp.GetSuccess(), resp.GetMsg()); err != nil {
		return &GraphView{Success: false, Msg: err.Error()}, nil
	}
	return fromRPCGraphView(resp), nil
}

// GetPath 查询两个节点之间的最短可见路径。
func (c *RPCClient) GetPath(ctx context.Context, viewerID, sourceID, targetID int64, input GraphQuery) (*PathDetail, error) {
	resp, err := c.client.GetPath(ctx, &knowledge.KnowledgePathReq{
		ViewerId:   viewerID,
		SourceId:   sourceID,
		TargetId:   targetID,
		Query:      input.Query,
		Limit:      int64(input.Limit),
		DocumentId: input.DocumentID,
		Hops:       int64(input.Hops),
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return &PathDetail{Success: false, Msg: defaultRPCMsg(resp.GetMsg())}, nil
	}
	return fromRPCPath(resp), nil
}

func (c *RPCClient) CreateGraphReviewCandidate(ctx context.Context, viewerID int64, input CreateGraphReviewCandidateInput) (*GraphReviewCandidate, error) {
	resp, err := c.client.CreateGraphReviewCandidate(ctx, &knowledge.CreateGraphReviewCandidateReq{
		ViewerId: viewerID,
		ItemType: input.ItemType,
		ItemId:   input.ItemID,
		Reason:   input.Reason,
		Query:    input.Query,
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, errors.New(defaultRPCMsg(resp.GetMsg()))
	}
	return fromRPCReviewCandidate(resp.GetCandidate()), nil
}

func (c *RPCClient) ListGraphReviewCandidates(ctx context.Context, viewerID int64, input ListGraphReviewCandidatesInput) (*GraphReviewCandidateList, error) {
	resp, err := c.client.ListGraphReviewCandidates(ctx, &knowledge.ListGraphReviewCandidatesReq{
		ViewerId: viewerID,
		Status:   input.Status,
		ItemType: input.ItemType,
		Limit:    int64(input.Limit),
		Offset:   int64(input.Offset),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &GraphReviewCandidateList{Success: false, Msg: "knowledge-service返回空审核列表"}, nil
	}
	return &GraphReviewCandidateList{
		Success:    resp.GetSuccess(),
		Candidates: fromRPCReviewCandidates(resp.GetCandidates()),
		Total:      resp.GetTotal(),
		Msg:        resp.GetMsg(),
	}, nil
}

func (c *RPCClient) ReviewGraphCandidate(ctx context.Context, viewerID int64, input ReviewGraphCandidateInput) (*GraphReviewCandidate, error) {
	resp, err := c.client.ReviewGraphCandidate(ctx, &knowledge.ReviewGraphCandidateReq{
		ViewerId:    viewerID,
		CandidateId: input.CandidateID,
		Action:      input.Action,
		Note:        input.Note,
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, errors.New(defaultRPCMsg(resp.GetMsg()))
	}
	return fromRPCReviewCandidate(resp.GetCandidate()), nil
}

func fromRPCGraphView(resp *knowledge.KnowledgeGraphResp) *GraphView {
	if resp == nil {
		return &GraphView{Success: false, Msg: "knowledge-service返回空响应"}
	}
	return &GraphView{
		Success:     resp.GetSuccess(),
		Nodes:       fromRPCNodes(resp.GetNodes()),
		Edges:       fromRPCEdges(resp.GetEdges()),
		Communities: fromRPCCommunities(resp.GetCommunities()),
		Stats:       fromRPCStats(resp.GetStats()),
		Msg:         resp.GetMsg(),
	}
}

func fromRPCNodeDetail(resp *knowledge.KnowledgeNodeDetailResp) *NodeDetail {
	if resp == nil {
		return &NodeDetail{Success: false, Msg: "knowledge-service返回空节点详情"}
	}
	return &NodeDetail{
		Success:   resp.GetSuccess(),
		Node:      fromRPCNode(resp.GetNode()),
		Relations: fromRPCEdges(resp.GetRelations()),
		Neighbors: fromRPCNodes(resp.GetNeighbors()),
		Msg:       resp.GetMsg(),
	}
}

func fromRPCEdgeDetail(resp *knowledge.KnowledgeEdgeDetailResp) *EdgeDetail {
	if resp == nil {
		return &EdgeDetail{Success: false, Msg: "knowledge-service返回空关系详情"}
	}
	source := fromRPCNode(resp.GetSource())
	target := fromRPCNode(resp.GetTarget())
	return &EdgeDetail{
		Success: resp.GetSuccess(),
		Edge:    fromRPCEdge(resp.GetEdge()),
		Source:  &source,
		Target:  &target,
		Msg:     resp.GetMsg(),
	}
}

func fromRPCPath(resp *knowledge.KnowledgePathResp) *PathDetail {
	if resp == nil {
		return &PathDetail{Success: false, Msg: "knowledge-service返回空路径"}
	}
	return &PathDetail{
		Success: resp.GetSuccess(),
		Nodes:   fromRPCNodes(resp.GetNodes()),
		Edges:   fromRPCEdges(resp.GetEdges()),
		NodeIDs: resp.GetNodeIds(),
		EdgeIDs: resp.GetEdgeIds(),
		Msg:     resp.GetMsg(),
	}
}

func fromRPCNodes(nodes []*knowledge.KnowledgeGraphNode) []GraphNode {
	out := make([]GraphNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		out = append(out, fromRPCNode(node))
	}
	return out
}

func fromRPCNode(node *knowledge.KnowledgeGraphNode) GraphNode {
	if node == nil {
		return GraphNode{}
	}
	return GraphNode{
		ID:          node.GetId(),
		Name:        node.GetName(),
		Type:        node.GetType(),
		Summary:     node.GetSummary(),
		CommunityID: node.GetCommunityId(),
		Score:       node.GetScore(),
		Degree:      int(node.GetDegree()),
		Size:        node.GetSize(),
		Color:       node.GetColor(),
	}
}

func fromRPCEdges(edges []*knowledge.KnowledgeGraphEdge) []GraphEdge {
	out := make([]GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if edge == nil {
			continue
		}
		out = append(out, fromRPCEdge(edge))
	}
	return out
}

func fromRPCEdge(edge *knowledge.KnowledgeGraphEdge) GraphEdge {
	if edge == nil {
		return GraphEdge{}
	}
	return GraphEdge{
		ID:          edge.GetId(),
		SourceID:    edge.GetSourceId(),
		TargetID:    edge.GetTargetId(),
		Relation:    edge.GetRelation(),
		Description: edge.GetDescription(),
		Weight:      edge.GetWeight(),
		Evidence:    edge.GetEvidence(),
		Color:       edge.GetColor(),
		DocumentID:  edge.GetDocumentId(),
	}
}

func fromRPCCommunities(communities []*knowledge.KnowledgeGraphCommunity) []GraphCommunity {
	out := make([]GraphCommunity, 0, len(communities))
	for _, community := range communities {
		if community == nil {
			continue
		}
		out = append(out, GraphCommunity{
			ID:      community.GetId(),
			Name:    community.GetName(),
			Summary: community.GetSummary(),
			Level:   community.GetLevel(),
			Color:   community.GetColor(),
		})
	}
	return out
}

func fromRPCStats(stats *knowledge.KnowledgeGraphStats) GraphStats {
	if stats == nil {
		return GraphStats{}
	}
	return GraphStats{
		NodeCount:      int(stats.GetNodeCount()),
		EdgeCount:      int(stats.GetEdgeCount()),
		CommunityCount: int(stats.GetCommunityCount()),
		Types:          stats.GetTypes(),
		Relations:      stats.GetRelations(),
	}
}

func fromRPCReviewCandidates(candidates []*knowledge.GraphReviewCandidate) []GraphReviewCandidate {
	out := make([]GraphReviewCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		out = append(out, *fromRPCReviewCandidate(candidate))
	}
	return out
}

func fromRPCReviewCandidate(candidate *knowledge.GraphReviewCandidate) *GraphReviewCandidate {
	if candidate == nil {
		return nil
	}
	return &GraphReviewCandidate{
		ID:         candidate.GetId(),
		ItemType:   candidate.GetItemType(),
		ItemID:     candidate.GetItemId(),
		Name:       candidate.GetName(),
		Type:       candidate.GetType(),
		Summary:    candidate.GetSummary(),
		Evidence:   candidate.GetEvidence(),
		Reason:     candidate.GetReason(),
		Status:     candidate.GetStatus(),
		ReviewNote: candidate.GetReviewNote(),
		CreatedAt:  candidate.GetCreatedAt(),
		UpdatedAt:  candidate.GetUpdatedAt(),
		ReviewedAt: candidate.GetReviewedAt(),
	}
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	return errors.New(defaultRPCMsg(msg))
}

func defaultRPCMsg(msg string) string {
	if msg == "" {
		return "knowledge-service RPC调用失败"
	}
	return msg
}
