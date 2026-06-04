package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"fmt"
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
		Content:    "agent-manager-service 写入 agent_dispatch_records，并调用 agent-runtime-service。agent-manager-service 消费 claran.im.events。",
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
	if !strings.Contains(result.Answer, "根据知识库") || !strings.Contains(result.Answer, "Agent RAG 手册") {
		t.Fatalf("Answer = %q, want user-facing synthesized RAG answer with source title", result.Answer)
	}
}

func TestIngestDocumentSplitsSingleNewlineStructuredText(t *testing.T) {
	repo := newFakeRAGRepo()
	index := NewLocalVectorIndex()
	svc := NewRAGService(repo, index, 64, "hybrid")
	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("第%d段说明：这里模拟 DOCX 或 OCR 解析后的单换行长文本，内容包含 RAG 分片、父块摘要、子块检索和权限过滤。", i+1))
	}
	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "单换行文档",
		Content:    strings.Join(lines, "\n"),
		SourceType: "docx",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	parentCount := 0
	childCount := 0
	for _, chunk := range repo.chunks {
		switch normalizedChunkLevel(chunk) {
		case model.ChunkLevelParent:
			parentCount++
		case model.ChunkLevelChild:
			childCount++
		}
	}
	if parentCount < 2 {
		t.Fatalf("parent chunks = %d, want multiple parent chunks for single-newline document", parentCount)
	}
	if childCount < 4 || result.ChunkCount != int64(childCount) {
		t.Fatalf("child chunks = %d result=%d, want multiple child chunks and accurate count", childCount, result.ChunkCount)
	}
	hits, err := index.Search(context.Background(), hashEmbedding("父块摘要 子块检索 权限过滤", 64), 200)
	if err != nil {
		t.Fatalf("index.Search returned error: %v", err)
	}
	if len(hits) != childCount {
		t.Fatalf("indexed child chunks = %d, want %d", len(hits), childCount)
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

func TestDocumentSearchRestrictsSourcesToSelectedDocument(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "adaptive")
	first, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "Go 并发学习笔记",
		Content:    "学习 Go 并发要理解 goroutine、channel、select 和调度器。这个文档不包含融媒体中心面试简历。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("first IngestDocument returned error: %v", err)
	}
	second, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "融媒体中心面试简历",
		Content:    "融媒体中心面试简历应突出新闻采编、短视频剪辑、公众号运营、活动策划和跨部门沟通经历。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("second IngestDocument returned error: %v", err)
	}
	if first.Document.Id == 0 || second.Document.Id == 0 || first.Document.Id == second.Document.Id {
		t.Fatalf("document ids invalid: first=%d second=%d", first.Document.Id, second.Document.Id)
	}

	result, err := svc.Search(context.Background(), SearchInput{
		ViewerID:   1001,
		Query:      "如何写这份简历",
		Mode:       "document",
		DocumentID: second.Document.Id,
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("sources empty, want selected document hit")
	}
	for _, source := range result.Sources {
		if source.DocumentId != second.Document.Id {
			t.Fatalf("source document_id=%d title=%q, want only selected document_id=%d", source.DocumentId, source.Title, second.Document.Id)
		}
		if source.DocumentId == first.Document.Id {
			t.Fatalf("source leaked first document: %#v", source)
		}
	}
	if !strings.Contains(result.Answer, "融媒体中心面试简历") {
		t.Fatalf("answer=%q, want selected document title", result.Answer)
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
	if !strings.Contains(msgCore.AliasesJSON, "msg core service") {
		t.Fatalf("aliases = %q, want merged strong English aliases", msgCore.AliasesJSON)
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

func TestParseGraphExtractResultAcceptsLLMJSONAndKeepsEvidence(t *testing.T) {
	result, err := parseGraphExtractResult(`模型输出如下：
{
  "entities": [
    {
      "name": "msg-core-service",
      "type": "Service",
      "description": "消息核心服务，负责消息事实写入和事件发布",
      "aliases": ["消息核心服务", "MessageService"]
    },
    {
      "name": "event_outbox",
      "type": "DatabaseTable",
      "description": "事务Outbox表",
      "aliases": []
    }
  ],
  "relationships": [
    {
      "source": "msg-core-service",
      "target": "event_outbox",
      "type": "writes",
      "description": "msg-core-service 在消息事务中写入 event_outbox",
      "evidence": "发送消息时同事务写入 messages 和 event_outbox",
      "confidence": 0.91
    },
    {
      "source": "msg-core-service",
      "target": "missing-node",
      "type": "calls",
      "description": "非法关系应被丢弃",
      "evidence": "",
      "confidence": 0.4
    }
  ]
}`)
	if err != nil {
		t.Fatalf("parseGraphExtractResult returned error: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Fatalf("entities len = %d, want 2", len(result.Entities))
	}
	if result.Entities[0].Type != "Service" || !strings.Contains(strings.Join(result.Entities[0].Aliases, ","), "消息核心服务") {
		t.Fatalf("first entity = %#v, want Service with aliases", result.Entities[0])
	}
	if len(result.Relationships) != 1 {
		t.Fatalf("relationships len = %d, want invalid relation filtered", len(result.Relationships))
	}
	rel := result.Relationships[0]
	if rel.Type != "WRITES" || rel.Confidence != 0.91 || !strings.Contains(rel.Evidence, "event_outbox") {
		t.Fatalf("relationship = %#v, want normalized WRITES with confidence and evidence", rel)
	}
}

func TestFilterGraphExtractResultDropsNoiseAndKeepsGroundedRelations(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "agent-manager-service", Type: "Service", Description: "Agent 管理服务"},
			{Name: "agent_dispatch_records", Type: "DatabaseTable", Description: "Agent 调度幂等表"},
			{Name: "agent-runtime-service", Type: "Service", Description: "Agent 运行服务"},
			{Name: "截图", Type: "Concept", Description: "无意义噪声"},
			{Name: "35ec16e3cc97f5f5dd482af5d901220d.png", Type: "Concept", Description: "图片文件名"},
			{Name: "页面", Type: "Concept", Description: "泛词"},
		},
		Relationships: []extractedRelationship{
			{Source: "agent-manager-service", Target: "agent_dispatch_records", Type: "WRITES", Description: "agent-manager-service 写入 agent_dispatch_records", Evidence: "agent-manager-service 写入 agent_dispatch_records", Confidence: 0.91},
			{Source: "agent-manager-service", Target: "agent-runtime-service", Type: "CALLS", Description: "agent-manager-service 调用 agent-runtime-service", Evidence: "agent-manager-service 调用 agent-runtime-service", Confidence: 0.89},
			{Source: "截图", Target: "页面", Type: "RELATED_TO", Description: "噪声关系", Evidence: "截图和页面", Confidence: 0.95},
		},
	}, "agent-manager-service 写入 agent_dispatch_records，并调用 agent-runtime-service。截图 文件 页面 35ec16e3cc97f5f5dd482af5d901220d.png")

	names := map[string]bool{}
	for _, entity := range result.Entities {
		names[entity.Name] = true
	}
	if !names["agent-manager-service"] || !names["agent_dispatch_records"] || !names["agent-runtime-service"] {
		t.Fatalf("entities=%#v, want useful service/table entities kept", result.Entities)
	}
	if names["截图"] || names["页面"] || names["35ec16e3cc97f5f5dd482af5d901220d.png"] {
		t.Fatalf("entities=%#v, want screenshot/page/file noise dropped", result.Entities)
	}
	if len(result.Relationships) != 2 {
		t.Fatalf("relationships=%#v, want only grounded technical relations", result.Relationships)
	}
	relations := map[string]bool{}
	for _, relation := range result.Relationships {
		relations[relation.Source+" "+relation.Type+" "+relation.Target] = true
	}
	if !relations["agent-manager-service WRITES agent_dispatch_records"] || !relations["agent-manager-service CALLS agent-runtime-service"] {
		t.Fatalf("relationships=%#v, want WRITES and CALLS technical relations", result.Relationships)
	}
}

