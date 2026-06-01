package knowledgeclient

import (
	"ClaranAIM/kitex_gen/rag"
	"context"
	"testing"
)

func TestGraphViewFiltersAndComputesVisualAttributes(t *testing.T) {
	source := &fakeGraphSource{graph: sampleGraph()}
	svc := NewRAGBackedService(source)

	view, err := svc.GetGraphView(context.Background(), 1001, GraphQuery{
		Query:           "agent",
		TypeFilters:     []string{"Service", "DatabaseTable"},
		RelationFilters: []string{"WRITES", "CALLS"},
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
	if !containsString(view.Stats.Types, "Service") || !containsString(view.Stats.Relations, "WRITES") {
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
	if !detail.Success || detail.Edge.Relation != "WRITES" {
		t.Fatalf("detail = %#v, want WRITES edge", detail)
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

type fakeGraphSource struct {
	graph *rag.GraphResp
}

func (s *fakeGraphSource) GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error) {
	_ = ctx
	_ = viewerID
	_ = input
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
