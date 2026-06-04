package knowledgeclient

import (
	"ClaranAIM/kitex_gen/rag"
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
)

// GraphSource 表示 knowledge-service 可以读取的底层图谱来源。
// 当前由 rag-service.GetGraph 提供；未来如果图谱独立落库，只需要替换该接口实现。
type GraphSource interface {
	GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error)
}

type ragBackedService struct {
	source GraphSource
}

// NewRAGBackedService 创建基于 rag-service GraphRAG 子图的知识图谱视图服务。
func NewRAGBackedService(source GraphSource) Service {
	return &ragBackedService{source: source}
}

func (s *ragBackedService) GetGraphView(ctx context.Context, viewerID int64, input GraphQuery) (*GraphView, error) {
	graph, err := s.loadGraph(ctx, viewerID, input)
	if err != nil {
		return nil, err
	}
	nodes, edges, communities := normalizeGraph(graph)
	nodes, edges, communities = filterGraph(nodes, edges, communities, input)
	edges = dedupeAndSortEdges(edges)
	nodes, edges = applyVisualAttrs(nodes, edges, communities)
	sortGraphNodes(nodes)
	return &GraphView{
		Success:     true,
		Nodes:       nodes,
		Edges:       edges,
		Communities: communities,
		Stats:       buildStats(nodes, edges, communities),
		Msg:         graph.GetMsg(),
	}, nil
}

func (s *ragBackedService) GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*NodeDetail, error) {
	view, err := s.GetGraphView(ctx, viewerID, input)
	if err != nil {
		return nil, err
	}
	nodeByID := map[int64]GraphNode{}
	for _, node := range view.Nodes {
		nodeByID[node.ID] = node
	}
	node, ok := nodeByID[nodeID]
	if !ok {
		return &NodeDetail{Success: false, Msg: "节点不存在或当前用户不可见"}, nil
	}
	var relations []GraphEdge
	neighborIDs := map[int64]bool{}
	for _, edge := range view.Edges {
		if edge.SourceID != nodeID && edge.TargetID != nodeID {
			continue
		}
		relations = append(relations, edge)
		if edge.SourceID != nodeID {
			neighborIDs[edge.SourceID] = true
		}
		if edge.TargetID != nodeID {
			neighborIDs[edge.TargetID] = true
		}
	}
	neighbors := make([]GraphNode, 0, len(neighborIDs))
	for id := range neighborIDs {
		if neighbor, ok := nodeByID[id]; ok {
			neighbors = append(neighbors, neighbor)
		}
	}
	sort.Slice(neighbors, func(i, j int) bool { return neighbors[i].Degree > neighbors[j].Degree })
	return &NodeDetail{Success: true, Node: node, Relations: relations, Neighbors: neighbors}, nil
}

func (s *ragBackedService) GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input GraphQuery) (*EdgeDetail, error) {
	view, err := s.GetGraphView(ctx, viewerID, input)
	if err != nil {
		return nil, err
	}
	nodeByID := map[int64]GraphNode{}
	for _, node := range view.Nodes {
		nodeByID[node.ID] = node
	}
	for _, edge := range view.Edges {
		if edge.ID != edgeID {
			continue
		}
		source := nodeByID[edge.SourceID]
		target := nodeByID[edge.TargetID]
		return &EdgeDetail{Success: true, Edge: edge, Source: &source, Target: &target}, nil
	}
	return &EdgeDetail{Success: false, Msg: "关系不存在或当前用户不可见"}, nil
}

func (s *ragBackedService) GetNeighborhood(ctx context.Context, viewerID, nodeID int64, input GraphQuery) (*GraphView, error) {
	graph, err := s.loadGraph(ctx, viewerID, input)
	if err != nil {
		return nil, err
	}
	nodes, edges, communities := normalizeGraph(graph)
	nodes, edges, communities = filterGraph(nodes, edges, communities, input)
	edges = dedupeAndSortEdges(edges)
	nodeByID := indexNodes(nodes)
	center, ok := nodeByID[nodeID]
	if !ok {
		return &GraphView{Success: false, Msg: "节点不存在或当前用户不可见"}, nil
	}
	depth := input.Hops
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	allowedIDs := expandByHops(map[int64]bool{nodeID: true}, edges, depth)
	subNodes, subEdges, subCommunities := subgraphByIDs(nodes, edges, communities, allowedIDs)
	subEdges = dedupeAndSortEdges(subEdges)
	subNodes, subEdges = applyVisualAttrs(subNodes, subEdges, subCommunities)
	sortGraphNodes(subNodes)
	moveNodeFirst(subNodes, center.ID)
	return &GraphView{
		Success:     true,
		Nodes:       subNodes,
		Edges:       subEdges,
		Communities: subCommunities,
		Stats:       buildStats(subNodes, subEdges, subCommunities),
	}, nil
}