func TestFilterGraphExtractResultKeepsUsefulEntitiesWithoutRelations(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "融媒体中心面试简历", Type: "Concept", Description: "文档主题"},
			{Name: "个人经历", Type: "Concept", Description: "简历中的经历信息"},
			{Name: "求职目标", Type: "Concept", Description: "候选人的求职目标"},
		},
		Relationships: nil,
	}, "这份文档主要讨论融媒体中心面试简历、个人经历和求职目标。")

	if len(result.Relationships) != 0 {
		t.Fatalf("relationships=%#v, want no unsupported relation", result.Relationships)
	}
	names := map[string]bool{}
	for _, entity := range result.Entities {
		names[entity.Name] = true
	}
	if !names["融媒体中心面试简历"] {
		t.Fatalf("entities=%#v, want useful document-specific entity kept even when relation extraction is empty", result.Entities)
	}
	if names["个人经历"] || names["求职目标"] {
		t.Fatalf("entities=%#v, want generic resume section entities still filtered", result.Entities)
	}
}

func TestFilterGraphExtractResultRequiresStrongRelationshipEvidence(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "GraphRAG", Type: "Concept", Description: "图谱增强检索方法"},
			{Name: "Milvus", Type: "Product", Description: "向量数据库"},
			{Name: "课堂示例", Type: "Concept", Description: "普通示例"},
		},
		Relationships: []extractedRelationship{
			{Source: "GraphRAG", Target: "Milvus", Type: "DEPENDS_ON", Description: "GraphRAG 结合 Milvus 做召回", Evidence: "GraphRAG 结合 Milvus 做召回", Confidence: 0.91},
			{Source: "课堂示例", Target: "Milvus", Type: "RELATED_TO", Description: "同段出现", Evidence: "课堂示例中提到了 Milvus", Confidence: 0.6},
		},
	}, "GraphRAG 结合 Milvus 做召回。课堂示例中提到了 Milvus。")

	names := map[string]bool{}
	for _, entity := range result.Entities {
		names[entity.Name] = true
	}
	if !names["GraphRAG"] || !names["Milvus"] {
		t.Fatalf("entities=%#v, want strongly related entities kept", result.Entities)
	}
	if names["课堂示例"] {
		t.Fatalf("entities=%#v, want weak related example entity dropped", result.Entities)
	}
	if len(result.Relationships) != 1 || result.Relationships[0].Type != "DEPENDS_ON" {
		t.Fatalf("relationships=%#v, want only strong DEPENDS_ON kept", result.Relationships)
	}
}

func TestFilterGraphExtractResultRejectsWeakOnlyRelatedToGraph(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "融媒体中心面试简历", Type: "Concept", Description: "文档主题"},
			{Name: "求职目标", Type: "Concept", Description: "简历栏目"},
		},
		Relationships: []extractedRelationship{
			{Source: "融媒体中心面试简历", Target: "求职目标", Type: "RELATED_TO", Description: "同一份简历中出现", Evidence: "融媒体中心面试简历包含求职目标", Confidence: 0.95},
		},
	}, "融媒体中心面试简历包含求职目标。")

	if len(result.Relationships) != 0 {
		t.Fatalf("relationships=%#v, want weak RELATED_TO relation rejected", result.Relationships)
	}
	names := map[string]bool{}
	for _, entity := range result.Entities {
		names[entity.Name] = true
	}
	if !names["融媒体中心面试简历"] {
		t.Fatalf("entities=%#v, want useful root topic entity kept for document graph visibility", result.Entities)
	}
	if names["求职目标"] {
		t.Fatalf("entities=%#v, want generic section entity filtered", result.Entities)
	}
}

func TestFilterGraphExtractResultRejectsGenericRelatedEvenWithHighConfidence(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "融媒体中心面试简历", Type: "文档", Description: "面向融媒体中心岗位的简历文档"},
			{Name: "新闻采编能力", Type: "能力", Description: "简历中的岗位能力重点"},
		},
		Relationships: []extractedRelationship{
			{Source: "融媒体中心面试简历", Target: "新闻采编能力", Type: "RELATED_TO", Description: "新闻采编能力是融媒体中心面试简历的核心能力维度", Evidence: "简历需要体现新闻采编能力、内容策划能力和新媒体运营能力", Confidence: 0.94},
		},
	}, "简历需要体现新闻采编能力、内容策划能力和新媒体运营能力。")

	if len(result.Relationships) != 0 {
		t.Fatalf("relationships=%#v, want generic RELATED_TO rejected; LLM should output a concrete relation such as 需要体现", result.Relationships)
	}
	if len(result.Entities) != 2 {
		t.Fatalf("entities=%#v, want useful entities preserved while generic relation is dropped", result.Entities)
	}
}

