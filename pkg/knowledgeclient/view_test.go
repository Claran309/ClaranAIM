package knowledgeclient

import (
	"ClaranAIM/kitex_gen/rag"
	"context"
	"strings"
	"testing"
)

func TestGraphViewFiltersAndComputesVisualAttributes(t *testing.T) {
	source := &fakeGraphSource{graph: sampleGraph()}
	svc := NewRAGBackedService(source)

	view, err := svc.GetGraphView(context.Background(), 1001, GraphQuery{
		Query:           "agent",
		TypeFilters:     []string{"Service", "Data"},
		RelationFilters: []string{"DATA_FLOW", "CALLS"},
		Hops:            1,
		Limit:           20,
	})
	if err != nil {
		t.Fatalf("GetGraphView returned error: %v", err)
	}
	if !view.Success {
		t.Fatalf("view success = false, msg=%s", view.Msg)
	}
	if len(view.Nodes) != 3 {
		t.Fatalf("nodes len = %d, want 3 filtered service/table nodes: %#v", len(view.Nodes), view.Nodes)
	}
	if len(view.Edges) != 2 {
		t.Fatalf("edges len = %d, want WRITES and CALLS edges: %#v", len(view.Edges), view.Edges)
	}
	for _, node := range view.Nodes {
		if node.Type == "EventTopic" {
			t.Fatalf("EventTopic node should be filtered out: %#v", node)
		}
		if node.Color == "" || node.Size <= 0 {
			t.Fatalf("node visual attrs missing: %#v", node)
		}
	}
	if view.Stats.NodeCount != len(view.Nodes) || view.Stats.EdgeCount != len(view.Edges) {
		t.Fatalf("stats = %#v, want counts to match view", view.Stats)
	}
	if !containsString(view.Stats.Types, "Service") || !containsString(view.Stats.Relations, "DATA_FLOW") {
		t.Fatalf("stats = %#v, want type and relation facets", view.Stats)
	}
	if view.Nodes[0].Name != "agent-manager-service" || view.Nodes[0].Degree != 2 {
		t.Fatalf("first node = %#v, want nodes sorted by computed degree after visual attrs", view.Nodes[0])
	}
}

func TestNodeDetailReturnsNeighborRelations(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	detail, err := svc.GetNodeDetail(context.Background(), 1001, 1, GraphQuery{Limit: 20})
	if err != nil {
		t.Fatalf("GetNodeDetail returned error: %v", err)
	}
	if !detail.Success || detail.Node.Name != "agent-manager-service" {
		t.Fatalf("detail = %#v, want agent-manager-service", detail)
	}
	if len(detail.Relations) != 3 {
		t.Fatalf("relations len = %d, want all node incident relations", len(detail.Relations))
	}
	if len(detail.Neighbors) != 3 {
		t.Fatalf("neighbors len = %d, want 3 neighbors", len(detail.Neighbors))
	}
}

func TestEdgeDetailReturnsEndpointsAndEvidence(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	detail, err := svc.GetEdgeDetail(context.Background(), 1001, 101, GraphQuery{Limit: 20})
	if err != nil {
		t.Fatalf("GetEdgeDetail returned error: %v", err)
	}
	if !detail.Success || detail.Edge.Relation != "DATA_FLOW" {
		t.Fatalf("detail = %#v, want DATA_FLOW edge", detail)
	}
	if detail.Source == nil || detail.Source.Name != "agent-manager-service" {
		t.Fatalf("source = %#v, want agent-manager-service", detail.Source)
	}
	if detail.Target == nil || detail.Target.Name != "agent_dispatch_records" {
		t.Fatalf("target = %#v, want agent_dispatch_records", detail.Target)
	}
	if detail.Edge.Evidence == "" {
		t.Fatalf("edge evidence should be kept")
	}
}

