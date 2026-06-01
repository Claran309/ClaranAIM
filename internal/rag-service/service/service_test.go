package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"strings"
	"testing"
	"time"
)

func TestIngestDocumentBuildsChunksAndGraph(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "adaptive")

	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "ClaranAIM RAG 设计",
		Content:    "ClaranAIM 使用 Agent Runtime 调用 RagService。RagService 负责 Hybrid Search、GraphRAG、CRAG 和 Self-RAG。",
		Source:     "unit-test",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if result.ChunkCount == 0 {
		t.Fatalf("ChunkCount = 0, want > 0")
	}
	if result.EntityCount == 0 {
		t.Fatalf("EntityCount = 0, want > 0")
	}
	if result.RelationCount == 0 {
		t.Fatalf("RelationCount = 0, want > 0")
	}
	if len(repo.chunks) == 0 {
		t.Fatalf("repo chunks empty after ingest")
	}
	if len(repo.entities) == 0 {
		t.Fatalf("repo entities empty after ingest")
	}
}

func TestSearchReturnsSourcesAndSelfRAGCheckpoints(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "adaptive")
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "Agent RAG 手册",
		Content:    "Agentic RAG 会先做 Hybrid Search，再通过 Reranking 精排，并在 CRAG 质量门里判断内部知识是否足够可靠。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "Agentic RAG 为什么需要 Hybrid Search 和 Reranking",
		Mode:     "adaptive",
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Search success = false, msg=%s", result.Msg)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("Search returned no sources")
	}
	if result.SelfCheck == nil {
		t.Fatalf("SelfCheck is nil")
	}
	if !strings.Contains(result.Answer, "RAG 路线") {
		t.Fatalf("Answer = %q, want synthesized RAG answer", result.Answer)
	}
}

func TestHybridRetrieveUsesDenseBM25AndRRF(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")
	docs := []IngestInput{
		{
			OwnerID:    1001,
			Title:      "BM25 术语手册",
			Content:    "BM25 使用词频、逆文档频率和文档长度归一化计算稀疏检索分数。",
			Visibility: model.VisibilityPrivate,
		},
		{
			OwnerID:    1001,
			Title:      "向量召回说明",
			Content:    "Dense embedding 使用向量相似度召回语义相关的知识片段。",
			Visibility: model.VisibilityPrivate,
		},
	}
	for _, doc := range docs {
		if _, err := svc.IngestDocument(context.Background(), doc); err != nil {
			t.Fatalf("IngestDocument returned error: %v", err)
		}
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "BM25 稀疏检索和 dense embedding 如何融合",
		Mode:     "hybrid",
		Limit:    4,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Sources) < 2 {
		t.Fatalf("sources len = %d, want both dense and sparse candidates", len(result.Sources))
	}
	reason := result.Sources[0].Reason
	for _, want := range []string{"rrf=", "bm25_rank=", "dense_rank="} {
		if !strings.Contains(reason, want) {
			t.Fatalf("source reason = %q, want %s", reason, want)
		}
	}
}