func TestFilterGraphExtractResultPreservesBroaderLLMExtraction(t *testing.T) {
	entities := []extractedEntity{{Name: "agent-manager-service", Type: "Service", Description: "Agent 管理服务"}}
	relations := make([]extractedRelationship, 0, 18)
	evidenceParts := []string{"agent-manager-service"}
	for i := 0; i < 18; i++ {
		target := fmt.Sprintf("agent_table_%02d_records", i)
		entities = append(entities, extractedEntity{Name: target, Type: "DatabaseTable", Description: "Agent 数据表"})
		relations = append(relations, extractedRelationship{
			Source:      "agent-manager-service",
			Target:      target,
			Type:        "WRITES",
			Description: "agent-manager-service 写入 " + target,
			Evidence:    "agent-manager-service 写入 " + target,
			Confidence:  0.9 - float64(i)*0.01,
		})
		evidenceParts = append(evidenceParts, target)
	}
	result := filterGraphExtractResult(graphExtractResult{Entities: entities, Relationships: relations}, strings.Join(evidenceParts, " "))
	if len(result.Relationships) != 18 {
		t.Fatalf("relationships len=%d, want broader LLM extraction preserved", len(result.Relationships))
	}
	if len(result.Entities) != 19 {
		t.Fatalf("entities len=%d, want broader connected entity set preserved", len(result.Entities))
	}
}

func TestParseGraphExtractResultPreservesLLMCustomRelationType(t *testing.T) {
	result, err := parseGraphExtractResult(`{
  "entities": [
    {"name": "面试者", "type": "岗位候选人", "description": "简历中的候选人", "aliases": []},
    {"name": "职业装", "type": "服装", "description": "面试者穿着的服装", "aliases": []}
  ],
  "relationships": [
    {"source": "面试者", "target": "职业装", "type": "穿着", "description": "面试者穿着职业装参加面试", "evidence": "面试者穿着职业装", "confidence": 0.91}
  ]
}`)
	if err != nil {
		t.Fatalf("parseGraphExtractResult returned error: %v", err)
	}
	if len(result.Relationships) != 1 || result.Relationships[0].Type != "穿着" {
		t.Fatalf("relationships=%#v, want custom LLM relation type preserved", result.Relationships)
	}
	if result.Entities[0].Type != "岗位候选人" || result.Entities[1].Type != "服装" {
		t.Fatalf("entities=%#v, want custom LLM entity types preserved", result.Entities)
	}
	filtered := filterGraphExtractResult(result, "面试者穿着职业装参加面试。")
	if len(filtered.Relationships) != 1 || filtered.Relationships[0].Type != "穿着" {
		t.Fatalf("filtered relationships=%#v, want custom LLM relation type kept through post-filter", filtered.Relationships)
	}
	if len(filtered.Entities) != 2 || filtered.Entities[0].Type == "Concept" || filtered.Entities[1].Type == "Concept" {
		t.Fatalf("filtered entities=%#v, want custom LLM entity types not collapsed to Concept", filtered.Entities)
	}
}

func TestGraphExtractResultPreservesFreeEntityTypesAndRelations(t *testing.T) {
	result, err := parseGraphExtractResult(`{
  "entities": [
    {"name": "融媒体中心面试简历", "type": "文档", "description": "面向融媒体中心岗位的简历材料", "aliases": []},
    {"name": "新闻采编能力", "type": "能力", "description": "简历需要体现的核心能力", "aliases": []},
    {"name": "融媒体中心岗位", "type": "岗位", "description": "简历投递的目标岗位", "aliases": []}
  ],
  "relationships": [
    {"source": "融媒体中心面试简历", "target": "新闻采编能力", "type": "需要体现", "description": "简历需要体现新闻采编能力", "evidence": "融媒体中心面试简历需要体现新闻采编能力", "confidence": 0.93},
    {"source": "新闻采编能力", "target": "融媒体中心岗位", "type": "面向", "description": "新闻采编能力面向融媒体中心岗位要求", "evidence": "新闻采编能力适配融媒体中心岗位", "confidence": 0.9}
  ]
}`)
	if err != nil {
		t.Fatalf("parseGraphExtractResult returned error: %v", err)
	}
	filtered := filterGraphExtractResult(result, "融媒体中心面试简历需要体现新闻采编能力，并面向融媒体中心岗位要求。")
	types := map[string]bool{}
	relations := map[string]bool{}
	for _, entity := range filtered.Entities {
		types[entity.Type] = true
	}
	for _, relation := range filtered.Relationships {
		relations[relation.Type] = true
	}
	for _, want := range []string{"文档", "能力", "岗位"} {
		if !types[want] {
			t.Fatalf("types=%#v entities=%#v, want free entity type %s preserved", types, filtered.Entities, want)
		}
	}
	for _, want := range []string{"需要体现", "面向"} {
		if !relations[want] {
			t.Fatalf("relations=%#v raw=%#v, want free relation %s preserved", relations, filtered.Relationships, want)
		}
	}
}

func TestGraphExtractResultDropsGenericRelationLabels(t *testing.T) {
	result, err := parseGraphExtractResult(`{
  "entities": [
    {"name": "测试实体A", "type": "主题", "description": "测试实体A", "aliases": []},
    {"name": "测试实体B", "type": "主题", "description": "测试实体B", "aliases": []}
  ],
  "relationships": [
    {"source": "测试实体A", "target": "测试实体B", "type": "相关", "description": "测试实体A和测试实体B只是同段出现", "evidence": "测试实体A和测试实体B只是同段出现", "confidence": 0.95}
  ]
}`)
	if err != nil {
		t.Fatalf("parseGraphExtractResult returned error: %v", err)
	}
	filtered := filterGraphExtractResult(result, "测试实体A和测试实体B只是同段出现。")
	if len(filtered.Relationships) != 0 {
		t.Fatalf("relationships=%#v, want generic relation dropped instead of collapsed to RELATED_TO", filtered.Relationships)
	}
	if len(filtered.Entities) != 2 {
		t.Fatalf("entities=%#v, want useful entities preserved for graph diagnosis", filtered.Entities)
	}
}

func TestFallbackTopicGraphUsesDocumentRootAndContainsInsteadOfRelatedTo(t *testing.T) {
	repo := newFakeRAGRepo()
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid").(*ragServiceImpl)
	chunk := model.Chunk{
		ID:           1,
		DocumentID:   99,
		OwnerID:      1001,
		ChunkLevel:   model.ChunkLevelParent,
		Summary:      "RAG 系统设计",
		Content:      "RAG 系统设计包括检索策略、向量召回、权限过滤、答案生成和来源治理，这些主题需要在知识库工作台中清晰展示。",
		QualityScore: 0.9,
	}
	_, relationCount := svc.buildFallbackTopicGraph(context.Background(), 1001, 99, "RAG 系统设计文档", []model.Chunk{chunk})
	if relationCount == 0 {
		t.Fatalf("relationCount=0, want fallback document-root graph")
	}
	hasDocumentRoot := false
	for _, entity := range repo.entities {
		if entity.CanonicalKey == "document:99" {
			hasDocumentRoot = true
			break
		}
	}
	if !hasDocumentRoot {
		t.Fatalf("entities=%#v, want document title root node", repo.entities)
	}
	for _, relation := range repo.relations {
		if relation.Relation == "RELATED_TO" {
			t.Fatalf("relations=%#v, fallback graph must not create RELATED_TO skeleton", repo.relations)
		}
	}
}