func TestNeighborhoodReturnsCenteredSubgraphByDepth(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	view, err := svc.GetNeighborhood(context.Background(), 1001, 1, GraphQuery{Hops: 1, Limit: 20})
	if err != nil {
		t.Fatalf("GetNeighborhood returned error: %v", err)
	}
	if !view.Success {
		t.Fatalf("neighborhood success = false, msg=%s", view.Msg)
	}
	if len(view.Nodes) != 4 {
		t.Fatalf("nodes len = %d, want center plus 3 one-hop neighbors: %#v", len(view.Nodes), view.Nodes)
	}
	if len(view.Edges) != 3 {
		t.Fatalf("edges len = %d, want all incident one-hop edges: %#v", len(view.Edges), view.Edges)
	}
	if view.Nodes[0].Name != "agent-manager-service" {
		t.Fatalf("first node = %#v, want center node kept first", view.Nodes[0])
	}
}

func TestNeighborhoodAppliesGraphFilters(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	view, err := svc.GetNeighborhood(context.Background(), 1001, 1, GraphQuery{
		TypeFilters:     []string{"Service"},
		RelationFilters: []string{"CALLS"},
		Hops:            1,
		Limit:           20,
	})
	if err != nil {
		t.Fatalf("GetNeighborhood returned error: %v", err)
	}
	if !view.Success {
		t.Fatalf("neighborhood success = false, msg=%s", view.Msg)
	}
	if len(view.Nodes) != 2 {
		t.Fatalf("nodes len = %d, want only service center and service neighbor: %#v", len(view.Nodes), view.Nodes)
	}
	if len(view.Edges) != 1 || view.Edges[0].Relation != "CALLS" {
		t.Fatalf("edges = %#v, want only CALLS edge", view.Edges)
	}
	for _, node := range view.Nodes {
		if node.Type != "Service" {
			t.Fatalf("node = %#v, want filter to keep only Service nodes", node)
		}
	}
}

func TestPathFindsShortestPathAndBuildsSubgraph(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	path, err := svc.GetPath(context.Background(), 1001, 2, 3, GraphQuery{Limit: 20})
	if err != nil {
		t.Fatalf("GetPath returned error: %v", err)
	}
	if !path.Success {
		t.Fatalf("path success = false, msg=%s", path.Msg)
	}
	if got, want := path.NodeIDs, []int64{2, 1, 3}; !sameInt64s(got, want) {
		t.Fatalf("path nodes = %#v, want %#v", got, want)
	}
	if got, want := path.EdgeIDs, []int64{101, 102}; !sameInt64s(got, want) {
		t.Fatalf("path edges = %#v, want %#v", got, want)
	}
	if len(path.Nodes) != 3 || len(path.Edges) != 2 {
		t.Fatalf("path graph nodes=%d edges=%d, want 3/2", len(path.Nodes), len(path.Edges))
	}
}