func (s *ragBackedService) GetPath(ctx context.Context, viewerID, sourceID, targetID int64, input GraphQuery) (*PathDetail, error) {
	graph, err := s.loadGraph(ctx, viewerID, input)
	if err != nil {
		return nil, err
	}
	nodes, edges, communities := normalizeGraph(graph)
	nodes, edges, communities = filterGraph(nodes, edges, communities, input)
	edges = dedupeAndSortEdges(edges)
	nodeByID := indexNodes(nodes)
	if _, ok := nodeByID[sourceID]; !ok {
		return &PathDetail{Success: false, Msg: "起点不存在或当前用户不可见"}, nil
	}
	if _, ok := nodeByID[targetID]; !ok {
		return &PathDetail{Success: false, Msg: "终点不存在或当前用户不可见"}, nil
	}
	nodeIDs, edgeIDs := shortestPath(sourceID, targetID, edges)
	if len(nodeIDs) == 0 {
		return &PathDetail{Success: false, Msg: "未找到可见路径"}, nil
	}
	allowedIDs := map[int64]bool{}
	for _, id := range nodeIDs {
		allowedIDs[id] = true
	}
	subNodes, subEdges, subCommunities := subgraphByIDs(nodes, edges, communities, allowedIDs)
	edgeIDSet := map[int64]bool{}
	for _, id := range edgeIDs {
		edgeIDSet[id] = true
	}
	pathEdges := make([]GraphEdge, 0, len(edgeIDs))
	for _, edge := range subEdges {
		if edgeIDSet[edge.ID] {
			pathEdges = append(pathEdges, edge)
		}
	}
	subNodes, pathEdges = applyVisualAttrs(subNodes, pathEdges, subCommunities)
	orderNodesByPath(subNodes, nodeIDs)
	orderEdgesByPath(pathEdges, edgeIDs)
	return &PathDetail{
		Success: true,
		Nodes:   subNodes,
		Edges:   pathEdges,
		NodeIDs: nodeIDs,
		EdgeIDs: edgeIDs,
	}, nil
}

func (s *ragBackedService) CreateGraphReviewCandidate(ctx context.Context, viewerID int64, input CreateGraphReviewCandidateInput) (*GraphReviewCandidate, error) {
	return nil, errors.New("图谱审核需要通过knowledge-service仓储执行")
}

func (s *ragBackedService) ListGraphReviewCandidates(ctx context.Context, viewerID int64, input ListGraphReviewCandidatesInput) (*GraphReviewCandidateList, error) {
	return &GraphReviewCandidateList{Success: false, Msg: "图谱审核需要通过knowledge-service仓储执行"}, nil
}

func (s *ragBackedService) ReviewGraphCandidate(ctx context.Context, viewerID int64, input ReviewGraphCandidateInput) (*GraphReviewCandidate, error) {
	return nil, errors.New("图谱审核需要通过knowledge-service仓储执行")
}

func (s *ragBackedService) loadGraph(ctx context.Context, viewerID int64, input GraphQuery) (*rag.GraphResp, error) {
	if s == nil || s.source == nil {
		return nil, errors.New("knowledge graph source未初始化")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 160
	}
	resp, err := s.source.GetGraph(ctx, viewerID, GraphInput{Query: input.Query, Limit: limit, DocumentID: input.DocumentID, Hops: input.Hops})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("knowledge graph source返回空响应")
	}
	if !resp.GetSuccess() {
		msg := resp.GetMsg()
		if msg == "" {
			msg = "知识图谱查询失败"
		}
		return nil, errors.New(msg)
	}
	return resp, nil
}

