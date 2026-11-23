// Package service 实现 Milvus-ready RAG、Hybrid Search 和 GraphRAG。
package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/graphstore"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/settings/settingsservice"
	"ClaranAIM/pkg/observability"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RAGService 是 rag-service 的业务接口。
type RAGService interface {
	IngestDocument(ctx context.Context, input IngestInput) (IngestResult, error)
	Search(ctx context.Context, input SearchInput) (SearchResult, error)
	GetGraph(ctx context.Context, input GraphInput) (GraphResult, error)
	ListDocuments(ctx context.Context, viewerID int64, limit, offset int) ([]rag.RAGDocument, int64, error)
	DeleteDocument(ctx context.Context, viewerID, documentID int64) error
	DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error
}

type IngestInput struct {
	OwnerID           int64
	Title             string
	Content           string
	Source            string
	SourceType        string
	Visibility        string
	GroupID           int64
	ConversationID    int64
	GraphRelationMode string
}

type IngestResult struct {
	Document      rag.RAGDocument
	ChunkCount    int64
	EntityCount   int64
	RelationCount int64
}

type SearchInput struct {
	ViewerID       int64
	Query          string
	Mode           string
	Limit          int
	GroupID        int64
	ConversationID int64
	DocumentID     int64
}

type SearchResult = rag.SearchResp

type GraphInput struct {
	ViewerID   int64
	Query      string
	Limit      int
	DocumentID int64
	Hops       int
}

type GraphResult = rag.GraphResp

// VectorIndex 是 Milvus 等向量后端的最小接口。
// 当前默认使用 LocalVectorIndex；生产接入 Milvus SDK 时只需要替换该接口实现。
type VectorIndex interface {
	Upsert(ctx context.Context, chunkID int64, vector []float32, metadata map[string]string) error
	Search(ctx context.Context, queryVector []float32, limit int) ([]VectorHit, error)
}

type VectorHit struct {
	ChunkID int64
	Score   float64
}

type ragServiceImpl struct {
	repo            dao.Repository
	graphStore      graphstore.GraphStore
	vectorIndex     VectorIndex
	embedder        EmbeddingProvider
	router          RAGRouter
	reranker        Reranker
	cragEvaluator   CRAGEvaluator
	selfJudge       SelfRAGJudge
	graphExtractor  graphExtractor
	graphSummarizer graphCommunitySummarizer
	settings        settingsservice.Client
	routerFactory   func(resolvedLLMConfig) RAGRouter
	embeddingDim    int
	defaultMode     string
	llmGraphEnabled bool
}

type resolvedLLMConfig struct {
	ProfileID      int64
	APIKey         string
	BaseURL        string
	ModelName      string
	ProviderType   string
	PromptTemplate string
}

func (s *ragServiceImpl) IngestDocument(ctx context.Context, input IngestInput) (IngestResult, error) {
	start := time.Now()
	status := "success"
	defer func() {
		observability.RecordBusinessDuration("rag", "ingest_document", status, time.Since(start))
		observability.RecordBusinessEvent("rag", "ingest_document", status)
	}()
	if s.repo == nil {
		status = "error"
		return IngestResult{}, errors.New("rag repository未配置")
	}
	if input.OwnerID <= 0 {
		status = "error"
		return IngestResult{}, errors.New("owner_id不能为空")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		status = "error"
		return IngestResult{}, errors.New("文档内容不能为空")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = firstLine(content, "未命名知识")
	}
	visibility := strings.TrimSpace(input.Visibility)
	if visibility == "" {
		visibility = model.VisibilityPrivate
	}
	doc := &model.Document{
		OwnerID:        input.OwnerID,
		Title:          title,
		Source:         strings.TrimSpace(input.Source),
		SourceType:     defaultString(input.SourceType, "text"),
		Visibility:     visibility,
		GroupID:        input.GroupID,
		ConversationID: input.ConversationID,
		Status:         model.DocumentStatusReady,
	}
	chunks, err := buildHierarchicalChunks(input, content)
	if err != nil {
		status = "error"
		return IngestResult{}, err
	}
	if err := s.repo.CreateDocumentWithChunks(ctx, doc, chunks); err != nil {
		status = "error"
		return IngestResult{}, err
	}
	for _, chunk := range chunks {
		if chunk.ID <= 0 || normalizedChunkLevel(chunk) != model.ChunkLevelChild {
			continue
		}
		_ = s.vectorIndex.Upsert(ctx, chunk.ID, s.embedding(ctx, chunk.Content), map[string]string{"document_id": fmt.Sprint(doc.ID)})
	}
	entityCount, relationCount := int64(0), int64(0)
	if shouldBuildGraphForSourceType(doc.SourceType) {
		entityCount, relationCount = s.buildGraph(ctx, input.OwnerID, doc.ID, doc.Title, doc.SourceType, chunks, normalizeGraphRelationMode(input.GraphRelationMode))
	}
	dto := documentToRPC(doc)
	return IngestResult{Document: dto, ChunkCount: int64(countChildChunks(chunks)), EntityCount: entityCount, RelationCount: relationCount}, nil
}