func TestRuleGraphExtractorBuildsMultipleDirectedRelationsFromSentence(t *testing.T) {
	entities := []extractedEntity{
		{Name: "agent-manager-service", Type: "Service", Aliases: []string{"agent-manager-service"}},
		{Name: "agent_dispatch_records", Type: "DatabaseTable", Aliases: []string{"agent_dispatch_records"}},
		{Name: "agent-runtime-service", Type: "Service", Aliases: []string{"agent-runtime-service"}},
		{Name: "claran.im.events", Type: "EventTopic", Aliases: []string{"claran.im.events"}},
	}
	relations := extractGraphRelationships("agent-manager-service 消费 claran.im.events，写入 agent_dispatch_records，并调用 agent-runtime-service。", entities)
	got := map[string]bool{}
	for _, relation := range relations {
		got[relation.Source+" "+relation.Type+" "+relation.Target] = true
	}
	for _, want := range []string{
		"agent-manager-service CONSUMES claran.im.events",
		"agent-manager-service WRITES agent_dispatch_records",
		"agent-manager-service CALLS agent-runtime-service",
	} {
		if !got[want] {
			t.Fatalf("relations=%#v, want %s", relations, want)
		}
	}
}

func TestRuleGraphExtractorDropsMetadataFieldsStatusAndFileNames(t *testing.T) {
	result, err := (ruleGraphExtractor{}).Extract(context.Background(), graphExtractInput{
		Chunk: model.Chunk{Content: strings.Join([]string{
			"agent-manager-service 写入 agent_dispatch_records，并发布 claran.im.events。",
			"字段 event_id、client_msg_id、status、created_at 用于审计，不应该成为知识图谱节点。",
			"文件 internal/agent-manager-service/eventconsumer/agent_consumer.go 和截图 35ec16e3cc97f5f5dd482af5d901220d.png 只是证据来源。",
			"状态 pending、completed、failed 只是枚举值。",
		}, "\n")},
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	filtered := filterGraphExtractResult(result, strings.Join([]string{
		"agent-manager-service 写入 agent_dispatch_records，并发布 claran.im.events。",
		"字段 event_id、client_msg_id、status、created_at 用于审计，不应该成为知识图谱节点。",
		"文件 internal/agent-manager-service/eventconsumer/agent_consumer.go 和截图 35ec16e3cc97f5f5dd482af5d901220d.png 只是证据来源。",
		"状态 pending、completed、failed 只是枚举值。",
	}, "\n"))
	names := map[string]bool{}
	for _, entity := range filtered.Entities {
		names[entity.Name] = true
	}
	for _, want := range []string{"agent-manager-service", "agent_dispatch_records", "claran.im.events"} {
		if !names[want] {
			t.Fatalf("entities=%#v, want useful entity %s kept", filtered.Entities, want)
		}
	}
	for _, noise := range []string{"event_id", "client_msg_id", "status", "created_at", "agent_consumer.go", "35ec16e3cc97f5f5dd482af5d901220d.png", "pending", "completed", "failed"} {
		if names[noise] {
			t.Fatalf("entities=%#v, want metadata/status/file noise %s dropped", filtered.Entities, noise)
		}
	}
}

func TestRuleGraphExtractorDoesNotCollectOrdinaryExamples(t *testing.T) {
	result, err := (ruleGraphExtractor{}).Extract(context.Background(), graphExtractInput{
		Chunk: model.Chunk{Content: strings.Join([]string{
			"Teacher、Student、Customer 和 Linux 只是课堂示例，不应该成为知识图谱实体。",
			"这一段没有服务、数据表、事件主题或 API 之间的明确关系。",
			"agent-manager-service 写入 agent_dispatch_records 才是强结构关系。",
		}, "\n")},
	})
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	filtered := filterGraphExtractResult(result, strings.Join([]string{
		"Teacher、Student、Customer 和 Linux 只是课堂示例，不应该成为知识图谱实体。",
		"这一段没有服务、数据表、事件主题或 API 之间的明确关系。",
		"agent-manager-service 写入 agent_dispatch_records 才是强结构关系。",
	}, "\n"))
	names := map[string]bool{}
	for _, entity := range filtered.Entities {
		names[entity.Name] = true
	}
	for _, noise := range []string{"Teacher", "Student", "Customer", "Linux"} {
		if names[noise] {
			t.Fatalf("entities=%#v, want ordinary example %s dropped", filtered.Entities, noise)
		}
	}
	if !names["agent-manager-service"] || !names["agent_dispatch_records"] {
		t.Fatalf("entities=%#v, want strong technical entities kept", filtered.Entities)
	}
}

func TestRelationTriggersRequireEnglishWordBoundaries(t *testing.T) {
	triggers := relationTriggersInSentence("agent-runtime-service keeps readiness metadata for thread_local_state")
	if len(triggers) != 0 {
		t.Fatalf("triggers=%#v, want no READ trigger inside readiness/thread", triggers)
	}
	triggers = relationTriggersInSentence("agent-manager-service calls agent-runtime-service and writes agent_dispatch_records")
	got := map[string]bool{}
	for _, trigger := range triggers {
		got[trigger.Type] = true
	}
	if !got["CALLS"] || !got["WRITES"] {
		t.Fatalf("triggers=%#v, want CALLS and WRITES", triggers)
	}
}

func TestGraphRelationPolicyRejectsImpossibleTypedEdges(t *testing.T) {
	tests := []struct {
		name       string
		relation   string
		sourceType string
		targetType string
		want       bool
	}{
		{name: "service writes table", relation: "WRITES", sourceType: "Service", targetType: "DatabaseTable", want: true},
		{name: "service consumes topic", relation: "CONSUMES", sourceType: "Service", targetType: "EventTopic", want: true},
		{name: "service calls service", relation: "CALLS", sourceType: "Service", targetType: "Service", want: true},
		{name: "table cannot call service", relation: "CALLS", sourceType: "DatabaseTable", targetType: "Service", want: false},
		{name: "topic cannot write table", relation: "WRITES", sourceType: "EventTopic", targetType: "DatabaseTable", want: false},
		{name: "table cannot own topic", relation: "OWNS", sourceType: "DatabaseTable", targetType: "EventTopic", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relationAllowedByEntityTypes(tt.relation, tt.sourceType, tt.targetType)
			if got != tt.want {
				t.Fatalf("relationAllowedByEntityTypes(%s,%s,%s)=%v, want %v", tt.relation, tt.sourceType, tt.targetType, got, tt.want)
			}
		})
	}
}

func TestFilterGraphExtractResultDropsImpossibleTypedRelations(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "agent-manager-service", Type: "Service", Description: "Agent 管理服务"},
			{Name: "agent_dispatch_records", Type: "DatabaseTable", Description: "Agent 调度表"},
			{Name: "agent-runtime-service", Type: "Service", Description: "Agent 运行服务"},
			{Name: "claran.im.events", Type: "EventTopic", Description: "IM 事件 Topic"},
		},
		Relationships: []extractedRelationship{
			{Source: "agent-manager-service", Target: "agent_dispatch_records", Type: "WRITES", Description: "服务写入调度表", Evidence: "agent-manager-service 写入 agent_dispatch_records", Confidence: 0.9},
			{Source: "agent-manager-service", Target: "agent-runtime-service", Type: "CALLS", Description: "服务调用运行时", Evidence: "agent-manager-service 调用 agent-runtime-service", Confidence: 0.9},
			{Source: "agent-manager-service", Target: "claran.im.events", Type: "CONSUMES", Description: "服务消费事件", Evidence: "agent-manager-service 消费 claran.im.events", Confidence: 0.9},
			{Source: "agent_dispatch_records", Target: "agent-runtime-service", Type: "CALLS", Description: "错误：表调用服务", Evidence: "agent_dispatch_records 调用 agent-runtime-service", Confidence: 0.9},
			{Source: "claran.im.events", Target: "agent_dispatch_records", Type: "WRITES", Description: "错误：Topic 写表", Evidence: "claran.im.events 写入 agent_dispatch_records", Confidence: 0.9},
		},
	}, "agent-manager-service 写入 agent_dispatch_records，调用 agent-runtime-service，并消费 claran.im.events。")

	relations := map[string]bool{}
	for _, relation := range result.Relationships {
		relations[relation.Source+" "+relation.Type+" "+relation.Target] = true
	}
	for _, want := range []string{
		"agent-manager-service WRITES agent_dispatch_records",
		"agent-manager-service CALLS agent-runtime-service",
		"agent-manager-service CONSUMES claran.im.events",
	} {
		if !relations[want] {
			t.Fatalf("relationships=%#v, want %s kept", result.Relationships, want)
		}
	}
	for _, impossible := range []string{
		"agent_dispatch_records CALLS agent-runtime-service",
		"claran.im.events WRITES agent_dispatch_records",
	} {
		if relations[impossible] {
			t.Fatalf("relationships=%#v, want impossible relation %s dropped", result.Relationships, impossible)
		}
	}
}