func TestPathAppliesGraphFilters(t *testing.T) {
	svc := NewRAGBackedService(&fakeGraphSource{graph: sampleGraph()})

	path, err := svc.GetPath(context.Background(), 1001, 2, 3, GraphQuery{
		TypeFilters: []string{"Service"},
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("GetPath returned error: %v", err)
	}
	if path.Success {
		t.Fatalf("path success = true, want database-table source filtered out")
	}
	if path.Msg == "" {
		t.Fatalf("path failure should explain filtered or invisible endpoint")
	}
}

func TestGraphViewPassesDocumentScopeToGraphSource(t *testing.T) {
	source := &fakeGraphSource{graph: sampleGraph()}
	svc := NewRAGBackedService(source)

	_, err := svc.GetGraphView(context.Background(), 1001, GraphQuery{
		Query:      "agent",
		DocumentID: 42,
		Hops:       2,
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("GetGraphView returned error: %v", err)
	}
	if source.lastInput.DocumentID != 42 || source.lastInput.Hops != 2 {
		t.Fatalf("last input = %#v, want document_id=42 hops=2 passed through", source.lastInput)
	}
}

func TestGraphViewPreservesEmptyGraphDiagnosticMessage(t *testing.T) {
	source := &fakeGraphSource{graph: &rag.GraphResp{
		Success: true,
		Msg:     "该文档共有 601 个可读分块，但没有通过 GraphRAG 实体/关系质量过滤。",
	}}
	svc := NewRAGBackedService(source)

	view, err := svc.GetGraphView(context.Background(), 1001, GraphQuery{DocumentID: 42, Limit: 20})
	if err != nil {
		t.Fatalf("GetGraphView returned error: %v", err)
	}
	if view.Msg == "" || !strings.Contains(view.Msg, "601") {
		t.Fatalf("view msg = %q, want rag-service diagnostic message preserved", view.Msg)
	}
}

func TestGraphViewDropsMeaninglessNodesBeforeRendering(t *testing.T) {
	graph := sampleGraph()
	graph.Nodes = append(graph.Nodes,
		&rag.RAGGraphNode{Id: 9, Name: "0", Type: "Concept", Summary: "无意义实体", Score: 1},
		&rag.RAGGraphNode{Id: 10, Name: "event_id", Type: "Concept", Summary: "字段名", Score: 1},
	)
	graph.Edges = append(graph.Edges,
		&rag.RAGGraphEdge{Id: 109, SourceId: 1, TargetId: 9, Relation: "RELATED_TO", Weight: 0.9},
		&rag.RAGGraphEdge{Id: 110, SourceId: 10, TargetId: 3, Relation: "RELATED_TO", Weight: 0.9},
	)
	svc := NewRAGBackedService(&fakeGraphSource{graph: graph})

	view, err := svc.GetGraphView(context.Background(), 1001, GraphQuery{Limit: 20})
	if err != nil {
		t.Fatalf("GetGraphView returned error: %v", err)
	}
	for _, node := range view.Nodes {
		if node.Name == "0" || node.Name == "event_id" {
			t.Fatalf("nodes=%#v, want meaningless node %s dropped", view.Nodes, node.Name)
		}
	}
	for _, edge := range view.Edges {
		if edge.SourceID == 9 || edge.TargetID == 9 || edge.SourceID == 10 || edge.TargetID == 10 {
			t.Fatalf("edges=%#v, want edges touching meaningless nodes dropped", view.Edges)
		}
	}
}

type fakeGraphSource struct {
	graph     *rag.GraphResp
	lastInput GraphInput
}

func (s *fakeGraphSource) GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error) {
	_ = ctx
	_ = viewerID
	s.lastInput = input
	return s.graph, nil
}

func sampleGraph() *rag.GraphResp {
	return &rag.GraphResp{
		Success: true,
		Nodes: []*rag.RAGGraphNode{
			{Id: 1, Name: "agent-manager-service", Type: "Service", Summary: "Agent 管理与调度服务", CommunityId: 11, Score: 2.2},
			{Id: 2, Name: "agent_dispatch_records", Type: "DatabaseTable", Summary: "Agent 调度幂等表", CommunityId: 11, Score: 1.5},
			{Id: 3, Name: "agent-runtime-service", Type: "Service", Summary: "Agent 运行时服务", CommunityId: 11, Score: 1.4},
			{Id: 4, Name: "claran.im.events", Type: "EventTopic", Summary: "IM 统一事件 Topic", CommunityId: 12, Score: 1.1},
		},
		Edges: []*rag.RAGGraphEdge{
			{Id: 101, SourceId: 1, TargetId: 2, Relation: "WRITES", Weight: 0.91, Evidence: "agent-manager-service 写入 agent_dispatch_records"},
			{Id: 102, SourceId: 1, TargetId: 3, Relation: "CALLS", Weight: 0.86, Evidence: "agent-manager-service 调用 agent-runtime-service"},
			{Id: 103, SourceId: 1, TargetId: 4, Relation: "CONSUMES", Weight: 0.82, Evidence: "agent-manager-service 消费 claran.im.events"},
		},
		Communities: []*rag.RAGGraphCommunity{
			{Id: 11, Name: "Agent 事件链路", Summary: "Agent 调度相关服务和表", Level: 1},
			{Id: 12, Name: "IM 消息链路", Summary: "IM 事件与消息链路", Level: 1},
		},
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameInt64s(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