func TestSearchUsesModelRerankerAfterRRFTop30(t *testing.T) {
	repo := newFakeRAGRepo()
	reranker := &recordingReranker{
		scoresByTitle: map[string]float64{
			"低相关文档": 0.10,
			"高相关文档": 0.99,
		},
	}
	svc := NewRAGServiceWithRouterAndReranker(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, reranker)
	for _, doc := range []IngestInput{
		{
			OwnerID:    1001,
			Title:      "低相关文档",
			Content:    "Agentic RAG 使用 Hybrid Search、Dense、BM25 和 RRF 做初步召回。",
			Visibility: model.VisibilityPrivate,
		},
		{
			OwnerID:    1001,
			Title:      "高相关文档",
			Content:    "Agentic RAG 在 RRF 后调用模型 reranker 阅读 query 和 chunk，重新给相关性分数。",
			Visibility: model.VisibilityPrivate,
		},
	} {
		if _, err := svc.IngestDocument(context.Background(), doc); err != nil {
			t.Fatalf("IngestDocument returned error: %v", err)
		}
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "RRF 后怎么用模型 reranker 重新排序",
		Mode:     "hybrid",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if reranker.calledWithTopN != 1 {
		t.Fatalf("reranker topN = %d, want final limit 1", reranker.calledWithTopN)
	}
	if reranker.candidateCount < 2 {
		t.Fatalf("reranker candidate count = %d, want RRF candidates before final cut", reranker.candidateCount)
	}
	if len(result.Sources) != 1 || result.Sources[0].Title != "高相关文档" {
		t.Fatalf("sources = %#v, want model reranker to put 高相关文档 first", result.Sources)
	}
	if !strings.Contains(result.Sources[0].Reason, "model_rerank=") {
		t.Fatalf("reason = %q, want model rerank trace", result.Sources[0].Reason)
	}
}

func TestSearchUsesCRAGEvaluatorAfterRerank(t *testing.T) {
	repo := newFakeRAGRepo()
	evaluator := &recordingCRAGEvaluator{
		evaluation: CRAGEvaluation{
			Label:       CRAGLabelAmbiguous,
			Score:       0.56,
			Relevance:   0.72,
			Coverage:    0.45,
			Specificity: 0.62,
			Conflict:    0.10,
			Reason:      "资料提到了 Agent 调度，但没有解释 event_id 和 agent_user_id 的业务含义",
		},
	}
	svc := NewRAGServiceWithRouterRerankerAndCRAG(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, evaluator)
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "Agent 事件调度",
		Content:    "Agent 调度器会消费 message.created 事件，并根据订阅规则决定是否运行 Agent。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "event_id 和 agent_user_id 在 Agent 调度里分别是什么意思",
		Mode:     "hybrid",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if !evaluator.called {
		t.Fatalf("CRAG evaluator was not called")
	}
	if evaluator.query != "event_id 和 agent_user_id 在 Agent 调度里分别是什么意思" {
		t.Fatalf("evaluator query = %q", evaluator.query)
	}
	if evaluator.sourceCount == 0 {
		t.Fatalf("evaluator source count = 0, want reranked sources")
	}
	if result.CragAction != CRAGLabelAmbiguous {
		t.Fatalf("CragAction = %q, want ambiguous", result.CragAction)
	}
	if result.SelfCheck == nil || !strings.Contains(result.SelfCheck.Note, "Coverage=0.45") || !strings.Contains(result.SelfCheck.Note, "event_id") {
		t.Fatalf("SelfCheck note = %#v, want CRAG dimensions and reason", result.SelfCheck)
	}
}

func TestSearchUsesStructuredSelfRAGJudgeWithoutToolExecution(t *testing.T) {
	repo := newFakeRAGRepo()
	router := RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		_ = ctx
		return RouterDecision{
			Mode:            "hybrid",
			Retrieve:        true,
			RetrievalSource: "project_docs",
			Query:           "agent_dispatch_records event_id agent_user_id",
			Reason:          "需要项目文档",
		}, nil
	})
	judge := &recordingSelfRAGJudge{
		result: SelfRAGJudgement{IsRel: true, IsSup: true, IsUse: true, Reason: "答案由来源支持并解释了业务含义"},
	}
	svc := NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, NewLocalVectorIndex(), 64, "adaptive", nil, router, nil, RuleCRAGEvaluator{}, judge)
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "Agent 调度字段说明",
		Content:    "agent_dispatch_records 中 event_id 表示触发 Agent 的 IM 事件，agent_user_id 表示 Agent 作为真实系统用户发言的用户ID。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "这两个字段是什么意思",
		Mode:     "adaptive",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if !judge.called {
		t.Fatalf("Self-RAG judge was not called")
	}
	if judge.query != "这两个字段是什么意思" {
		t.Fatalf("judge query = %q, want original user query", judge.query)
	}
	if !strings.Contains(result.SelfCheck.Note, "retrieval_source=project_docs") || !strings.Contains(result.SelfCheck.Note, "retrieval_query=agent_dispatch_records event_id agent_user_id") {
		t.Fatalf("SelfCheck note = %q, want structured retrieve decision", result.SelfCheck.Note)
	}
	if !result.SelfCheck.Retrieve || !result.SelfCheck.IsRel || !result.SelfCheck.IsSup || !result.SelfCheck.IsUse {
		t.Fatalf("SelfCheck = %#v, want all checkpoints true from judge", result.SelfCheck)
	}
}