func TestGraphRAGUsesInjectedLLMExtractorAndCommunitySummarizer(t *testing.T) {
	repo := newFakeRAGRepo()
	extractor := &fakeGraphExtractor{result: graphExtractResult{
		Entities: []extractedEntity{
			{Name: "msg-core-service", Type: "Service", Description: "消息事实源", Aliases: []string{"消息核心服务"}},
			{Name: "event_outbox", Type: "DatabaseTable", Description: "事务Outbox表"},
			{Name: "claran.message.events", Type: "EventTopic", Description: "消息事件Topic"},
		},
		Relationships: []extractedRelationship{
			{Source: "msg-core-service", Target: "event_outbox", Type: "WRITES", Description: "写入Outbox", Evidence: "msg-core-service 写入 event_outbox", Confidence: 0.93},
			{Source: "msg-core-service", Target: "claran.message.events", Type: "PUBLISHES", Description: "发布消息事件", Evidence: "发布到 claran.message.events", Confidence: 0.88},
		},
	}}
	summarizer := &fakeGraphCommunitySummarizer{
		title:   "IM 消息链路",
		summary: "该社区描述 msg-core-service 写入 event_outbox 并发布 claran.message.events 的消息事件链路。",
	}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, summarizer)

	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "消息链路",
		Content:    "这段文本由 fake extractor 接管，内容本身不参与规则抽取。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if !extractor.called {
		t.Fatalf("LLM graph extractor was not called")
	}
	if !summarizer.called {
		t.Fatalf("community summarizer was not called")
	}
	if result.EntityCount != 4 || result.RelationCount != 5 {
		t.Fatalf("ingest graph counts = entities %d relations %d, want extracted graph plus document contains links", result.EntityCount, result.RelationCount)
	}
	if len(repo.communities) != 1 {
		t.Fatalf("communities len = %d, want one LLM summarized community", len(repo.communities))
	}
	if repo.communities[0].Name != "IM 消息链路" || !strings.Contains(repo.communities[0].Summary, "event_outbox") {
		t.Fatalf("community = %#v, want injected LLM summary", repo.communities[0])
	}
	for _, entity := range repo.entities {
		if entity.CommunityID == 0 {
			t.Fatalf("entity %s has no community id after rebuild", entity.Name)
		}
	}
}

func TestGraphRAGKeepsDocumentTitleNodeAndContainsRelations(t *testing.T) {
	repo := newFakeRAGRepo()
	extractor := &fakeGraphExtractor{result: graphExtractResult{
		Entities: []extractedEntity{
			{Name: "msg-core-service", Type: "Service", Description: "消息事实源"},
			{Name: "event_outbox", Type: "DatabaseTable", Description: "事务Outbox表"},
		},
		Relationships: []extractedRelationship{
			{Source: "msg-core-service", Target: "event_outbox", Type: "WRITES", Description: "消息服务写入Outbox", Evidence: "msg-core-service 写入 event_outbox", Confidence: 0.93},
		},
	}}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, nil)

	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "消息链路设计",
		Content:    "msg-core-service 写入 event_outbox。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if result.EntityCount < 3 || result.RelationCount < 3 {
		t.Fatalf("graph counts = entities %d relations %d, want document node plus contains relations", result.EntityCount, result.RelationCount)
	}
	var documentNode *model.Entity
	for i := range repo.entities {
		if repo.entities[i].Type == "文档" && repo.entities[i].Name == "消息链路设计" && strings.HasPrefix(repo.entities[i].CanonicalKey, "document:") {
			documentNode = &repo.entities[i]
			break
		}
	}
	if documentNode == nil {
		t.Fatalf("entities=%#v, want document title node with document canonical key", repo.entities)
	}
	containsTargets := map[int64]bool{}
	for _, relation := range repo.relations {
		if relation.SourceID == documentNode.ID && relation.Relation == "CONTAINS" && relation.DocumentID == result.Document.Id {
			containsTargets[relation.TargetID] = true
		}
	}
	if len(containsTargets) < 2 {
		t.Fatalf("relations=%#v, want document CONTAINS relation to extracted entities", repo.relations)
	}
}