func normalizeGraph(resp *rag.GraphResp) ([]GraphNode, []GraphEdge, []GraphCommunity) {
	nodes := make([]GraphNode, 0, len(resp.GetNodes()))
	for _, node := range resp.GetNodes() {
		if node == nil {
			continue
		}
		if !isDisplayableGraphNode(node.GetName(), node.GetType(), node.GetSummary()) {
			continue
		}
		nodes = append(nodes, GraphNode{
			ID:          node.GetId(),
			Name:        node.GetName(),
			Type:        displayEntityType(defaultString(node.GetType(), "主题")),
			Summary:     node.GetSummary(),
			CommunityID: node.GetCommunityId(),
			Score:       node.GetScore(),
		})
	}
	edges := make([]GraphEdge, 0, len(resp.GetEdges()))
	for _, edge := range resp.GetEdges() {
		if edge == nil {
			continue
		}
		if edge.GetSourceId() <= 0 || edge.GetTargetId() <= 0 || edge.GetSourceId() == edge.GetTargetId() {
			continue
		}
		relation := normalizeRelation(edge.GetRelation())
		if relation == "" {
			continue
		}
		edges = append(edges, GraphEdge{
			ID:          edge.GetId(),
			SourceID:    edge.GetSourceId(),
			TargetID:    edge.GetTargetId(),
			Relation:    relation,
			Description: relationDescription(edge.GetRelation(), edge.GetEvidence()),
			Weight:      edge.GetWeight(),
			Evidence:    edge.GetEvidence(),
			DocumentID:  edge.GetDocumentId(),
		})
	}
	communities := make([]GraphCommunity, 0, len(resp.GetCommunities()))
	for idx, community := range resp.GetCommunities() {
		if community == nil {
			continue
		}
		communities = append(communities, GraphCommunity{
			ID:      community.GetId(),
			Name:    community.GetName(),
			Summary: community.GetSummary(),
			Level:   community.GetLevel(),
			Color:   communityColor(idx),
		})
	}
	return nodes, edges, communities
}