func TestMarkdownIngestBuildsParentChildChunksAndSearchReturnsParentContext(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "项目文档",
		SourceType: "markdown",
		Content: strings.Join([]string{
			"# 项目文档",
			"## 鉴权设计",
			"这里说明 access token、refresh token、角色权限和续期策略。",
			"### Refresh Token",
			"refresh token 用于在 access token 过期后续期，并且需要服务端保存撤销状态。",
			"## RAG 设计",
			"这里说明 Hybrid Search、GraphRAG 和知识图谱展示。",
		}, "\n"),
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	var parent model.Chunk
	childCount := 0
	for _, chunk := range repo.chunks {
		if chunk.ChunkLevel == model.ChunkLevelParent && strings.Contains(chunk.Content, "鉴权设计") {
			parent = chunk
		}
		if chunk.ChunkLevel == model.ChunkLevelChild && chunk.ParentChunkID != 0 {
			childCount++
		}
	}
	if parent.ID == 0 {
		t.Fatalf("expected markdown ## section to create parent chunk, chunks=%#v", repo.chunks)
	}
	if childCount == 0 {
		t.Fatalf("expected markdown section content to create child chunks")
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "refresh token 续期 撤销状态",
		Mode:     "hybrid",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("Search returned no sources")
	}
	if result.Sources[0].ChunkId != parent.ID {
		t.Fatalf("source chunk_id = %d, want parent chunk id %d", result.Sources[0].ChunkId, parent.ID)
	}
	if !strings.Contains(result.Sources[0].Content, "鉴权设计") || !strings.Contains(result.Sources[0].Content, "refresh token") {
		t.Fatalf("source content = %q, want parent context", result.Sources[0].Content)
	}
	if !strings.Contains(result.Sources[0].Reason, "child_chunk_id=") || !strings.Contains(result.Sources[0].Reason, "parent_chunk_id=") {
		t.Fatalf("source reason = %q, want child and parent trace", result.Sources[0].Reason)
	}
}

func TestGoIngestSplitsByDeclarationsAndKeepsFunctionComments(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "消息服务代码",
		SourceType: "go",
		Content: strings.Join([]string{
			"package message",
			"",
			"type MessageStatus string",
			"",
			"// SendMessageExt 校验参数、写入事务、删除缓存并发布消息事件。",
			"func SendMessageExt(content string) error {",
			"	if content == \"\" {",
			"		return nil",
			"	}",
			"	return nil",
			"}",
			"",
			"func RecallMessage(id int64) error {",
			"	return nil",
			"}",
		}, "\n"),
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	var sendParent model.Chunk
	for _, chunk := range repo.chunks {
		if chunk.ChunkLevel == model.ChunkLevelParent && strings.Contains(chunk.Content, "func SendMessageExt") {
			sendParent = chunk
			break
		}
	}
	if sendParent.ID == 0 {
		t.Fatalf("expected SendMessageExt to create a parent chunk, chunks=%#v", repo.chunks)
	}
	if !strings.Contains(sendParent.Content, "校验参数") {
		t.Fatalf("SendMessageExt parent content = %q, want preceding comment", sendParent.Content)
	}
	if strings.Contains(sendParent.Content, "func RecallMessage") {
		t.Fatalf("SendMessageExt parent content should not include next function: %q", sendParent.Content)
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "SendMessageExt 删除缓存 发布消息事件",
		Mode:     "hybrid",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("Search returned no sources")
	}
	if result.Sources[0].ChunkId != sendParent.ID {
		t.Fatalf("source chunk_id = %d, want SendMessageExt parent id %d", result.Sources[0].ChunkId, sendParent.ID)
	}
	if !strings.Contains(result.Sources[0].Reason, "child_chunk_id=") {
		t.Fatalf("source reason = %q, want child trace", result.Sources[0].Reason)
	}
}