func TestGraphEntityTypeNormalizationInfersInvalidGenericType(t *testing.T) {
	result := filterGraphExtractResult(graphExtractResult{
		Entities: []extractedEntity{
			{Name: "msg-core-service", Type: "实体", Description: "ClaranAIM 的消息核心服务，负责写入消息事实"},
			{Name: "event_outbox", Type: "对象", Description: "消息事务中写入的 Outbox 数据表"},
		},
		Relationships: []extractedRelationship{
			{Source: "msg-core-service", Target: "event_outbox", Type: "WRITES", Description: "消息核心服务写入Outbox表", Evidence: "msg-core-service 写入 event_outbox", Confidence: 0.93},
		},
	}, "msg-core-service 是消息核心服务，负责写入 event_outbox 数据表。")

	typeByName := map[string]string{}
	for _, entity := range result.Entities {
		typeByName[entity.Name] = entity.Type
	}
	if typeByName["msg-core-service"] != "服务" || typeByName["event_outbox"] != "数据库表" {
		t.Fatalf("types=%#v, want generic invalid types inferred as free Chinese labels 服务 and 数据库表", typeByName)
	}
}

func TestConversationIngestSkipsKnowledgeGraphBuild(t *testing.T) {
	repo := newFakeRAGRepo()
	extractor := &fakeGraphExtractor{result: graphExtractResult{
		Entities: []extractedEntity{{Name: "会话主题", Type: "Concept", Description: "不应进入知识图谱"}},
		Relationships: []extractedRelationship{
			{Source: "会话主题", Target: "会话摘要", Type: "RELATED_TO", Description: "会话归档关系", Evidence: "会话摘要", Confidence: 0.9},
		},
	}}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, nil)

	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "会话摘要",
		SourceType: "conversation",
		Content:    "这一小时大家主要讨论了 RAG 和 Memory。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if extractor.called || result.EntityCount != 0 || result.RelationCount != 0 {
		t.Fatalf("conversation graph build = called %v entities %d relations %d, want skipped", extractor.called, result.EntityCount, result.RelationCount)
	}
}

func TestGraphRAGExtractsFromParentChunksFirst(t *testing.T) {
	repo := newFakeRAGRepo()
	extractor := &fakeGraphExtractor{result: graphExtractResult{
		Entities: []extractedEntity{
			{Name: "GraphRAG", Type: "Concept", Description: "知识图谱增强检索"},
			{Name: "Milvus", Type: "Product", Description: "向量数据库"},
		},
		Relationships: []extractedRelationship{
			{Source: "GraphRAG", Target: "Milvus", Type: "DEPENDS_ON", Description: "结合向量库召回", Evidence: "GraphRAG 结合 Milvus 做召回", Confidence: 0.9},
		},
	}}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, nil)

	_, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "长文档",
		SourceType: "markdown",
		Content: strings.Join([]string{
			"# RAG 文档",
			"## GraphRAG 章节",
			"GraphRAG 结合 Milvus 做召回，并用社区摘要解释实体关系。",
			"这段补充更多背景，让父块足够长，便于模型抽取章节级实体和关系。",
		}, "\n"),
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if len(extractor.inputs) == 0 {
		t.Fatalf("graph extractor was not called")
	}
	if normalizedChunkLevel(extractor.inputs[0].Chunk) != model.ChunkLevelParent {
		t.Fatalf("first graph extraction chunk level = %q, want parent", extractor.inputs[0].Chunk.ChunkLevel)
	}
}

func TestConfiguredLLMGraphExtractorDoesNotFallbackToRuleNoise(t *testing.T) {
	repo := newFakeRAGRepo()
	extractor := &fakeGraphExtractor{result: graphExtractResult{}}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, nil)

	result, err := svc.IngestDocument(context.Background(), IngestInput{
		OwnerID:    1001,
		Title:      "规则可抽取但LLM为空",
		Content:    "agent-manager-service 消费 claran.im.events，并调用 agent-runtime-service。agent-manager-service 写入 agent_dispatch_records。",
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("IngestDocument returned error: %v", err)
	}
	if result.EntityCount != 0 || result.RelationCount != 0 {
		t.Fatalf("graph counts = entities %d relations %d, want no rule fallback when LLM extractor is configured", result.EntityCount, result.RelationCount)
	}
}