func filterGraph(nodes []GraphNode, edges []GraphEdge, communities []GraphCommunity, input GraphQuery) ([]GraphNode, []GraphEdge, []GraphCommunity) {
	typeSet := toSet(input.TypeFilters)
	relationSet := toSet(input.RelationFilters)
	nodeByID := map[int64]GraphNode{}
	allowedIDs := map[int64]bool{}
	for _, node := range nodes {
		if len(typeSet) > 0 && !typeSet[node.Type] && !typeSet[displayEntityType(node.Type)] {
			continue
		}
		if input.CommunityID > 0 && node.CommunityID != input.CommunityID {
			continue
		}
		nodeByID[node.ID] = node
		allowedIDs[node.ID] = true
	}
	filteredEdges := make([]GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if len(relationSet) > 0 && !relationSet[edge.Relation] {
			continue
		}
		if !allowedIDs[edge.SourceID] || !allowedIDs[edge.TargetID] {
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	if input.Hops == 1 || input.Hops == 2 {
		allowedIDs = expandByHops(allowedIDs, filteredEdges, input.Hops)
	}
	filteredNodes := make([]GraphNode, 0, len(allowedIDs))
	for _, node := range nodeByID {
		if allowedIDs[node.ID] {
			filteredNodes = append(filteredNodes, node)
		}
	}
	usedCommunities := map[int64]bool{}
	for _, node := range filteredNodes {
		if node.CommunityID > 0 {
			usedCommunities[node.CommunityID] = true
		}
	}
	filteredCommunities := make([]GraphCommunity, 0, len(communities))
	for _, community := range communities {
		if usedCommunities[community.ID] {
			filteredCommunities = append(filteredCommunities, community)
		}
	}
	return filteredNodes, filteredEdges, filteredCommunities
}

func dedupeAndSortEdges(edges []GraphEdge) []GraphEdge {
	best := map[string]GraphEdge{}
	for _, edge := range edges {
		if edge.SourceID <= 0 || edge.TargetID <= 0 || edge.SourceID == edge.TargetID {
			continue
		}
		key := edgeKey(edge)
		if existing, ok := best[key]; ok && existing.Weight >= edge.Weight {
			continue
		}
		best[key] = edge
	}
	out := make([]GraphEdge, 0, len(best))
	for _, edge := range best {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		if out[i].Relation != out[j].Relation {
			return out[i].Relation < out[j].Relation
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func edgeKey(edge GraphEdge) string {
	return normalizeRelation(edge.Relation) + ":" + strconv.FormatInt(edge.SourceID, 10) + ">" + strconv.FormatInt(edge.TargetID, 10)
}

func applyVisualAttrs(nodes []GraphNode, edges []GraphEdge, communities []GraphCommunity) ([]GraphNode, []GraphEdge) {
	degree := map[int64]int{}
	for i := range edges {
		edges[i].Color = relationColor(edges[i].Relation)
		degree[edges[i].SourceID]++
		degree[edges[i].TargetID]++
	}
	communityColorByID := map[int64]string{}
	for _, community := range communities {
		communityColorByID[community.ID] = community.Color
	}
	for i := range nodes {
		nodes[i].Degree = degree[nodes[i].ID]
		nodes[i].Color = defaultString(communityColorByID[nodes[i].CommunityID], typeColor(nodes[i].Type))
		nodes[i].Size = math.Max(24, math.Min(76, 28+float64(nodes[i].Degree)*8+nodes[i].Score*4))
	}
	return nodes, edges
}

func buildStats(nodes []GraphNode, edges []GraphEdge, communities []GraphCommunity) GraphStats {
	typeSet := map[string]bool{}
	relationSet := map[string]bool{}
	for _, node := range nodes {
		typeSet[node.Type] = true
	}
	for _, edge := range edges {
		relationSet[edge.Relation] = true
	}
	return GraphStats{
		NodeCount:      len(nodes),
		EdgeCount:      len(edges),
		CommunityCount: len(communities),
		Types:          sortedKeys(typeSet),
		Relations:      sortedKeys(relationSet),
	}
}

func indexNodes(nodes []GraphNode) map[int64]GraphNode {
	out := make(map[int64]GraphNode, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func subgraphByIDs(nodes []GraphNode, edges []GraphEdge, communities []GraphCommunity, allowedIDs map[int64]bool) ([]GraphNode, []GraphEdge, []GraphCommunity) {
	subNodes := make([]GraphNode, 0, len(allowedIDs))
	usedCommunities := map[int64]bool{}
	for _, node := range nodes {
		if !allowedIDs[node.ID] {
			continue
		}
		subNodes = append(subNodes, node)
		if node.CommunityID > 0 {
			usedCommunities[node.CommunityID] = true
		}
	}
	subEdges := make([]GraphEdge, 0, len(edges))
	for _, edge := range edges {
		if allowedIDs[edge.SourceID] && allowedIDs[edge.TargetID] {
			subEdges = append(subEdges, edge)
		}
	}
	subCommunities := make([]GraphCommunity, 0, len(communities))
	for _, community := range communities {
		if usedCommunities[community.ID] {
			subCommunities = append(subCommunities, community)
		}
	}
	return subNodes, subEdges, subCommunities
}

func moveNodeFirst(nodes []GraphNode, nodeID int64) {
	for i := range nodes {
		if nodes[i].ID != nodeID {
			continue
		}
		if i > 0 {
			nodes[0], nodes[i] = nodes[i], nodes[0]
		}
		return
	}
}

type pathStep struct {
	nodeID int64
	edgeID int64
}

func shortestPath(sourceID, targetID int64, edges []GraphEdge) ([]int64, []int64) {
	if sourceID == targetID {
		return []int64{sourceID}, nil
	}
	adjacency := map[int64][]pathStep{}
	for _, edge := range edges {
		adjacency[edge.SourceID] = append(adjacency[edge.SourceID], pathStep{nodeID: edge.TargetID, edgeID: edge.ID})
		adjacency[edge.TargetID] = append(adjacency[edge.TargetID], pathStep{nodeID: edge.SourceID, edgeID: edge.ID})
	}
	queue := []int64{sourceID}
	visited := map[int64]bool{sourceID: true}
	prevNode := map[int64]int64{}
	prevEdge := map[int64]int64{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if visited[next.nodeID] {
				continue
			}
			visited[next.nodeID] = true
			prevNode[next.nodeID] = current
			prevEdge[next.nodeID] = next.edgeID
			if next.nodeID == targetID {
				return rebuildPath(sourceID, targetID, prevNode, prevEdge)
			}
			queue = append(queue, next.nodeID)
		}
	}
	return nil, nil
}

func rebuildPath(sourceID, targetID int64, prevNode, prevEdge map[int64]int64) ([]int64, []int64) {
	var reversedNodes []int64
	var reversedEdges []int64
	for current := targetID; ; current = prevNode[current] {
		reversedNodes = append(reversedNodes, current)
		if current == sourceID {
			break
		}
		reversedEdges = append(reversedEdges, prevEdge[current])
	}
	for i, j := 0, len(reversedNodes)-1; i < j; i, j = i+1, j-1 {
		reversedNodes[i], reversedNodes[j] = reversedNodes[j], reversedNodes[i]
	}
	for i, j := 0, len(reversedEdges)-1; i < j; i, j = i+1, j-1 {
		reversedEdges[i], reversedEdges[j] = reversedEdges[j], reversedEdges[i]
	}
	return reversedNodes, reversedEdges
}

func orderNodesByPath(nodes []GraphNode, nodeIDs []int64) {
	order := map[int64]int{}
	for i, id := range nodeIDs {
		order[id] = i
	}
	sort.Slice(nodes, func(i, j int) bool { return order[nodes[i].ID] < order[nodes[j].ID] })
}

func orderEdgesByPath(edges []GraphEdge, edgeIDs []int64) {
	order := map[int64]int{}
	for i, id := range edgeIDs {
		order[id] = i
	}
	sort.Slice(edges, func(i, j int) bool { return order[edges[i].ID] < order[edges[j].ID] })
}

func sortGraphNodes(nodes []GraphNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].CommunityID != nodes[j].CommunityID {
			return nodes[i].CommunityID < nodes[j].CommunityID
		}
		if nodes[i].Degree != nodes[j].Degree {
			return nodes[i].Degree > nodes[j].Degree
		}
		if nodes[i].Score != nodes[j].Score {
			return nodes[i].Score > nodes[j].Score
		}
		return nodes[i].Name < nodes[j].Name
	})
}

func expandByHops(seed map[int64]bool, edges []GraphEdge, hops int) map[int64]bool {
	out := map[int64]bool{}
	frontier := map[int64]bool{}
	for id := range seed {
		out[id] = true
		frontier[id] = true
	}
	for step := 0; step < hops; step++ {
		next := map[int64]bool{}
		for _, edge := range edges {
			if frontier[edge.SourceID] && !out[edge.TargetID] {
				out[edge.TargetID] = true
				next[edge.TargetID] = true
			}
			if frontier[edge.TargetID] && !out[edge.SourceID] {
				out[edge.SourceID] = true
				next[edge.SourceID] = true
			}
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}
	return out
}

func toSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
			out[displayEntityType(value)] = true
		}
	}
	return out
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeRelation(value string) string {
	raw := strings.Trim(value, " \t\r\n，。,.；;:：()（）[]【】\"'")
	raw = strings.Join(strings.Fields(raw), " ")
	upper := strings.ToUpper(raw)
	upper = strings.ReplaceAll(upper, "-", "_")
	upper = strings.ReplaceAll(upper, " ", "_")
	switch upper {
	case "CALLS", "PUBLISHES", "CONSUMES", "STORES", "OWNS", "DEPENDS_ON", "CONFIGURES", "TRIGGERS", "READS", "WRITES", "RELATED_TO", "CONTAINS":
		if upper == "RELATED_TO" {
			return ""
		}
		return upper
	default:
		if isGenericGraphRelation(raw) || len([]rune(raw)) > 24 {
			return ""
		}
		return raw
	}
}

func displayEntityType(value string) string {
	value = strings.Trim(value, " \t\r\n，。,.；;:：()（）[]【】\"'")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || len([]rune(value)) > 24 || isGenericGraphEntityType(value) {
		return "主题"
	}
	return value
}

func relationDescription(relation, evidence string) string {
	relation = normalizeRelation(relation)
	label := relationLabel(relation)
	if strings.TrimSpace(evidence) != "" {
		return label + "： " + truncateText(strings.TrimSpace(evidence), 220)
	}
	return label + "：该关系由 GraphRAG 从文档上下文中抽取，用于说明两个实体之间的结构联系。"
}

func relationLabel(relation string) string {
	switch normalizeRelation(relation) {
	case "CALLS":
		return "调用关系"
	case "PUBLISHES":
		return "发布关系"
	case "CONSUMES":
		return "消费关系"
	case "STORES":
		return "存储关系"
	case "OWNS":
		return "负责关系"
	case "DEPENDS_ON":
		return "依赖关系"
	case "CONFIGURES":
		return "配置关系"
	case "TRIGGERS":
		return "触发关系"
	case "READS":
		return "读取关系"
	case "WRITES":
		return "写入关系"
	case "CONTAINS":
		return "包含关系"
	default:
		return defaultString(relation, "关系")
	}
}

func isGenericGraphEntityType(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, item := range []string{"entity", "node", "object", "content", "data", "information", "unknown", "实体", "节点", "对象", "内容", "数据", "信息", "未知", "concept", "概念"} {
		if lower == strings.ToLower(item) {
			return true
		}
	}
	return false
}

func isGenericGraphRelation(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, item := range []string{"related_to", "relates_to", "related", "associated_with", "associates_with", "相关", "关联", "有关", "提到", "提及", "同现", "一起出现", "co-occur", "mentioned"} {
		if lower == strings.ToLower(item) || strings.Contains(lower, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func isDisplayableGraphNode(name, entityType, summary string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if len([]rune(key)) < 2 {
		return false
	}
	if isNumericLikeName(key) {
		return false
	}
	if isFieldOrStatusGraphName(key) {
		return false
	}
	noise := []string{"正文", "标题", "内容", "图片", "截图", "页面", "文件", "文本", "数据", "信息", "说明", "示例", "用户", "系统", "未知实体", "unknown", "none", "n/a", "todo"}
	for _, item := range noise {
		if key == strings.ToLower(item) || strings.Contains(strings.ToLower(summary), "无意义实体") {
			return false
		}
	}
	if strings.HasPrefix(key, "http") || strings.Contains(key, "\\") || strings.Contains(key, "/") {
		return false
	}
	if strings.HasSuffix(key, ".png") || strings.HasSuffix(key, ".jpg") || strings.HasSuffix(key, ".jpeg") || strings.HasSuffix(key, ".webp") || strings.HasSuffix(key, ".pdf") || strings.HasSuffix(key, ".docx") {
		return false
	}
	_ = entityType
	return true
}

func isFieldOrStatusGraphName(value string) bool {
	fields := map[string]bool{
		"id": true, "ids": true, "user_id": true, "sender_id": true, "owner_id": true, "group_id": true, "conversation_id": true,
		"message_id": true, "msg_id": true, "reply_to_id": true, "event_id": true, "source_event_id": true,
		"client_msg_id": true, "trace_id": true, "agent_trace_id": true, "agent_user_id": true, "bot_id": true,
		"created_at": true, "updated_at": true, "deleted_at": true, "status": true, "type": true, "scope": true,
		"pending": true, "processing": true, "completed": true, "failed": true, "success": true, "error": true,
	}
	if fields[value] {
		return true
	}
	return strings.HasSuffix(value, "_id") || strings.HasSuffix(value, "_ids") || strings.HasSuffix(value, "_status") || strings.HasSuffix(value, "_type")
}

func isNumericLikeName(value string) bool {
	if value == "" {
		return true
	}
	for _, r := range value {
		if (r < '0' || r > '9') && r != '.' && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func truncateText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func typeColor(entityType string) string {
	switch entityType {
	case "Service":
		return "#0ea5e9"
	case "DatabaseTable":
		return "#22c55e"
	case "EventTopic":
		return "#f97316"
	case "API":
		return "#a855f7"
	case "Module":
		return "#6366f1"
	case "Person":
		return "#ec4899"
	case "Organization":
		return "#14b8a6"
	case "Product":
		return "#84cc16"
	default:
		return "#64748b"
	}
}

func relationColor(relation string) string {
	switch normalizeRelation(relation) {
	case "READS", "WRITES", "STORES":
		return "#16a34a"
	case "CALLS":
		return "#2563eb"
	case "PUBLISHES", "CONSUMES", "TRIGGERS":
		return "#ea580c"
	case "DEPENDS_ON", "CONFIGURES", "OWNS":
		return "#9333ea"
	case "CONTAINS":
		return "#64748b"
	default:
		return "#64748b"
	}
}

func communityColor(index int) string {
	palette := []string{"#0f766e", "#2563eb", "#b45309", "#be123c", "#7c3aed", "#15803d", "#475569"}
	if index < 0 {
		index = 0
	}
	return palette[index%len(palette)]
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