// Search 执行 Adaptive RAG：按问题复杂度选择 Hybrid、GraphRAG、Web/Memory 外部路线或简单直答，并附带 CRAG 和 Self-RAG 检查点。
func (s *ragServiceImpl) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	start := time.Now()
	status := "success"
	defer func() {
		observability.RecordBusinessDuration("rag", "search", status, time.Since(start))
		observability.RecordBusinessEvent("rag", "search", status)
	}()
	if s.repo == nil {
		status = "error"
		return SearchResult{}, errors.New("rag repository未配置")
	}
	if input.ViewerID <= 0 {
		status = "error"
		return SearchResult{}, errors.New("viewer_id不能为空")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		status = "error"
		return SearchResult{}, errors.New("query不能为空")
	}
	limit := input.Limit
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	mode := strings.TrimSpace(input.Mode)
	decision := RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: sanitizeRAGMode(mode, s.defaultMode), Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs"}, Strategy: "hybrid_rerank", Query: query, Reason: "用户指定检索路线"}
	if mode == "document" {
		reason := "用户指定文档检索，强制查询知识库"
		if input.DocumentID > 0 {
			reason = "用户指定文档检索，已按文档ID限定知识范围"
		}
		decision = RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: "document", Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs"}, Strategy: "document_search", Query: query, Reason: reason}
	} else if mode == "" || mode == "adaptive" {
		decision = s.route(ctx, input)
		mode = decision.Mode
	} else {
		mode = decision.Mode
	}
	self := &rag.SelfRAGCheckpoints{Retrieve: decision.Retrieve, Note: routerNote(decision)}
	if !decision.Retrieve || decision.Route == AdaptiveRouteDirect || mode == "direct" {
		self.IsRel = true
		self.IsSup = true
		self.IsUse = true
		answer := s.synthesizeDirectAnswer(ctx, input.ViewerID, query, decision)
		return SearchResult{
			Success:    true,
			Answer:     answer,
			Route:      "direct",
			CragAction: "skip_vector",
			SelfCheck:  self,
		}, nil
	}
	if decision.Route == AdaptiveRouteWebRAG || mode == "web" {
		return s.externalRouteHint(query, decision, self, "web_rag", "这个问题需要外部实时资料。当前 RAG 服务已路由到 Web RAG，但 Web Search 工具由 agent-runtime 执行；请让 Agent 调用 web_search 或接入 web-search-service 后再合并内部资料。"), nil
	}
	if decision.Route == AdaptiveRouteMemoryRAG || mode == "memory" {
		return s.externalRouteHint(query, decision, self, "memory_rag", "这个问题需要私有长期记忆。memory-service 已提供 Memory RAG 召回链路，包括 embedding、Milvus/本地向量候选、元数据过滤、MySQL 回源校验、融合打分和可选小模型过滤；请通过 memory-service Recall 在当前 Agent、用户和会话权限边界内召回记忆后再回答。"), nil
	}
	if decision.Route == AdaptiveRouteToolAction || mode == "tool_action" {
		return s.externalRouteHint(query, decision, self, "tool_action", "这是动作请求，不应由 RAG 检索直接执行。请交给 Agent 工具审批/执行链路，并保留授权和审计。"), nil
	}
	retrievalQuery := defaultString(decision.Query, query)
	retrievalSource := defaultString(decision.RetrievalSource, "project_docs")
	searchInput := input
	searchInput.Query = retrievalQuery
	rows, err := s.hybridRetrieve(ctx, searchInput, 30)
	if err != nil {
		return SearchResult{}, err
	}
	reranked := s.rerank(ctx, retrievalQuery, rows, limit)
	cragEval := s.evaluateCRAG(ctx, query, reranked)
	cragAction := cragActionForLabel(cragEval.Label)
	graph, _ := s.GetGraph(ctx, GraphInput{ViewerID: input.ViewerID, Query: query, Limit: 30, DocumentID: input.DocumentID})
	answer := s.synthesizeAnswer(ctx, input.ViewerID, query, mode, cragAction, reranked, graph.Nodes)
	judgement := s.judgeSelfRAG(ctx, query, answer, reranked, cragEval, mode)
	self.IsRel = judgement.IsRel
	self.IsSup = judgement.IsSup
	self.IsUse = judgement.IsUse
	self.Note = self.Note + fmt.Sprintf("；Self-RAG Retrieve: retrieval_source=%s retrieval_query=%s；", retrievalSource, retrievalQuery) + cragNote(cragEval) + "；" + selfRAGNote(judgement)
	return SearchResult{
		Success:    true,
		Answer:     answer,
		Sources:    chunksToSources(reranked),
		GraphNodes: graph.Nodes,
		GraphEdges: graph.Edges,
		Route:      mode,
		CragAction: cragAction,
		SelfCheck:  self,
	}, nil
}