func TestLeidenLikeCommunitiesSeparatesDenseClusters(t *testing.T) {
	entities := []model.Entity{
		{ID: 1, Name: "msg-core-service", Type: "Service", Score: 1},
		{ID: 2, Name: "event_outbox", Type: "DatabaseTable", Score: 1},
		{ID: 3, Name: "claran.message.events", Type: "EventTopic", Score: 1},
		{ID: 4, Name: "agent-manager-service", Type: "Service", Score: 1},
		{ID: 5, Name: "agent-runtime-service", Type: "Service", Score: 1},
		{ID: 6, Name: "agent_dispatch_records", Type: "DatabaseTable", Score: 1},
	}
	relations := []model.Relation{
		{SourceID: 1, TargetID: 2, Weight: 1},
		{SourceID: 1, TargetID: 3, Weight: 1},
		{SourceID: 2, TargetID: 3, Weight: 1},
		{SourceID: 4, TargetID: 5, Weight: 1},
		{SourceID: 4, TargetID: 6, Weight: 1},
		{SourceID: 5, TargetID: 6, Weight: 1},
		{SourceID: 3, TargetID: 4, Weight: 0.01},
	}

	assignments := leidenLikeCommunities(entities, relations)
	if assignments[1] != assignments[2] || assignments[1] != assignments[3] {
		t.Fatalf("message cluster assignments = %#v, want nodes 1/2/3 together", assignments)
	}
	if assignments[4] != assignments[5] || assignments[4] != assignments[6] {
		t.Fatalf("agent cluster assignments = %#v, want nodes 4/5/6 together", assignments)
	}
	if assignments[1] == assignments[4] {
		t.Fatalf("assignments = %#v, want weakly connected dense clusters separated", assignments)
	}

	svc := NewRAGService(newFakeRAGRepo(), NewLocalVectorIndex(), 64, "hybrid").(*ragServiceImpl)
	communities := svc.buildCommunityModels(context.Background(), 1001, entities, relations, assignments)
	if len(communities) != 2 {
		t.Fatalf("communities len = %d, want 2", len(communities))
	}
	if !strings.Contains(communities[0].KeyEntitiesJSON, "msg-core-service") && !strings.Contains(communities[1].KeyEntitiesJSON, "msg-core-service") {
		t.Fatalf("communities = %#v, want key entities encoded", communities)
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

func TestGetGraphRebuildsMissingDocumentGraphFromStoredChunks(t *testing.T) {
	repo := newFakeRAGRepo()
	doc := &model.Document{OwnerID: 1001, Title: "已有长文档", SourceType: "markdown", Visibility: model.VisibilityPrivate}
	chunks := []model.Chunk{
		{ChunkLevel: model.ChunkLevelParent, Content: "GraphRAG 结合 Milvus 做召回，并用知识图谱解释实体关系。", Summary: "GraphRAG 与 Milvus", QualityScore: 0.9},
		{ChunkLevel: model.ChunkLevelChild, Content: "GraphRAG 结合 Milvus 做召回。", Summary: "GraphRAG 与 Milvus", QualityScore: 0.8},
	}
	if err := repo.CreateDocumentWithChunks(context.Background(), doc, chunks); err != nil {
		t.Fatalf("CreateDocumentWithChunks returned error: %v", err)
	}
	extractor := &fakeGraphExtractor{result: graphExtractResult{
		Entities: []extractedEntity{
			{Name: "GraphRAG", Type: "Concept", Description: "知识图谱增强检索"},
			{Name: "Milvus", Type: "Product", Description: "向量数据库"},
		},
		Relationships: []extractedRelationship{
			{Source: "GraphRAG", Target: "Milvus", Type: "DEPENDS_ON", Description: "结合向量库召回", Evidence: "GraphRAG 结合 Milvus 做召回", Confidence: 0.9},
		},
	}}
	svc := NewRAGServiceWithGraphExtractor(repo, NewLocalVectorIndex(), 64, "hybrid", nil, nil, nil, nil, nil, extractor, nil)

	graph, err := svc.GetGraph(context.Background(), GraphInput{ViewerID: 1001, DocumentID: doc.ID, Limit: 20})
	if err != nil {
		t.Fatalf("GetGraph returned error: %v", err)
	}
	if !extractor.called {
		t.Fatalf("missing document graph should trigger one rebuild from stored chunks")
	}
	if len(graph.Nodes) != 3 || len(graph.Edges) != 3 {
		t.Fatalf("graph nodes=%d edges=%d, want rebuilt scoped graph with document title node", len(graph.Nodes), len(graph.Edges))
	}
}

func TestGetGraphWarnsLegacyDocumentGraphNeedsRebuild(t *testing.T) {
	repo := newFakeRAGRepo()
	doc := &model.Document{OwnerID: 1001, Title: "旧版图谱文档", SourceType: "markdown", Visibility: model.VisibilityPrivate}
	if err := repo.CreateDocumentWithChunks(context.Background(), doc, []model.Chunk{
		{ChunkLevel: model.ChunkLevelChild, Content: "旧版图谱只有相关关系。", Summary: "旧版图谱", QualityScore: 0.8},
	}); err != nil {
		t.Fatalf("CreateDocumentWithChunks returned error: %v", err)
	}
	a := &model.Entity{OwnerID: 1001, Name: "旧实体A", CanonicalKey: "old-a", Type: "Concept", Summary: "旧版实体A", Score: 1}
	b := &model.Entity{OwnerID: 1001, Name: "旧实体B", CanonicalKey: "old-b", Type: "Concept", Summary: "旧版实体B", Score: 1}
	if err := repo.SaveEntity(context.Background(), a); err != nil {
		t.Fatalf("SaveEntity A returned error: %v", err)
	}
	if err := repo.SaveEntity(context.Background(), b); err != nil {
		t.Fatalf("SaveEntity B returned error: %v", err)
	}
	if err := repo.SaveRelation(context.Background(), &model.Relation{
		OwnerID:    1001,
		SourceID:   a.ID,
		TargetID:   b.ID,
		Relation:   "RELATED_TO",
		Weight:     0.95,
		Confidence: 0.95,
		Evidence:   "旧版图谱只有相关关系",
		DocumentID: doc.ID,
	}); err != nil {
		t.Fatalf("SaveRelation returned error: %v", err)
	}
	svc := NewRAGService(repo, NewLocalVectorIndex(), 64, "hybrid")

	graph, err := svc.GetGraph(context.Background(), GraphInput{ViewerID: 1001, DocumentID: doc.ID, Limit: 20})
	if err != nil {
		t.Fatalf("GetGraph returned error: %v", err)
	}
	if !strings.Contains(graph.Msg, "旧版图谱") || !strings.Contains(graph.Msg, "重建当前文档图谱") {
		t.Fatalf("graph msg=%q, want legacy rebuild diagnostic", graph.Msg)
	}
}

func TestFilterGraphForDisplayDropsNoiseAndImpossibleRelations(t *testing.T) {
	entities := []model.Entity{
		{ID: 1, Name: "agent-manager-service", Type: "Service", Summary: "Agent 管理服务", Score: 2},
		{ID: 2, Name: "agent_dispatch_records", Type: "DatabaseTable", Summary: "Agent 调度幂等表", Score: 1.5},
		{ID: 3, Name: "agent-runtime-service", Type: "Service", Summary: "Agent 运行服务", Score: 1.4},
		{ID: 4, Name: "0", Type: "Concept", Summary: "无意义实体", Score: 1},
		{ID: 5, Name: "event_id", Type: "Concept", Summary: "字段名", Score: 1},
	}
	relations := []model.Relation{
		{ID: 101, SourceID: 1, TargetID: 2, Relation: "WRITES", Weight: 0.92, Evidence: "agent-manager-service 写入 agent_dispatch_records"},
		{ID: 102, SourceID: 1, TargetID: 3, Relation: "CALLS", Weight: 0.9, Evidence: "agent-manager-service 调用 agent-runtime-service"},
		{ID: 103, SourceID: 2, TargetID: 3, Relation: "CALLS", Weight: 0.9, Evidence: "错误：表调用服务"},
		{ID: 104, SourceID: 1, TargetID: 4, Relation: "RELATED_TO", Weight: 0.9, Evidence: "错误：连接数字实体"},
		{ID: 105, SourceID: 5, TargetID: 3, Relation: "RELATED_TO", Weight: 0.9, Evidence: "错误：连接字段名"},
	}

	filteredEntities, filteredRelations := filterGraphForDisplay(entities, relations)
	names := map[string]bool{}
	for _, entity := range filteredEntities {
		names[entity.Name] = true
	}
	if names["0"] || names["event_id"] {
		t.Fatalf("entities=%#v, want numeric and field-name nodes dropped", filteredEntities)
	}
	edges := map[string]bool{}
	for _, relation := range filteredRelations {
		edges[fmt.Sprintf("%d-%s-%d", relation.SourceID, relation.Relation, relation.TargetID)] = true
	}
	if !edges["1-WRITES-2"] || !edges["1-CALLS-3"] {
		t.Fatalf("relations=%#v, want meaningful service relations kept", filteredRelations)
	}
	if edges["2-CALLS-3"] || edges["1-RELATED_TO-4"] || edges["5-RELATED_TO-3"] {
		t.Fatalf("relations=%#v, want impossible/noisy relations dropped", filteredRelations)
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

type fakeGraphExtractor struct {
	result graphExtractResult
	err    error
	called bool
	inputs []graphExtractInput
}

type fakeGraphCommunitySummarizer struct {
	title   string
	summary string
	err     error
	called  bool
}

func (e *fakeGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	_ = ctx
	e.called = true
	e.inputs = append(e.inputs, input)
	return e.result, e.err
}

func (s *fakeGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	_ = ctx
	_ = input
	s.called = true
	return graphCommunitySummary{Title: s.title, Summary: s.summary}, s.err
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

func (r *fakeRAGRepo) CountChildChunksByDocumentIDs(ctx context.Context, documentIDs []int64) (map[int64]int64, error) {
	_ = ctx
	allowed := map[int64]bool{}
	for _, id := range documentIDs {
		allowed[id] = true
	}
	out := map[int64]int64{}
	for _, chunk := range r.chunks {
		if !allowed[chunk.DocumentID] || normalizedChunkLevel(chunk) != model.ChunkLevelChild {
			continue
		}
		out[chunk.DocumentID]++
	}
	return out, nil
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
		if filter.DocumentID > 0 && doc.ID != filter.DocumentID {
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

func (r *fakeRAGRepo) DeleteDocument(ctx context.Context, viewerID, documentID int64) error {
	_ = ctx
	r.DeleteDocumentGraph(ctx, viewerID, documentID)
	docs := make([]model.Document, 0, len(r.docs))
	for _, doc := range r.docs {
		if doc.ID == documentID && doc.OwnerID == viewerID {
			continue
		}
		docs = append(docs, doc)
	}
	r.docs = docs
	chunks := make([]model.Chunk, 0, len(r.chunks))
	for _, chunk := range r.chunks {
		if chunk.DocumentID == documentID && chunk.OwnerID == viewerID {
			continue
		}
		chunks = append(chunks, chunk)
	}
	r.chunks = chunks
	return nil
}

func (r *fakeRAGRepo) DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error {
	_ = ctx
	relations := make([]model.Relation, 0, len(r.relations))
	used := map[int64]bool{}
	for _, relation := range r.relations {
		if relation.DocumentID == documentID && relation.OwnerID == viewerID {
			continue
		}
		relations = append(relations, relation)
		used[relation.SourceID] = true
		used[relation.TargetID] = true
	}
	r.relations = relations
	entities := make([]model.Entity, 0, len(r.entities))
	for _, entity := range r.entities {
		if entity.OwnerID == viewerID && !used[entity.ID] {
			continue
		}
		entities = append(entities, entity)
	}
	r.entities = entities
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

func (r *fakeRAGRepo) ListOwnerGraph(ctx context.Context, ownerID int64) ([]model.Entity, []model.Relation, error) {
	_ = ctx
	entities := make([]model.Entity, 0, len(r.entities))
	for _, entity := range r.entities {
		if entity.OwnerID == ownerID {
			entities = append(entities, entity)
		}
	}
	relations := make([]model.Relation, 0, len(r.relations))
	for _, relation := range r.relations {
		if relation.OwnerID == ownerID {
			relations = append(relations, relation)
		}
	}
	return entities, relations, nil
}

func (r *fakeRAGRepo) ReplaceOwnerCommunities(ctx context.Context, ownerID int64, communities []model.Community, entityCommunity map[int64]int64) error {
	_ = ctx
	kept := make([]model.Community, 0, len(r.communities))
	for _, community := range r.communities {
		if community.OwnerID != ownerID {
			kept = append(kept, community)
		}
	}
	for i := range communities {
		if communities[i].ID == 0 {
			communities[i].ID = r.allocID()
		}
		communities[i].OwnerID = ownerID
		kept = append(kept, communities[i])
	}
	r.communities = kept
	for i := range r.entities {
		if r.entities[i].OwnerID != ownerID {
			continue
		}
		if communityID, ok := entityCommunity[r.entities[i].ID]; ok {
			r.entities[i].CommunityID = communityID
		}
	}
	return nil
}

func (r *fakeRAGRepo) ListGraph(ctx context.Context, viewerID int64, query string, limit int, documentID int64, hops int) ([]model.Entity, []model.Relation, []model.Community, error) {
	_ = ctx
	if limit <= 0 {
		limit = 80
	}
	_ = hops
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
		if documentID > 0 && relation.DocumentID != documentID {
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

func (s *fakeRouterSettings) TestLLMProfile(ctx context.Context, ownerID int64, input settingsclient.TestLLMProfileInput) (settingsclient.TestLLMProfileResult, error) {
	return settingsclient.TestLLMProfileResult{OK: true, Msg: "ok"}, nil
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

func (s *fakeRouterSettings) SaveMCPServer(ctx context.Context, ownerID int64, input settingsclient.SaveMCPServerInput) (*settingsclient.MCPServerConfig, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ListMCPServers(ctx context.Context, ownerID int64, scope string, agentID, conversationID int64, includeDisabled bool) ([]settingsclient.MCPServerConfig, error) {
	return nil, nil
}

func (s *fakeRouterSettings) ResolveMCPServers(ctx context.Context, ownerID, agentID, conversationID int64) ([]settingsclient.MCPServerConfig, error) {
	return nil, nil
}

func (s *fakeRouterSettings) DeleteMCPServer(ctx context.Context, ownerID, serverID int64) error {
	return nil
}