func TestParentContextUsesSummaryAndHitChildWhenParentIsLong(t *testing.T) {
	longParent := strings.Repeat("背景材料很多。", 180) + "\n\n关键结论：缓存更新后写后删除，并通过 outbox 保证事件最终发布。"
	parent := model.Chunk{
		ID:         101,
		ChunkLevel: model.ChunkLevelParent,
		Content:    longParent,
		Summary:    "父块摘要：缓存策略与 outbox 发布。",
	}
	child := rankedChunk{
		Chunk: model.Chunk{
			ID:            202,
			ParentChunkID: 101,
			ChunkLevel:    model.ChunkLevelChild,
			Content:       "命中小块：写后删除缓存，outbox 异步发布事件。",
		},
		Document: model.Document{ID: 1, Title: "缓存设计"},
		Score:    0.8,
		Reason:   "hybrid rrf=0.03",
	}
	grouped := groupChildHitsByParent([]rankedChunk{child}, map[int64]dao.ChunkWithDocument{
		101: {Chunk: parent, Document: child.Document},
	})
	if len(grouped) != 1 {
		t.Fatalf("grouped len = %d, want 1", len(grouped))
	}
	content := grouped[0].Chunk.Content
	if !strings.Contains(content, "父块摘要") || !strings.Contains(content, "命中小块") {
		t.Fatalf("context content = %q, want parent summary and hit child excerpt", content)
	}
	if strings.Contains(content, strings.Repeat("背景材料很多。", 80)) {
		t.Fatalf("context content includes too much long parent body")
	}
}

func TestGraphRAGCanonicalizesEntityAliasesAndKeepsTypedRelations(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")

	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID: 1001,
		Title:   "消息事件链路",
		Content: strings.Join([]string{
			"msg-core-service 在发送消息事务中写入 event_outbox。",
			"msg core service 也叫消息核心服务，负责把 message.created 发布到 claran.message.events。",
			"websocket-gateway 消费 claran.message.events 并推送在线用户。",
		}, "\n"),
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}

	countMsgCore := 0
	var msgCore model.Entity
	for _, entity := range repo.entities {
		if entity.CanonicalKey == "msgcoreservice" {
			countMsgCore++
			msgCore = entity
		}
	}
	if countMsgCore != 1 {
		t.Fatalf("msg-core-service canonical entity count = %d, want 1; entities=%#v", countMsgCore, repo.entities)
	}
	if !strings.Contains(msgCore.AliasesJSON, "msg core service") || !strings.Contains(msgCore.AliasesJSON, "消息核心服务") {
		t.Fatalf("aliases = %q, want merged English and Chinese aliases", msgCore.AliasesJSON)
	}

	var hasWrites, hasConsumes bool
	for _, rel := range repo.relations {
		if rel.Relation == "WRITES" && rel.Description != "" && rel.EvidenceChunkID != 0 {
			hasWrites = true
		}
		if rel.Relation == "CONSUMES" && rel.Description != "" && rel.EvidenceChunkID != 0 {
			hasConsumes = true
		}
	}
	if !hasWrites || !hasConsumes {
		t.Fatalf("relations=%#v, want typed WRITES and CONSUMES relations with description and evidence chunk", repo.relations)
	}
}

func TestGraphRAGQueryExpandsOneHopSubgraph(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")
	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "Agent 调度链路",
		Content:    "agent-manager-service 消费 claran.im.events，并调用 agent-runtime-service。agent-manager-service 写入 agent_dispatch_records 用于幂等调度。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}

	graph, err := svc.GetGraph(context.Background(), GraphInput{ViewerID: 1001, Query: "agent_dispatch_records", Limit: 20})
	if err != nil {
		t.Fatalf("GetGraph returned error: %v", err)
	}
	names := map[string]bool{}
	for _, node := range graph.Nodes {
		names[node.Name] = true
	}
	if !names["agent_dispatch_records"] || !names["agent-manager-service"] {
		t.Fatalf("graph nodes=%#v, want seed entity and one-hop neighbor", graph.Nodes)
	}
	if len(graph.Edges) == 0 {
		t.Fatalf("graph edges empty, want one-hop relation")
	}
	if len(graph.Communities) == 0 || !strings.Contains(graph.Communities[0].Summary, "实体") {
		t.Fatalf("communities=%#v, want community summary", graph.Communities)
	}
}