// GetGraph 返回 GraphRAG 知识图谱子图。
func (s *ragServiceImpl) GetGraph(ctx context.Context, input GraphInput) (GraphResult, error) {
	if s.repo == nil {
		return GraphResult{}, errors.New("rag repository未配置")
	}
	if s.graphStore == nil {
		return GraphResult{Success: false, Msg: "graph store未配置"}, nil
	}
	if input.DocumentID > 0 {
		if _, err := s.repo.GetVisibleDocument(ctx, input.ViewerID, input.DocumentID); err != nil {
			return GraphResult{Success: false, Msg: err.Error()}, nil
		}
	}
	nodes, edges, communities, err := s.graphStore.ListGraph(ctx, input.ViewerID, input.Query, input.Limit, input.DocumentID, input.Hops)
	if err != nil {
		return GraphResult{}, err
	}
	if input.DocumentID > 0 && len(edges) == 0 {
		s.rebuildDocumentGraphIfMissing(ctx, input.ViewerID, input.DocumentID)
		nodes, edges, communities, err = s.graphStore.ListGraph(ctx, input.ViewerID, input.Query, input.Limit, input.DocumentID, input.Hops)
		if err != nil {
			return GraphResult{}, err
		}
	}
	legacyGraph := input.DocumentID > 0 && looksLikeLegacyDocumentGraph(nodes, edges, input.DocumentID)
	nodes, edges = filterGraphForDisplay(nodes, edges)
	edges = ensureDisplayEntityRelations(nodes, edges, input.DocumentID)
	msg := ""
	if legacyGraph {
		msg = "该文档当前显示的是旧版图谱：缺少文档标题节点或关系类型过于单一。请点击“重建当前文档图谱”使用新的 GraphRAG 抽取逻辑重建。"
	}
	if msg == "" && (len(nodes) == 0 || len(edges) == 0) {
		msg = s.describeEmptyGraph(ctx, input.ViewerID, input.DocumentID, len(nodes), len(edges))
	}
	return GraphResult{Success: true, Nodes: entitiesToRPC(nodes), Edges: relationsToRPC(edges), Communities: communitiesToRPC(communities), Msg: msg}, nil
}