func TestSearchUsesAdaptiveRuleRouterToSkipRetrieval(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "adaptive")

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID: 1001,
		Query:    "你好，介绍一下自己",
		Mode:     "adaptive",
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if result.Route != "direct" {
		t.Fatalf("Route = %q, want direct", result.Route)
	}
	if result.SelfCheck == nil || result.SelfCheck.Retrieve {
		t.Fatalf("SelfCheck.Retrieve = %v, want false", result.SelfCheck)
	}
	if !strings.Contains(result.SelfCheck.Note, AdaptiveRouteDirect) {
		t.Fatalf("SelfCheck.Note = %q, want router reason", result.SelfCheck.Note)
	}
	if len(result.Sources) != 0 {
		t.Fatalf("Sources len = %d, want 0", len(result.Sources))
	}
}

func TestRuleRouterClassifiesObviousAdaptiveRoutes(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantRoute  string
		wantMode   string
		wantSource string
		wantNeed   bool
	}{
		{name: "greeting", query: "你好", wantRoute: AdaptiveRouteDirect, wantMode: "direct", wantSource: "none", wantNeed: false},
		{name: "latest", query: "最新 Go 版本是多少", wantRoute: AdaptiveRouteWebRAG, wantMode: "web", wantSource: "web", wantNeed: true},
		{name: "project", query: "当前项目里的 agent_dispatch_records event_id 是什么意思", wantRoute: AdaptiveRouteProjectRAG, wantMode: "hybrid", wantSource: "project_docs", wantNeed: true},
		{name: "memory", query: "根据我的偏好回答这个问题", wantRoute: AdaptiveRouteMemoryRAG, wantMode: "memory", wantSource: "memory", wantNeed: true},
		{name: "action", query: "帮我创建一个任务并提醒负责人", wantRoute: AdaptiveRouteToolAction, wantMode: "tool_action", wantSource: "none", wantNeed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, uncertain := classifyByRules(tt.query, "adaptive")
			if uncertain {
				t.Fatalf("classifyByRules returned uncertain for obvious query %q", tt.query)
			}
			if decision.Route != tt.wantRoute || decision.Mode != tt.wantMode || decision.RetrievalSource != tt.wantSource || decision.Retrieve != tt.wantNeed {
				t.Fatalf("decision = %#v, want route=%s mode=%s source=%s need=%v", decision, tt.wantRoute, tt.wantMode, tt.wantSource, tt.wantNeed)
			}
		})
	}
}

func TestHybridAdaptiveRouterUsesRulesBeforeLLM(t *testing.T) {
	called := false
	router := HybridAdaptiveRouter{LLM: RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		called = true
		return RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: "hybrid", Retrieve: true}, nil
	})}
	decision, err := router.Route(context.Background(), RouterInput{Query: "最新 Kafka 版本是多少", DefaultMode: "adaptive"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if called {
		t.Fatalf("LLM router should not be called for obvious latest query")
	}
	if decision.Route != AdaptiveRouteWebRAG || decision.Mode != "web" || !decision.Retrieve {
		t.Fatalf("decision = %#v, want web route", decision)
	}
}

func TestHybridAdaptiveRouterFallsBackToLLMWhenRulesUncertain(t *testing.T) {
	called := false
	router := HybridAdaptiveRouter{LLM: RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		called = true
		return RouterDecision{Route: AdaptiveRouteStrictRAG, Mode: "hybrid", Retrieve: true, Strategy: "strict_rag", Complexity: "high", RetrievalSource: "project_docs", Reason: "LLM classifier"}, nil
	})}
	decision, err := router.Route(context.Background(), RouterInput{Query: "请分析这个设计是否合理", DefaultMode: "adaptive"})
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	if !called {
		t.Fatalf("LLM router should be called for uncertain query")
	}
	if decision.Route != AdaptiveRouteStrictRAG || decision.Strategy != "strict_rag" || decision.Complexity != "high" {
		t.Fatalf("decision = %#v, want LLM classifier decision", decision)
	}
}

func TestSearchPrefersUserRAGRouterProfile(t *testing.T) {
	repo := newFakeRAGRepo()
	userRouter := RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		_ = ctx
		return RouterDecision{Route: AdaptiveRouteDirect, Mode: "direct", Retrieve: false, Reason: "用户配置的小模型判断无需检索"}, nil
	})
	defaultRouter := RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		t.Fatalf("default router should not be used when user rag_router profile resolves")
		return RouterDecision{}, nil
	})
	svc := NewRAGServiceWithRouterProvider(repo, NewLocalVectorIndex(), 64, "adaptive", nil, defaultRouter, nil, nil, nil, &fakeRouterSettings{
		profiles: []settingsclient.LLMProfile{{ID: 77, Name: "自定义RAG路由", UsageType: settingsclient.ProviderRAGRouter, IsDefault: true, Enabled: true}},
		resolved: settingsclient.ResolvedLLMConfig{
			ProfileID: 77,
			APIKey:    "user-router-key",
			BaseURL:   "https://router.example/v1",
			ModelName: "glm-4-flash",
		},
	}, func(cfg settingsclient.ResolvedLLMConfig) RAGRouter {
		if cfg.ProfileID != 77 || cfg.APIKey == "" || cfg.BaseURL == "" || cfg.ModelName != "glm-4-flash" {
			t.Fatalf("router cfg = %#v", cfg)
		}
		return userRouter
	})

	result, err := svc.Search(context.Background(), SearchInput{ViewerID: 1001, Query: "请评价一下这个想法", Mode: "adaptive"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if result.Route != "direct" || result.SelfCheck == nil || result.SelfCheck.Retrieve {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.SelfCheck.Note, "用户配置的小模型") {
		t.Fatalf("note = %q", result.SelfCheck.Note)
	}
}

func TestSearchFallsBackToDefaultRouterWhenNoUserRAGRouterProfile(t *testing.T) {
	repo := newFakeRAGRepo()
	defaultRouter := RouterFunc(func(ctx context.Context, input RouterInput) (RouterDecision, error) {
		_ = ctx
		return RouterDecision{Route: AdaptiveRouteDirect, Mode: "direct", Retrieve: false, Reason: "项目内置小模型判断无需检索"}, nil
	})
	svc := NewRAGServiceWithRouterProvider(repo, NewLocalVectorIndex(), 64, "adaptive", nil, defaultRouter, nil, nil, nil, &fakeRouterSettings{}, nil)

	result, err := svc.Search(context.Background(), SearchInput{ViewerID: 1001, Query: "请评价一下这个想法", Mode: "adaptive"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if result.Route != "direct" || result.SelfCheck == nil || result.SelfCheck.Retrieve {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.SelfCheck.Note, "项目内置小模型") {
		t.Fatalf("note = %q", result.SelfCheck.Note)
	}
}

type fakeRAGRepo struct {
	nextID      int64
	docs        []model.Document
	chunks      []model.Chunk
	entities    []model.Entity
	relations   []model.Relation
	communities []model.Community
}

type recordingReranker struct {
	scoresByTitle  map[string]float64
	calledWithTopN int
	candidateCount int
}

type recordingCRAGEvaluator struct {
	evaluation  CRAGEvaluation
	called      bool
	query       string
	sourceCount int
}

type recordingSelfRAGJudge struct {
	result SelfRAGJudgement
	called bool
	query  string
}

func (j *recordingSelfRAGJudge) Judge(ctx context.Context, input SelfRAGJudgeInput) (SelfRAGJudgement, error) {
	_ = ctx
	j.called = true
	j.query = input.Query
	return j.result, nil
}

func (e *recordingCRAGEvaluator) Evaluate(ctx context.Context, input CRAGEvaluateInput) (CRAGEvaluation, error) {
	_ = ctx
	e.called = true
	e.query = input.Query
	e.sourceCount = len(input.Chunks)
	return e.evaluation, nil
}

func (r *recordingReranker) Rerank(ctx context.Context, query string, chunks []rankedChunk, topN int) ([]RerankScore, error) {
	_ = ctx
	_ = query
	r.calledWithTopN = topN
	r.candidateCount = len(chunks)
	scores := make([]RerankScore, 0, len(chunks))
	for i, chunk := range chunks {
		score := r.scoresByTitle[chunk.Document.Title]
		scores = append(scores, RerankScore{Index: i, Score: score})
	}
	return scores, nil
}

func newFakeRAGRepo() *fakeRAGRepo {
	return &fakeRAGRepo{nextID: 1}
}

func (r *fakeRAGRepo) allocID() int64 {
	id := r.nextID
	r.nextID++
	return id
}

func (r *fakeRAGRepo) CreateDocumentWithChunks(ctx context.Context, doc *model.Document, chunks []model.Chunk) error {
	_ = ctx
	if doc.ID == 0 {
		doc.ID = r.allocID()
	}
	now := time.Now()
	doc.CreatedAt = now
	doc.UpdatedAt = now
	r.docs = append(r.docs, *doc)
	for i := range chunks {
		if chunks[i].ID == 0 {
			chunks[i].ID = r.allocID()
		}
		chunks[i].DocumentID = doc.ID
		chunks[i].OwnerID = doc.OwnerID
		chunks[i].GroupID = doc.GroupID
		chunks[i].ConversationID = doc.ConversationID
		chunks[i].CreatedAt = now
		chunks[i].UpdatedAt = now
		r.chunks = append(r.chunks, chunks[i])
	}
	return nil
}

func (r *fakeRAGRepo) ListDocuments(ctx context.Context, filter dao.SearchFilter) ([]model.Document, int64, error) {
	_ = ctx
	var out []model.Document
	for _, doc := range r.docs {
		if doc.OwnerID == filter.ViewerID || doc.Visibility == model.VisibilityPublic {
			out = append(out, doc)
		}
	}
	return out, int64(len(out)), nil
}

func (r *fakeRAGRepo) ListChunks(ctx context.Context, filter dao.SearchFilter) ([]dao.ChunkWithDocument, error) {
	_ = ctx
	docByID := map[int64]model.Document{}
	for _, doc := range r.docs {
		docByID[doc.ID] = doc
	}
	out := make([]dao.ChunkWithDocument, 0, len(r.chunks))
	for _, chunk := range r.chunks {
		doc := docByID[chunk.DocumentID]
		if doc.ID == 0 {
			continue
		}
		if doc.OwnerID != filter.ViewerID && doc.Visibility != model.VisibilityPublic {
			continue
		}
		if filter.GroupID > 0 && doc.GroupID != 0 && doc.GroupID != filter.GroupID {
			continue
		}
		if filter.ConversationID > 0 && doc.ConversationID != 0 && doc.ConversationID != filter.ConversationID {
			continue
		}
		out = append(out, dao.ChunkWithDocument{Chunk: chunk, Document: doc})
	}
	return out, nil
}

func (r *fakeRAGRepo) GetEntityByName(ctx context.Context, ownerID int64, name string) (*model.Entity, error) {
	_ = ctx
	for i := range r.entities {
		if r.entities[i].OwnerID == ownerID && r.entities[i].Name == name {
			return &r.entities[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRAGRepo) GetEntityByCanonicalKey(ctx context.Context, ownerID int64, canonicalKey string) (*model.Entity, error) {
	_ = ctx
	for i := range r.entities {
		if r.entities[i].OwnerID == ownerID && r.entities[i].CanonicalKey == canonicalKey {
			return &r.entities[i], nil
		}
	}
	return nil, nil
}

func (r *fakeRAGRepo) SaveEntity(ctx context.Context, entity *model.Entity) error {
	_ = ctx
	if entity.ID == 0 {
		entity.ID = r.allocID()
		r.entities = append(r.entities, *entity)
		return nil
	}
	for i := range r.entities {
		if r.entities[i].ID == entity.ID {
			r.entities[i] = *entity
			return nil
		}
	}
	r.entities = append(r.entities, *entity)
	return nil
}

func (r *fakeRAGRepo) SaveRelation(ctx context.Context, relation *model.Relation) error {
	_ = ctx
	if relation.ID == 0 {
		relation.ID = r.allocID()
	}
	r.relations = append(r.relations, *relation)
	return nil
}

func (r *fakeRAGRepo) SaveCommunity(ctx context.Context, community *model.Community) error {
	_ = ctx
	if community.ID == 0 {
		community.ID = r.allocID()
		r.communities = append(r.communities, *community)
		return nil
	}
	for i := range r.communities {
		if r.communities[i].ID == community.ID {
			r.communities[i] = *community
			return nil
		}
	}
	r.communities = append(r.communities, *community)
	return nil
}

func (r *fakeRAGRepo) ListGraph(ctx context.Context, viewerID int64, query string, limit int) ([]model.Entity, []model.Relation, []model.Community, error) {
	_ = ctx
	if limit <= 0 {
		limit = 80
	}
	query = strings.ToLower(strings.TrimSpace(query))
	canonicalQuery := canonicalEntityKey(query)
	seeds := make([]model.Entity, 0, len(r.entities))
	for _, entity := range r.entities {
		if entity.OwnerID != viewerID && entity.OwnerID != 0 {
			continue
		}
		searchText := strings.ToLower(entity.Name + " " + entity.Summary + " " + entity.AliasesJSON + " " + entity.CanonicalKey)
		if query != "" && !strings.Contains(searchText, query) && !strings.Contains(entity.CanonicalKey, canonicalQuery) {
			continue
		}
		seeds = append(seeds, entity)
		if len(seeds) >= limit {
			break
		}
	}
	if len(seeds) == 0 && query != "" {
		for _, entity := range r.entities {
			if entity.OwnerID == viewerID || entity.OwnerID == 0 {
				seeds = append(seeds, entity)
			}
			if len(seeds) >= limit {
				break
			}
		}
	}
	entityByID := map[int64]model.Entity{}
	seedIDs := map[int64]bool{}
	for _, entity := range seeds {
		entityByID[entity.ID] = entity
		seedIDs[entity.ID] = true
	}
	var relations []model.Relation
	for _, relation := range r.relations {
		if relation.OwnerID != viewerID && relation.OwnerID != 0 {
			continue
		}
		if !seedIDs[relation.SourceID] && !seedIDs[relation.TargetID] {
			continue
		}
		relations = append(relations, relation)
		for _, entity := range r.entities {
			if entity.ID == relation.SourceID || entity.ID == relation.TargetID {
				entityByID[entity.ID] = entity
			}
		}
	}
	entities := make([]model.Entity, 0, len(entityByID))
	communityIDs := map[int64]bool{}
	for _, entity := range entityByID {
		entities = append(entities, entity)
		if entity.CommunityID > 0 {
			communityIDs[entity.CommunityID] = true
		}
	}
	var communities []model.Community
	for _, community := range r.communities {
		if communityIDs[community.ID] {
			communities = append(communities, community)
		}
	}
	return entities, relations, communities, nil
}

type fakeRouterSettings struct {
	profiles []settingsclient.LLMProfile
	resolved settingsclient.ResolvedLLMConfig
}

func (s *fakeRouterSettings) SaveLLMProfile(ctx context.Context, ownerID int64, input settingsclient.SaveLLMProfileInput) (*settingsclient.LLMProfile, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ListLLMProfiles(ctx context.Context, ownerID int64, usageType string) ([]settingsclient.LLMProfile, error) {
	if usageType != settingsclient.ProviderRAGRouter {
		return nil, nil
	}
	return s.profiles, nil
}

func (s *fakeRouterSettings) DeleteLLMProfile(ctx context.Context, ownerID, profileID int64) error {
	return nil
}

func (s *fakeRouterSettings) SavePrompt(ctx context.Context, ownerID int64, input settingsclient.SavePromptInput) (*settingsclient.PromptTemplate, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ListPrompts(ctx context.Context, ownerID int64) ([]settingsclient.PromptTemplate, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ResolveTranslationConfig(ctx context.Context, ownerID int64) (settingsclient.ResolvedLLMConfig, error) {
	return settingsclient.ResolvedLLMConfig{}, nil
}

func (s *fakeRouterSettings) ResolveLLMProfile(ctx context.Context, ownerID, profileID int64) (settingsclient.ResolvedLLMConfig, error) {
	return s.resolved, nil
}

func (s *fakeRouterSettings) SaveSkill(ctx context.Context, ownerID int64, input settingsclient.SaveSkillInput) (*settingsclient.AgentSkill, error) {
	return nil, nil
}

func (s *fakeRouterSettings) GetSkill(ctx context.Context, ownerID, skillID int64) (*settingsclient.AgentSkill, error) {
	return nil, nil
}

func (s *fakeRouterSettings) UpdateSkillContent(ctx context.Context, ownerID, skillID int64, name, description string, content []byte) (*settingsclient.AgentSkill, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ListSkills(ctx context.Context, ownerID int64, scope string, agentID int64) ([]settingsclient.AgentSkill, error) {
	return nil, nil
}

func (s *fakeRouterSettings) DeleteSkill(ctx context.Context, ownerID, skillID int64) error {
	return nil
}
