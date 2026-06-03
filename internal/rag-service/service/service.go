// Package service 实现 Milvus-ready RAG、Hybrid Search 和 GraphRAG。
package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/settingsclient"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
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
	OwnerID        int64
	Title          string
	Content        string
	Source         string
	SourceType     string
	Visibility     string
	GroupID        int64
	ConversationID int64
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
	vectorIndex     VectorIndex
	embedder        EmbeddingProvider
	router          RAGRouter
	reranker        Reranker
	cragEvaluator   CRAGEvaluator
	selfJudge       SelfRAGJudge
	graphExtractor  graphExtractor
	graphSummarizer graphCommunitySummarizer
	settings        settingsclient.Service
	routerFactory   func(settingsclient.ResolvedLLMConfig) RAGRouter
	embeddingDim    int
	defaultMode     string
	llmGraphEnabled bool
}

// NewRAGService 创建 RAG 业务服务。
func NewRAGService(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string) RAGService {
	if embeddingDim <= 0 {
		embeddingDim = 256
	}
	if vectorIndex == nil {
		vectorIndex = NewLocalVectorIndex()
	}
	if strings.TrimSpace(defaultMode) == "" {
		defaultMode = "adaptive"
	}
	return &ragServiceImpl{repo: repo, vectorIndex: vectorIndex, router: HybridAdaptiveRouter{}, graphExtractor: ruleGraphExtractor{}, graphSummarizer: ruleGraphCommunitySummarizer{}, embeddingDim: embeddingDim, defaultMode: defaultMode}
}

// NewRAGServiceWithEmbedding 创建带外部 embedding provider 的 RAG 服务。
// provider 不可用时每次向量生成都会自动降级到 hash embedding，保证入库链路不中断。
func NewRAGServiceWithEmbedding(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider) RAGService {
	svc := NewRAGService(repo, vectorIndex, embeddingDim, defaultMode).(*ragServiceImpl)
	svc.embedder = embedder
	return svc
}

// NewRAGServiceWithRouter 创建带 embedding provider 和 RAG router 的服务。
// router 通常是轻量 LLM；router 失败时 Search 会回退到规则路由，避免外部模型故障阻断检索。
func NewRAGServiceWithRouter(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter) RAGService {
	svc := NewRAGServiceWithEmbedding(repo, vectorIndex, embeddingDim, defaultMode, embedder).(*ragServiceImpl)
	if router != nil {
		svc.router = HybridAdaptiveRouter{LLM: router}
	}
	return svc
}

func NewRAGServiceWithRouterAndReranker(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker) RAGService {
	return NewRAGServiceWithRouterRerankerAndCRAG(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, nil)
}

func NewRAGServiceWithRouterRerankerAndCRAG(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator) RAGService {
	return NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, nil)
}

func NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge) RAGService {
	svc := NewRAGServiceWithRouter(repo, vectorIndex, embeddingDim, defaultMode, embedder, router).(*ragServiceImpl)
	svc.reranker = reranker
	if cragEvaluator == nil {
		cragEvaluator = RuleCRAGEvaluator{}
	}
	svc.cragEvaluator = cragEvaluator
	if selfJudge == nil {
		selfJudge = RuleSelfRAGJudge{}
	}
	svc.selfJudge = selfJudge
	return svc
}

// NewRAGServiceWithGraphExtractor 创建带 GraphRAG LLM 抽取器/社区摘要器的服务。
// 抽取器或摘要器失败时会自动降级到规则实现，避免外部模型故障阻断知识入库。
func NewRAGServiceWithGraphExtractor(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge).(*ragServiceImpl)
	if extractor != nil {
		svc.graphExtractor = fallbackGraphExtractor{primary: extractor}
		svc.llmGraphEnabled = true
	}
	if summarizer != nil {
		svc.graphSummarizer = fallbackGraphCommunitySummarizer{primary: summarizer, fallback: ruleGraphCommunitySummarizer{}}
	}
	return svc
}

// NewRAGServiceWithRouterProviderAndGraphExtractor 同时启用用户级 RAG Router 和 GraphRAG LLM 抽取/社区摘要。
// 这避免 settings-service 覆盖 Router 时把 GraphRAG 又退回纯规则抽取。
func NewRAGServiceWithRouterProviderAndGraphExtractor(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, settings settingsclient.Service, routerFactory func(settingsclient.ResolvedLLMConfig) RAGRouter, extractor graphExtractor, summarizer graphCommunitySummarizer) RAGService {
	svc := NewRAGServiceWithGraphExtractor(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge, extractor, summarizer).(*ragServiceImpl)
	svc.settings = settings
	svc.routerFactory = routerFactory
	return svc
}

// NewRAGServiceWithRouterProvider 创建支持“用户 RAG 路由小模型覆盖”的 RAG 服务。
// Search 时会先读取当前 viewer 的 settings-service rag_router 默认预设；如果没有配置、配置不完整、
// settings-service 暂不可用或用户小模型调用失败，就回退到启动时注入的项目内置 router。
func NewRAGServiceWithRouterProvider(repo dao.Repository, vectorIndex VectorIndex, embeddingDim int, defaultMode string, embedder EmbeddingProvider, router RAGRouter, reranker Reranker, cragEvaluator CRAGEvaluator, selfJudge SelfRAGJudge, settings settingsclient.Service, routerFactory func(settingsclient.ResolvedLLMConfig) RAGRouter) RAGService {
	svc := NewRAGServiceWithRouterRerankerCRAGAndSelfJudge(repo, vectorIndex, embeddingDim, defaultMode, embedder, router, reranker, cragEvaluator, selfJudge).(*ragServiceImpl)
	svc.settings = settings
	if routerFactory != nil {
		svc.routerFactory = routerFactory
	} else {
		svc.routerFactory = func(cfg settingsclient.ResolvedLLMConfig) RAGRouter {
			return NewLLMRouter(cfg.APIKey, cfg.BaseURL, cfg.ModelName)
		}
	}
	return svc
}

// IngestDocument 写入文档、分块、向量索引和 GraphRAG 图谱。
func (s *ragServiceImpl) IngestDocument(ctx context.Context, input IngestInput) (IngestResult, error) {
	if s.repo == nil {
		return IngestResult{}, errors.New("rag repository未配置")
	}
	if input.OwnerID <= 0 {
		return IngestResult{}, errors.New("owner_id不能为空")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
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
		return IngestResult{}, err
	}
	if err := s.repo.CreateDocumentWithChunks(ctx, doc, chunks); err != nil {
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
		entityCount, relationCount = s.buildGraph(ctx, input.OwnerID, doc.ID, doc.SourceType, chunks)
	}
	dto := documentToRPC(doc)
	return IngestResult{Document: dto, ChunkCount: int64(countChildChunks(chunks)), EntityCount: entityCount, RelationCount: relationCount}, nil
}

// Search 执行 Adaptive RAG：按问题复杂度选择 Hybrid、GraphRAG、Web/Memory 外部路线或简单直答，并附带 CRAG 和 Self-RAG 检查点。
func (s *ragServiceImpl) Search(ctx context.Context, input SearchInput) (SearchResult, error) {
	if s.repo == nil {
		return SearchResult{}, errors.New("rag repository未配置")
	}
	if input.ViewerID <= 0 {
		return SearchResult{}, errors.New("viewer_id不能为空")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
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
	nodes, edges, communities, err := s.repo.ListGraph(ctx, input.ViewerID, input.Query, input.Limit, input.DocumentID, input.Hops)
	if err != nil {
		return GraphResult{}, err
	}
	if input.DocumentID > 0 && len(edges) == 0 {
		s.rebuildDocumentGraphIfMissing(ctx, input.ViewerID, input.DocumentID)
		nodes, edges, communities, err = s.repo.ListGraph(ctx, input.ViewerID, input.Query, input.Limit, input.DocumentID, input.Hops)
		if err != nil {
			return GraphResult{}, err
		}
	}
	nodes, edges = filterGraphForDisplay(nodes, edges)
	msg := ""
	if len(nodes) == 0 || len(edges) == 0 {
		msg = s.describeEmptyGraph(ctx, input.ViewerID, input.DocumentID, len(nodes), len(edges))
	}
	return GraphResult{Success: true, Nodes: entitiesToRPC(nodes), Edges: relationsToRPC(edges), Communities: communitiesToRPC(communities), Msg: msg}, nil
}

func (s *ragServiceImpl) rebuildDocumentGraphIfMissing(ctx context.Context, viewerID, documentID int64) {
	if s.repo == nil || documentID <= 0 {
		return
	}
	rows, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: viewerID, DocumentID: documentID, Limit: 1200})
	if err != nil || len(rows) == 0 {
		return
	}
	chunks := make([]model.Chunk, 0, len(rows))
	sourceType := rows[0].Document.SourceType
	ownerID := rows[0].Document.OwnerID
	for _, row := range rows {
		chunks = append(chunks, row.Chunk)
		if strings.TrimSpace(sourceType) == "" {
			sourceType = row.Document.SourceType
		}
		if ownerID <= 0 {
			ownerID = row.Document.OwnerID
		}
	}
	if ownerID <= 0 || !shouldBuildGraphForSourceType(sourceType) {
		return
	}
	s.buildGraph(ctx, ownerID, documentID, sourceType, chunks)
}

func (s *ragServiceImpl) describeEmptyGraph(ctx context.Context, viewerID, documentID int64, nodeCount, edgeCount int) string {
	if documentID <= 0 {
		if nodeCount == 0 {
			return "当前可见知识库没有可展示的图谱节点。请先上传适合 GraphRAG 的文档，或检查 RAG Router/GraphRAG 小模型配置。"
		}
		return "当前图谱实体存在，但没有可展示的有效关系；可能是关系被质量过滤、关系证据不足，或只命中了孤立实体。"
	}
	rows, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: viewerID, DocumentID: documentID, Limit: 2000})
	if err != nil || len(rows) == 0 {
		return "该文档没有可读取的 RAG 分块，无法构建知识图谱。请确认文档已成功解析入库。"
	}
	sourceType := rows[0].Document.SourceType
	if !shouldBuildGraphForSourceType(sourceType) {
		return "该文档属于会话摘要/聊天归档类型，系统不会把它写入知识图谱。"
	}
	if nodeCount == 0 {
		return fmt.Sprintf("该文档共有 %d 个可读分块，但没有通过 GraphRAG 实体/关系质量过滤。请检查 GraphRAG 小模型配置，或确认文档中是否有明确的主体、概念和关系。", len(rows))
	}
	return fmt.Sprintf("该文档共有 %d 个可读分块，已抽取实体但缺少有效关系；可能是关系证据不足或被关系类型规则过滤。", len(rows))
}

func (s *ragServiceImpl) ListDocuments(ctx context.Context, viewerID int64, limit, offset int) ([]rag.RAGDocument, int64, error) {
	docs, total, err := s.repo.ListDocuments(ctx, dao.SearchFilter{ViewerID: viewerID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(docs))
	for i := range docs {
		ids = append(ids, docs[i].ID)
	}
	chunkCounts, err := s.repo.CountChildChunksByDocumentIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]rag.RAGDocument, 0, len(docs))
	for i := range docs {
		dto := documentToRPC(&docs[i])
		dto.ChunkCount = chunkCounts[docs[i].ID]
		out = append(out, dto)
	}
	return out, total, nil
}

func (s *ragServiceImpl) DeleteDocument(ctx context.Context, viewerID, documentID int64) error {
	if s.repo == nil {
		return errors.New("rag repository未配置")
	}
	return s.repo.DeleteDocument(ctx, viewerID, documentID)
}

func (s *ragServiceImpl) DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error {
	if s.repo == nil {
		return errors.New("rag repository未配置")
	}
	return s.repo.DeleteDocumentGraph(ctx, viewerID, documentID)
}

func (s *ragServiceImpl) route(ctx context.Context, input SearchInput) RouterDecision {
	routerInput := RouterInput{
		Query:          input.Query,
		DefaultMode:    s.defaultMode,
		GroupID:        input.GroupID,
		ConversationID: input.ConversationID,
	}
	router := s.resolveUserRouter(ctx, input.ViewerID)
	decision, err := router.Route(ctx, routerInput)
	if err != nil {
		fallback, _ := HybridAdaptiveRouter{}.Route(ctx, routerInput)
		fallback.Reason = "RAG Router不可用，已降级规则路由"
		return fallback
	}
	decision = normalizeRouterDecision(decision, s.defaultMode, input.Query)
	if !decision.Retrieve {
		decision.Mode = "direct"
	}
	return decision
}

func routerNote(decision RouterDecision) string {
	return fmt.Sprintf(
		"Adaptive Router: route=%s complexity=%s need_retrieve=%t sources=%s strategy=%s reason=%s",
		decision.Route,
		decision.Complexity,
		decision.Retrieve,
		strings.Join(decision.Sources, ","),
		decision.Strategy,
		defaultString(decision.Reason, "已选择检索路线"),
	)
}

func (s *ragServiceImpl) resolveUserRouter(ctx context.Context, viewerID int64) RAGRouter {
	defaultRouter := s.router
	if defaultRouter == nil {
		defaultRouter = RuleRouter{}
	}
	if s.settings == nil || viewerID <= 0 || s.routerFactory == nil {
		return defaultRouter
	}
	profiles, err := s.settings.ListLLMProfiles(ctx, viewerID, settingsclient.ProviderRAGRouter)
	if err != nil || len(profiles) == 0 {
		return defaultRouter
	}
	selected := profiles[0]
	for _, profile := range profiles {
		if profile.IsDefault {
			selected = profile
			break
		}
	}
	if selected.ID <= 0 {
		return defaultRouter
	}
	resolved, err := s.settings.ResolveLLMProfile(ctx, viewerID, selected.ID)
	if err != nil {
		return defaultRouter
	}
	if strings.TrimSpace(resolved.APIKey) == "" || strings.TrimSpace(resolved.BaseURL) == "" || strings.TrimSpace(resolved.ModelName) == "" {
		return defaultRouter
	}
	router := s.routerFactory(resolved)
	if router == nil {
		return defaultRouter
	}
	return fallbackRouter{primary: router, fallback: defaultRouter}
}

type fallbackRouter struct {
	primary  RAGRouter
	fallback RAGRouter
}

func (r fallbackRouter) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	decision, err := r.primary.Route(ctx, input)
	if err == nil {
		return decision, nil
	}
	if r.fallback == nil {
		return RouterDecision{}, err
	}
	return r.fallback.Route(ctx, input)
}

type chunkPlan struct {
	parentTitle   string
	parentContent string
	parentSummary string
	children      []string
}

func buildHierarchicalChunks(input IngestInput, content string) ([]model.Chunk, error) {
	plans := planHierarchicalChunks(defaultString(input.SourceType, "text"), content)
	if len(plans) == 0 {
		return nil, errors.New("未能生成有效知识分块")
	}
	chunks := make([]model.Chunk, 0, len(plans)*3)
	index := 0
	for _, plan := range plans {
		parentID, err := idgen.NextID()
		if err != nil {
			return nil, err
		}
		parentContent := strings.TrimSpace(plan.parentContent)
		parentSummary := parentSummary(plan.parentTitle, plan.parentSummary, parentContent)
		parentKeywords := strings.Join(extractKeywords(plan.parentTitle+" "+parentSummary+" "+parentContent, 24), ",")
		chunks = append(chunks, model.Chunk{
			ID:            parentID,
			ParentChunkID: 0,
			ChunkLevel:    model.ChunkLevelParent,
			ChunkIndex:    index,
			Content:       parentContent,
			Summary:       parentSummary,
			Keywords:      parentKeywords,
			EmbeddingRef:  fmt.Sprintf("mysql://rag_chunks/%d/parent", parentID),
			QualityScore:  qualityScore(parentContent),
		})
		index++
		children := plan.children
		if len(children) == 0 {
			children = splitIntoChunks(parentContent, 420)
		}
		for _, child := range children {
			child = strings.TrimSpace(child)
			if child == "" {
				continue
			}
			chunks = append(chunks, model.Chunk{
				ParentChunkID: parentID,
				ChunkLevel:    model.ChunkLevelChild,
				ChunkIndex:    index,
				Content:       child,
				Summary:       parentSummary,
				Keywords:      strings.Join(extractKeywords(plan.parentTitle+" "+child, 16), ","),
				EmbeddingRef:  fmt.Sprintf("milvus://claran_rag_chunks/pending/%d", index),
				QualityScore:  qualityScore(child),
			})
			index++
		}
	}
	return chunks, nil
}

func planHierarchicalChunks(sourceType, content string) []chunkPlan {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "md", "markdown":
		return planMarkdownChunks(content)
	case "go", "code", "golang":
		return planGoCodeChunks(content)
	case "conversation", "chat":
		return planConversationChunks(content)
	default:
		return planParagraphChunks(content)
	}
}

func planMarkdownChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	documentTitle := ""
	type section struct {
		title string
		lines []string
	}
	var sections []section
	current := section{title: "正文"}
	flush := func() {
		body := strings.TrimSpace(strings.Join(current.lines, "\n"))
		if body == "" && strings.TrimSpace(current.title) == "" {
			return
		}
		sections = append(sections, current)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && documentTitle == "" {
			documentTitle = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			flush()
			current = section{title: strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))}
			continue
		}
		current.lines = append(current.lines, line)
	}
	flush()
	plans := make([]chunkPlan, 0, len(sections))
	for _, sec := range sections {
		body := strings.TrimSpace(strings.Join(sec.lines, "\n"))
		if body == "" {
			continue
		}
		title := strings.TrimSpace(sec.title)
		if title == "" || title == "正文" {
			title = defaultString(documentTitle, "正文")
		}
		parentContent := strings.TrimSpace("## " + title + "\n" + body)
		children := splitMarkdownChildren(body, title)
		plans = append(plans, chunkPlan{parentTitle: title, parentContent: parentContent, children: children})
	}
	return plans
}

func splitMarkdownChildren(body, title string) []string {
	var children []string
	var current []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(current, "\n"))
		if text != "" {
			children = append(children, splitIntoChunks(text, 420)...)
		}
		current = nil
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			flush()
			current = append(current, "## "+title, line)
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return children
}

func planParagraphChunks(content string) []chunkPlan {
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return nil
	}
	var plans []chunkPlan
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		text := strings.Join(current, "\n\n")
		plans = append(plans, chunkPlan{parentTitle: firstLine(text, "段落"), parentContent: text, children: splitIntoChunks(text, 420)})
		current = nil
		currentLen = 0
	}
	for _, paragraph := range paragraphs {
		l := len([]rune(paragraph))
		if currentLen > 0 && currentLen+l > 1500 {
			flush()
		}
		current = append(current, paragraph)
		currentLen += l
	}
	flush()
	return plans
}

func splitParagraphs(content string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	parts := regexp.MustCompile(`\n\s*\n`).Split(normalized, -1)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, splitLongParagraph(part, 1100)...)
		}
	}
	if len(out) == 0 && strings.TrimSpace(content) != "" {
		out = splitIntoChunks(content, 1200)
	}
	return out
}

func splitLongParagraph(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 1100
	}
	if len([]rune(text)) <= maxRunes {
		return []string{text}
	}
	lines := strings.Split(text, "\n")
	nonEmptyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}
	if len(nonEmptyLines) <= 1 {
		return splitIntoChunks(text, maxRunes)
	}
	out := make([]string, 0, len(nonEmptyLines)/4+1)
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, strings.Join(current, "\n"))
		current = nil
		currentLen = 0
	}
	for _, line := range nonEmptyLines {
		lineLen := len([]rune(line))
		if currentLen > 0 && currentLen+lineLen > maxRunes {
			flush()
		}
		if lineLen > maxRunes {
			flush()
			out = append(out, splitIntoChunks(line, maxRunes)...)
			continue
		}
		current = append(current, line)
		currentLen += lineLen
	}
	flush()
	return out
}

func planConversationChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var messages []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			messages = append(messages, line)
		}
	}
	if len(messages) == 0 {
		return nil
	}
	var plans []chunkPlan
	for start := 0; start < len(messages); start += 20 {
		end := start + 20
		if end > len(messages) {
			end = len(messages)
		}
		parent := strings.Join(messages[start:end], "\n")
		children := make([]string, 0, end-start)
		for i := start; i < end; i += 3 {
			j := i + 3
			if j > end {
				j = end
			}
			children = append(children, strings.Join(messages[i:j], "\n"))
		}
		plans = append(plans, chunkPlan{parentTitle: fmt.Sprintf("聊天记录 %d-%d", start+1, end), parentContent: parent, children: children})
	}
	return plans
}

func planGoCodeChunks(content string) []chunkPlan {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var prelude []string
	var pendingComments []string
	var blocks []chunkPlan
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if isGoCommentLine(line) {
			pendingComments = append(pendingComments, lines[i])
			i++
			continue
		}
		if line == "" {
			if len(pendingComments) > 0 {
				pendingComments = append(pendingComments, lines[i])
			} else {
				prelude = append(prelude, lines[i])
			}
			i++
			continue
		}
		if isGoDeclarationStart(line) {
			blockLines := append([]string{}, pendingComments...)
			pendingComments = nil
			braceBalance := strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
			blockLines = append(blockLines, lines[i])
			i++
			for braceBalance > 0 && i < len(lines) {
				braceBalance += strings.Count(lines[i], "{") - strings.Count(lines[i], "}")
				blockLines = append(blockLines, lines[i])
				i++
			}
			block := strings.TrimSpace(strings.Join(blockLines, "\n"))
			blocks = append(blocks, chunkPlan{parentTitle: firstLine(block, "代码块"), parentContent: block, children: splitCodeChildren(block)})
			continue
		}
		if len(pendingComments) > 0 {
			prelude = append(prelude, pendingComments...)
			pendingComments = nil
		}
		if strings.HasPrefix(line, "package ") || strings.HasPrefix(line, "import ") {
			prelude = append(prelude, lines[i])
		}
		i++
	}
	if len(pendingComments) > 0 {
		prelude = append(prelude, pendingComments...)
	}
	if text := strings.TrimSpace(strings.Join(prelude, "\n")); text != "" {
		blocks = append([]chunkPlan{{parentTitle: "package/import", parentContent: text, children: splitIntoChunks(text, 420)}}, blocks...)
	}
	if len(blocks) == 0 {
		return planParagraphChunks(content)
	}
	return blocks
}

func isGoCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
}

func isGoDeclarationStart(line string) bool {
	return strings.HasPrefix(line, "func ") ||
		strings.HasPrefix(line, "type ") ||
		strings.HasPrefix(line, "var ") ||
		strings.HasPrefix(line, "const ")
}

func splitCodeChildren(block string) []string {
	if len([]rune(block)) <= 900 {
		return []string{block}
	}
	return splitIntoChunks(block, 700)
}

func parentSummary(title, existing, content string) string {
	if strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing)
	}
	paragraphs := splitParagraphs(content)
	if len(paragraphs) == 0 {
		return truncate(title, 240)
	}
	summary := strings.TrimSpace(paragraphs[0])
	if title != "" && !strings.Contains(summary, title) {
		summary = title + "： " + summary
	}
	return truncate(summary, 420)
}

func countChildChunks(chunks []model.Chunk) int {
	count := 0
	for _, chunk := range chunks {
		if normalizedChunkLevel(chunk) == model.ChunkLevelChild {
			count++
		}
	}
	return count
}

func normalizedChunkLevel(chunk model.Chunk) string {
	if strings.TrimSpace(chunk.ChunkLevel) == "" {
		return model.ChunkLevelChild
	}
	return chunk.ChunkLevel
}

func (s *ragServiceImpl) hybridRetrieve(ctx context.Context, input SearchInput, limit int) ([]rankedChunk, error) {
	rowLimit := 300
	if input.DocumentID > 0 {
		rowLimit = 1500
	}
	rows, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: input.ViewerID, GroupID: input.GroupID, ConversationID: input.ConversationID, DocumentID: input.DocumentID, Limit: rowLimit})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	childRows, parentRows := splitParentChildRows(rows)
	if len(childRows) == 0 {
		childRows = rows
	}
	queryVector := s.embedding(ctx, input.Query)
	vectorHits, _ := s.vectorIndex.Search(ctx, queryVector, limit)
	rowByChunkID := make(map[int64]dao.ChunkWithDocument, len(childRows))
	for _, row := range childRows {
		rowByChunkID[row.Chunk.ID] = row
	}
	denseRank := map[int64]int{}
	denseScore := map[int64]float64{}
	for _, hit := range vectorHits {
		if _, ok := rowByChunkID[hit.ChunkID]; !ok {
			continue
		}
		denseScore[hit.ChunkID] = hit.Score
		if _, exists := denseRank[hit.ChunkID]; !exists {
			denseRank[hit.ChunkID] = len(denseRank) + 1
		}
	}
	if len(denseRank) == 0 {
		denseRows := make([]rankedChunk, 0, len(childRows))
		for _, row := range childRows {
			score := cosine(queryVector, s.embedding(ctx, row.Chunk.Content))
			denseRows = append(denseRows, rankedChunk{Chunk: row.Chunk, Document: row.Document, Score: score})
		}
		sort.Slice(denseRows, func(i, j int) bool { return denseRows[i].Score > denseRows[j].Score })
		if len(denseRows) > limit {
			denseRows = denseRows[:limit]
		}
		for idx, row := range denseRows {
			denseRank[row.Chunk.ID] = idx + 1
			denseScore[row.Chunk.ID] = row.Score
		}
	}
	bm25Rows := bm25RankChunks(input.Query, childRows)
	if len(bm25Rows) > limit {
		bm25Rows = bm25Rows[:limit]
	}
	bm25Rank := map[int64]int{}
	bm25Score := map[int64]float64{}
	for idx, row := range bm25Rows {
		bm25Rank[row.Chunk.ID] = idx + 1
		bm25Score[row.Chunk.ID] = row.Score
	}
	candidates := map[int64]dao.ChunkWithDocument{}
	for chunkID := range denseRank {
		candidates[chunkID] = rowByChunkID[chunkID]
	}
	for _, row := range bm25Rows {
		candidates[row.Chunk.ID] = rowByChunkID[row.Chunk.ID]
	}
	out := make([]rankedChunk, 0, len(candidates))
	for _, row := range childRows {
		if _, ok := candidates[row.Chunk.ID]; !ok {
			continue
		}
		hierarchy := 0.05
		if input.GroupID > 0 && row.Document.GroupID == input.GroupID {
			hierarchy += 0.08
		}
		if input.ConversationID > 0 && row.Document.ConversationID == input.ConversationID {
			hierarchy += 0.08
		}
		score := reciprocalRankFusionScore(denseRank[row.Chunk.ID], bm25Rank[row.Chunk.ID]) + 0.01*hierarchy
		out = append(out, rankedChunk{
			Chunk:    row.Chunk,
			Document: row.Document,
			Score:    score,
			Reason: fmt.Sprintf(
				"hybrid rrf=%.4f dense_rank=%s dense=%.3f bm25_rank=%s bm25=%.3f",
				score,
				formatRank(denseRank[row.Chunk.ID]),
				denseScore[row.Chunk.ID],
				formatRank(bm25Rank[row.Chunk.ID]),
				bm25Score[row.Chunk.ID],
			),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	out = groupChildHitsByParent(out, parentRows)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func splitParentChildRows(rows []dao.ChunkWithDocument) ([]dao.ChunkWithDocument, map[int64]dao.ChunkWithDocument) {
	children := make([]dao.ChunkWithDocument, 0, len(rows))
	parents := map[int64]dao.ChunkWithDocument{}
	for _, row := range rows {
		switch normalizedChunkLevel(row.Chunk) {
		case model.ChunkLevelParent:
			parents[row.Chunk.ID] = row
		default:
			children = append(children, row)
		}
	}
	return children, parents
}

func groupChildHitsByParent(children []rankedChunk, parents map[int64]dao.ChunkWithDocument) []rankedChunk {
	grouped := make([]rankedChunk, 0, len(children))
	seen := map[int64]bool{}
	for _, child := range children {
		parentID := child.Chunk.ParentChunkID
		if parentID == 0 {
			parentID = child.Chunk.ID
		}
		if seen[parentID] {
			continue
		}
		seen[parentID] = true
		parentChunk := child.Chunk
		if parentRow, ok := parents[parentID]; ok {
			parentChunk = parentRow.Chunk
		}
		parentChunk.Content = parentContextContent(parentChunk, child.Chunk)
		reason := child.Reason + fmt.Sprintf("; child_chunk_id=%d parent_chunk_id=%d", child.Chunk.ID, parentID)
		grouped = append(grouped, rankedChunk{
			Chunk:    parentChunk,
			Document: child.Document,
			Score:    child.Score,
			Reason:   reason,
		})
	}
	return grouped
}

func parentContextContent(parentChunk, hitChild model.Chunk) string {
	summary := strings.TrimSpace(parentChunk.Summary)
	parentContent := strings.TrimSpace(parentChunk.Content)
	childContent := strings.TrimSpace(hitChild.Content)
	if parentContent == "" {
		return childContent
	}
	if len([]rune(parentContent)) <= 1200 {
		if summary != "" {
			return summary + "\n\n" + parentContent
		}
		return parentContent
	}
	var parts []string
	if summary != "" {
		parts = append(parts, "父块摘要：\n"+summary)
	}
	if childContent != "" {
		parts = append(parts, "命中小块：\n"+childContent)
	}
	if len(parts) == 0 {
		return truncate(parentContent, 1200)
	}
	return strings.Join(parts, "\n\n")
}

func reciprocalRankFusionScore(denseRank, bm25Rank int) float64 {
	const k = 60.0
	score := 0.0
	if denseRank > 0 {
		score += 1 / (k + float64(denseRank))
	}
	if bm25Rank > 0 {
		score += 1 / (k + float64(bm25Rank))
	}
	return score
}

func formatRank(rank int) string {
	if rank <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", rank)
}

func bm25RankChunks(query string, rows []dao.ChunkWithDocument) []rankedChunk {
	queryTerms := bm25Tokens(query)
	if len(queryTerms) == 0 || len(rows) == 0 {
		return nil
	}
	docFreq := map[string]int{}
	docTokens := make([][]string, len(rows))
	totalLen := 0
	for i, row := range rows {
		tokens := bm25Tokens(row.Document.Title + " " + row.Chunk.Content + " " + row.Chunk.Keywords)
		docTokens[i] = tokens
		totalLen += len(tokens)
		seen := map[string]bool{}
		for _, token := range tokens {
			if !seen[token] {
				docFreq[token]++
				seen[token] = true
			}
		}
	}
	avgDocLen := float64(totalLen) / float64(len(rows))
	if avgDocLen <= 0 {
		avgDocLen = 1
	}
	const k1 = 1.5
	const b = 0.75
	n := float64(len(rows))
	out := make([]rankedChunk, 0, len(rows))
	for i, row := range rows {
		tokens := docTokens[i]
		if len(tokens) == 0 {
			continue
		}
		tf := map[string]int{}
		for _, token := range tokens {
			tf[token]++
		}
		score := 0.0
		docLen := float64(len(tokens))
		for _, term := range queryTerms {
			freq := float64(tf[term])
			if freq == 0 {
				continue
			}
			df := float64(docFreq[term])
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			denominator := freq + k1*(1-b+b*docLen/avgDocLen)
			score += idf * (freq * (k1 + 1)) / denominator
		}
		if score > 0 {
			out = append(out, rankedChunk{Chunk: row.Chunk, Document: row.Document, Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func bm25Tokens(text string) []string {
	raw := wordRe.FindAllString(strings.ToLower(text), -1)
	out := make([]string, 0, len(raw)*2)
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out = append(out, token)
		runes := []rune(token)
		if len(runes) > 2 && containsHan(token) {
			for i := 0; i < len(runes)-1; i++ {
				out = append(out, string(runes[i:i+2]))
			}
		}
	}
	return out
}

func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (s *ragServiceImpl) embedding(ctx context.Context, text string) []float32 {
	if s.embedder != nil {
		if vector, err := s.embedder.Embed(ctx, text); err == nil && len(vector) > 0 {
			return normalizeVectorDim(vector, s.embeddingDim)
		}
	}
	return hashEmbedding(text, s.embeddingDim)
}

func normalizeVectorDim(vector []float32, dim int) []float32 {
	if dim <= 0 || len(vector) == dim {
		return vector
	}
	out := make([]float32, dim)
	copy(out, vector)
	return out
}

type graphExtractInput struct {
	DocumentID int64
	Chunk      model.Chunk
}

type extractedEntity struct {
	Name        string
	Type        string
	Description string
	Aliases     []string
}

type extractedRelationship struct {
	Source      string
	Target      string
	Type        string
	Description string
	Evidence    string
	Confidence  float64
}

type graphExtractResult struct {
	Entities      []extractedEntity
	Relationships []extractedRelationship
}

type graphEntityMention struct {
	Name  string
	Start int
	End   int
}

type graphRelationTrigger struct {
	Type  string
	Index int
	End   int
}

type graphExtractor interface {
	Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error)
}

type graphCommunitySummarizer interface {
	Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error)
}

type ruleGraphExtractor struct{}

func shouldBuildGraphForSourceType(sourceType string) bool {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "conversation", "chat", "conversation_summary", "conversation_topic", "conversation_digest":
		return false
	default:
		return true
	}
}

func (s *ragServiceImpl) buildGraph(ctx context.Context, ownerID, documentID int64, sourceType string, chunks []model.Chunk) (int64, int64) {
	if !shouldBuildGraphForSourceType(sourceType) {
		return 0, 0
	}
	hasConfiguredExtractor := s.llmGraphEnabled
	extractor := s.graphExtractor
	if extractor == nil {
		extractor = ruleGraphExtractor{}
	}
	candidates := graphExtractionCandidates(chunks)
	entityByCanonical := map[string]*model.Entity{}
	seenRelations := map[string]bool{}
	entityCount := int64(0)
	relationCount := int64(0)
	for _, chunk := range candidates {
		result, err := extractor.Extract(ctx, graphExtractInput{DocumentID: documentID, Chunk: chunk})
		if err != nil {
			continue
		}
		result = filterGraphExtractResult(result, chunk.Content)
		for _, extracted := range result.Entities {
			entity := s.upsertGraphEntity(ctx, ownerID, extracted)
			if entity == nil {
				continue
			}
			if _, exists := entityByCanonical[entity.CanonicalKey]; !exists {
				entityCount++
			}
			entityByCanonical[entity.CanonicalKey] = entity
		}
		for _, extracted := range result.Relationships {
			source := entityByCanonical[canonicalEntityKey(extracted.Source)]
			target := entityByCanonical[canonicalEntityKey(extracted.Target)]
			if source == nil || target == nil || source.ID == 0 || target.ID == 0 || source.ID == target.ID {
				continue
			}
			confidence := extracted.Confidence
			if confidence <= 0 {
				confidence = 0.72
			}
			relationType := normalizeRelationType(extracted.Type)
			relationKey := fmt.Sprintf("%d:%d:%s:%d", source.ID, target.ID, relationType, chunk.ID)
			if seenRelations[relationKey] {
				continue
			}
			seenRelations[relationKey] = true
			relation := &model.Relation{
				OwnerID:         ownerID,
				SourceID:        source.ID,
				TargetID:        target.ID,
				Relation:        relationType,
				Description:     defaultString(extracted.Description, source.Name+" 与 "+target.Name+" 存在知识图谱关系"),
				Weight:          confidence,
				Confidence:      confidence,
				Evidence:        truncate(defaultString(extracted.Evidence, chunk.Content), 500),
				EvidenceChunkID: chunk.ID,
				DocumentID:      documentID,
			}
			_ = s.repo.SaveRelation(ctx, relation)
			relationCount++
		}
	}
	if (!hasConfiguredExtractor || shouldUseSparseGraphFallback(candidates)) && entityCount == 0 && relationCount == 0 {
		fallbackEntities, fallbackRelations := s.buildFallbackTopicGraph(ctx, ownerID, documentID, candidates)
		entityCount += fallbackEntities
		relationCount += fallbackRelations
	}
	s.rebuildGraphCommunities(ctx, ownerID)
	return entityCount, relationCount
}

func shouldUseSparseGraphFallback(chunks []model.Chunk) bool {
	if len(chunks) >= 4 {
		return true
	}
	total := 0
	for _, chunk := range chunks {
		total += len([]rune(strings.TrimSpace(chunk.Content)))
	}
	return total >= 1800
}

func (s *ragServiceImpl) buildFallbackTopicGraph(ctx context.Context, ownerID, documentID int64, chunks []model.Chunk) (int64, int64) {
	if s.repo == nil || len(chunks) == 0 {
		return 0, 0
	}
	entityByKey := map[string]*model.Entity{}
	entityCount := int64(0)
	relationCount := int64(0)
	for _, chunk := range chunks {
		entities := fallbackTopicEntities(chunk)
		if len(entities) < 2 {
			continue
		}
		var saved []*model.Entity
		for _, extracted := range entities {
			entity := s.upsertGraphEntity(ctx, ownerID, extracted)
			if entity == nil {
				continue
			}
			if _, ok := entityByKey[entity.CanonicalKey]; !ok {
				entityCount++
			}
			entityByKey[entity.CanonicalKey] = entity
			saved = append(saved, entity)
		}
		for i := 0; i+1 < len(saved) && i < 3; i++ {
			source := saved[i]
			target := saved[i+1]
			if source == nil || target == nil || source.ID == target.ID {
				continue
			}
			relation := &model.Relation{
				OwnerID:         ownerID,
				SourceID:        source.ID,
				TargetID:        target.ID,
				Relation:        "RELATED_TO",
				Description:     "同一文档章节的核心主题关系，用于在规则抽取为空时保留可视化骨架",
				Weight:          0.9,
				Confidence:      0.9,
				Evidence:        truncate(chunk.Content, 500),
				EvidenceChunkID: chunk.ID,
				DocumentID:      documentID,
			}
			_ = s.repo.SaveRelation(ctx, relation)
			relationCount++
		}
		if entityCount >= 10 || relationCount >= 8 {
			break
		}
	}
	return entityCount, relationCount
}

func fallbackTopicEntities(chunk model.Chunk) []extractedEntity {
	text := strings.TrimSpace(chunk.Summary + " " + chunk.Content)
	keywords := extractKeywords(text, 24)
	out := make([]extractedEntity, 0, 4)
	seen := map[string]bool{}
	for _, keyword := range keywords {
		name := normalizeFallbackTopicName(keyword)
		if name == "" || seen[canonicalEntityKey(name)] {
			continue
		}
		seen[canonicalEntityKey(name)] = true
		out = append(out, extractedEntity{
			Name:        name,
			Type:        "Concept",
			Description: "文档章节中的核心主题词，用于辅助知识图谱可视化和后续人工审核",
			Aliases:     []string{name},
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func normalizeFallbackTopicName(keyword string) string {
	name := strings.Trim(strings.TrimSpace(keyword), "，。；;,.、:：()（）[]【】")
	if name == "" || len([]rune(name)) < 3 {
		return ""
	}
	lower := strings.ToLower(name)
	if regexp.MustCompile(`^\d+$`).MatchString(lower) || regexp.MustCompile(`^p?\d+[-_]\d+$`).MatchString(lower) {
		return ""
	}
	blocked := map[string]bool{
		"文档": true, "内容": true, "信息": true, "数据": true, "图片": true, "文件": true, "页面": true,
		"用户": true, "系统": true, "示例": true, "步骤": true, "章节": true, "标题": true, "正文": true,
		"document": true, "content": true, "image": true, "file": true, "example": true,
	}
	if blocked[lower] || isGenericDocumentSectionEntity(name) || isGraphStopEntityName(name) {
		return ""
	}
	return name
}

func graphExtractionCandidates(chunks []model.Chunk) []model.Chunk {
	const maxParentCandidates = 36
	const maxChildFallbackCandidates = 36
	parents := make([]model.Chunk, 0)
	children := make([]model.Chunk, 0)
	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if len([]rune(content)) < 80 {
			continue
		}
		switch normalizedChunkLevel(chunk) {
		case model.ChunkLevelParent:
			enriched := chunk
			if summary := strings.TrimSpace(chunk.Summary); summary != "" && !strings.Contains(content, summary) {
				enriched.Content = strings.TrimSpace("章节摘要： " + summary + "\n\n章节正文：\n" + content)
			}
			parents = append(parents, enriched)
		case model.ChunkLevelChild:
			children = append(children, chunk)
		}
	}
	if len(parents) == 0 && len(children) == 0 {
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk.Content) == "" {
				continue
			}
			if normalizedChunkLevel(chunk) == model.ChunkLevelParent {
				parents = append(parents, chunk)
			} else {
				children = append(children, chunk)
			}
			if len(parents)+len(children) >= 8 {
				break
			}
		}
	}
	sort.SliceStable(parents, func(i, j int) bool {
		return parents[i].QualityScore > parents[j].QualityScore
	})
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].QualityScore > children[j].QualityScore
	})
	if len(parents) > 0 {
		if len(parents) > maxParentCandidates {
			parents = parents[:maxParentCandidates]
		}
		return parents
	}
	if len(children) > maxChildFallbackCandidates {
		children = children[:maxChildFallbackCandidates]
	}
	return children
}

func (s *ragServiceImpl) upsertGraphEntity(ctx context.Context, ownerID int64, extracted extractedEntity) *model.Entity {
	name := strings.TrimSpace(extracted.Name)
	if name == "" {
		return nil
	}
	canonical := canonicalEntityKey(name)
	if canonical == "" {
		return nil
	}
	entity, err := s.repo.GetEntityByCanonicalKey(ctx, ownerID, canonical)
	if err != nil {
		return nil
	}
	if entity == nil {
		entity = &model.Entity{
			OwnerID:      ownerID,
			Name:         name,
			CanonicalKey: canonical,
			Type:         normalizeEntityType(extracted.Type, name),
			Summary:      defaultString(extracted.Description, "从文档分块中抽取的知识图谱实体"),
			AliasesJSON:  encodeAliases(mergeAliases(nil, append(extracted.Aliases, name)...)),
			Score:        1,
		}
	} else {
		entity.Score += 0.2
		entity.AliasesJSON = encodeAliases(mergeAliases(decodeAliases(entity.AliasesJSON), append(extracted.Aliases, name)...))
		if strings.TrimSpace(entity.Summary) == "" || entity.Summary == "从文档分块中抽取的知识图谱实体" {
			entity.Summary = defaultString(extracted.Description, entity.Summary)
		}
		if strings.TrimSpace(entity.Type) == "" || entity.Type == "Concept" {
			entity.Type = normalizeEntityType(extracted.Type, name)
		}
	}
	_ = s.repo.SaveEntity(ctx, entity)
	return entity
}

type fallbackGraphExtractor struct {
	primary  graphExtractor
	fallback graphExtractor
}

func (e fallbackGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	if e.primary != nil {
		result, err := e.primary.Extract(ctx, input)
		if err == nil && len(result.Entities) > 0 {
			return result, nil
		}
	}
	if e.fallback == nil {
		return graphExtractResult{}, nil
	}
	return e.fallback.Extract(ctx, input)
}

// LLMGraphExtractor 使用 OpenAI-compatible 小模型抽取 GraphRAG 实体和关系。
type LLMGraphExtractor struct {
	APIKey   string
	BaseURL  string
	Model    string
	Client   *http.Client
	MaxChars int
}

func NewLLMGraphExtractor(apiKey, baseURL, model string) *LLMGraphExtractor {
	return &LLMGraphExtractor{
		APIKey:   strings.TrimSpace(apiKey),
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:    defaultString(model, "glm-4-flash"),
		Client:   &http.Client{Timeout: 20 * time.Second},
		MaxChars: 4000,
	}
}

func (e *LLMGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	if e == nil || e.APIKey == "" || e.BaseURL == "" {
		return graphExtractResult{}, errors.New("llm graph extractor未配置")
	}
	content := truncate(input.Chunk.Content, e.MaxChars)
	payload := map[string]interface{}{
		"model": e.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是 ClaranAIM 的 GraphRAG indexing analyst。只输出 JSON，不要解释。你的任务不是抽取所有名词，而是阅读一篇文档的章节/父块，判断“这段文章真正想让读者记住的核心实体和关系”。\n\n分析步骤必须在心里完成，不要输出步骤：\n1. 先判断本章节的主题和作者真正讨论的对象。\n2. 只保留解释该主题不可缺少的实体。一个词如果删掉后不影响理解，就不要抽。\n3. 只抽明确关系：A 调用/写入/依赖/配置/触发/发布/消费/拥有/读取/存储 B。只是同段出现、举例、列表项、标题编号、页面元素，不算关系。\n4. 章节如果只是寒暄、会话摘要、目录、题号、步骤流水、截图 OCR 噪声、普通示例代码或无长期价值内容，返回空数组。\n\n硬性规则：\n- 每段最多 6 个实体、6 条关系；宁缺毋滥。\n- 不要抽取普通数字、序号、页码、字段名、状态词、文件名、图片名、临时变量、普通英文人名样例、Teacher/Student/Customer/Linux 这类教学例子，除非文章明确就是在定义它们。\n- 不要把“会话摘要、会话主题、文档、内容、数据、信息、用户、系统、示例、页面、图片、文件”作为实体。\n- 实体 description 必须说明它在本文中的业务/技术含义，不能写泛泛定义。\n- source/target 必须引用 entities 中的 name；关系必须有原文证据；没有明确动作就不要输出 RELATED_TO。\n\n实体类型只能是 Service、DatabaseTable、EventTopic、API、Module、Concept、Person、Organization、Product。关系类型只能是 CALLS、PUBLISHES、CONSUMES、STORES、OWNS、DEPENDS_ON、CONFIGURES、TRIGGERS、READS、WRITES、RELATED_TO。\n\nJSON格式：{\"entities\":[{\"name\":\"...\",\"type\":\"Concept\",\"description\":\"它在本文中的具体含义\",\"aliases\":[]}],\"relationships\":[{\"source\":\"...\",\"target\":\"...\",\"type\":\"DEPENDS_ON\",\"description\":\"说明关系含义\",\"evidence\":\"原文证据\",\"confidence\":0.85}]}。",
			},
			{
				"role":    "user",
				"content": content,
			},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return graphExtractResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return graphExtractResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return graphExtractResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return graphExtractResult{}, fmt.Errorf("llm graph extractor状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return graphExtractResult{}, err
	}
	if len(decoded.Choices) == 0 {
		return graphExtractResult{}, errors.New("llm graph extractor未返回结果")
	}
	return parseGraphExtractResult(decoded.Choices[0].Message.Content)
}

func parseGraphExtractResult(content string) (graphExtractResult, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		Entities []struct {
			Name        string   `json:"name"`
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Aliases     []string `json:"aliases"`
		} `json:"entities"`
		Relationships []struct {
			Source      string  `json:"source"`
			Target      string  `json:"target"`
			Type        string  `json:"type"`
			Description string  `json:"description"`
			Evidence    string  `json:"evidence"`
			Confidence  float64 `json:"confidence"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return graphExtractResult{}, err
	}
	result := graphExtractResult{}
	entityNames := map[string]bool{}
	for _, entity := range parsed.Entities {
		name := strings.TrimSpace(entity.Name)
		if name == "" {
			continue
		}
		entityNames[canonicalEntityKey(name)] = true
		result.Entities = append(result.Entities, extractedEntity{
			Name:        name,
			Type:        normalizeEntityType(entity.Type, name),
			Description: strings.TrimSpace(entity.Description),
			Aliases:     mergeAliases(entity.Aliases, name),
		})
	}
	for _, relation := range parsed.Relationships {
		source := strings.TrimSpace(relation.Source)
		target := strings.TrimSpace(relation.Target)
		if source == "" || target == "" || !entityNames[canonicalEntityKey(source)] || !entityNames[canonicalEntityKey(target)] {
			continue
		}
		confidence := relation.Confidence
		if confidence <= 0 {
			confidence = 0.72
		}
		result.Relationships = append(result.Relationships, extractedRelationship{
			Source:      source,
			Target:      target,
			Type:        normalizeRelationType(relation.Type),
			Description: strings.TrimSpace(relation.Description),
			Evidence:    strings.TrimSpace(relation.Evidence),
			Confidence:  confidence,
		})
	}
	if len(result.Entities) == 0 {
		return graphExtractResult{}, errors.New("llm graph extractor未抽取实体")
	}
	return result, nil
}

func (ruleGraphExtractor) Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error) {
	_ = ctx
	text := strings.TrimSpace(input.Chunk.Content)
	if text == "" {
		return graphExtractResult{}, nil
	}
	entities := extractGraphEntities(text)
	entityByCanonical := map[string]extractedEntity{}
	for _, entity := range entities {
		canonical := canonicalEntityKey(entity.Name)
		if canonical == "" {
			continue
		}
		if existing, ok := entityByCanonical[canonical]; ok {
			existing.Aliases = mergeAliases(existing.Aliases, append(entity.Aliases, entity.Name)...)
			if existing.Description == "" {
				existing.Description = entity.Description
			}
			entityByCanonical[canonical] = existing
			continue
		}
		entityByCanonical[canonical] = entity
	}
	out := graphExtractResult{Entities: make([]extractedEntity, 0, len(entityByCanonical))}
	for _, entity := range entityByCanonical {
		out.Entities = append(out.Entities, entity)
	}
	sort.Slice(out.Entities, func(i, j int) bool { return out.Entities[i].Name < out.Entities[j].Name })
	out.Relationships = extractGraphRelationships(text, out.Entities)
	return out, nil
}

func filterGraphExtractResult(result graphExtractResult, evidence string) graphExtractResult {
	validEntities := make([]extractedEntity, 0, len(result.Entities))
	validKeys := map[string]bool{}
	validEntityTypeByKey := map[string]string{}
	usedInRelation := map[string]bool{}
	for _, relation := range result.Relationships {
		if strings.TrimSpace(relation.Evidence) == "" && strings.TrimSpace(relation.Description) == "" {
			continue
		}
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey {
			continue
		}
		usedInRelation[sourceKey] = true
		usedInRelation[targetKey] = true
	}
	for _, entity := range result.Entities {
		if !isUsefulGraphEntity(entity, evidence, usedInRelation[canonicalEntityKey(entity.Name)]) {
			continue
		}
		key := canonicalEntityKey(entity.Name)
		if key == "" || validKeys[key] {
			continue
		}
		validKeys[key] = true
		validEntityTypeByKey[key] = normalizeEntityType(entity.Type, entity.Name)
		validEntities = append(validEntities, entity)
	}
	validRelations := make([]extractedRelationship, 0, len(result.Relationships))
	seenRelation := map[string]bool{}
	for _, relation := range result.Relationships {
		relation.Type = normalizeRelationType(relation.Type)
		sourceKey := canonicalEntityKey(relation.Source)
		targetKey := canonicalEntityKey(relation.Target)
		if sourceKey == "" || targetKey == "" || sourceKey == targetKey || !validKeys[sourceKey] || !validKeys[targetKey] {
			continue
		}
		if relation.Type == "RELATED_TO" && relation.Confidence < 0.82 {
			continue
		}
		if !relationAllowedByEntityTypes(relation.Type, validEntityTypeByKey[sourceKey], validEntityTypeByKey[targetKey]) {
			continue
		}
		if !isUsefulGraphRelation(relation, evidence) {
			continue
		}
		key := sourceKey + "->" + targetKey + ":" + relation.Type + ":" + canonicalEntityKey(relation.Evidence)
		if seenRelation[key] {
			continue
		}
		seenRelation[key] = true
		validRelations = append(validRelations, relation)
	}
	if !graphExtractResultQualified(validEntities, validRelations) {
		return graphExtractResult{}
	}
	validRelations = capGraphRelations(validRelations, 6)
	relatedKeys := map[string]bool{}
	for _, relation := range validRelations {
		relatedKeys[canonicalEntityKey(relation.Source)] = true
		relatedKeys[canonicalEntityKey(relation.Target)] = true
	}
	connectedEntities := make([]extractedEntity, 0, len(validEntities))
	for _, entity := range validEntities {
		if relatedKeys[canonicalEntityKey(entity.Name)] {
			connectedEntities = append(connectedEntities, entity)
		}
	}
	if len(connectedEntities) > 8 {
		connectedEntities = connectedEntities[:8]
	}
	return graphExtractResult{Entities: connectedEntities, Relationships: validRelations}
}

func graphExtractResultQualified(entities []extractedEntity, relations []extractedRelationship) bool {
	if len(entities) < 2 || len(relations) == 0 {
		return false
	}
	strongRelations := 0
	semanticRelations := 0
	for _, relation := range relations {
		if normalizeRelationType(relation.Type) != "RELATED_TO" {
			strongRelations++
			continue
		}
		if isHighQualitySemanticRelation(relation) {
			semanticRelations++
		}
	}
	return strongRelations > 0 || semanticRelations > 0
}

func isHighQualitySemanticRelation(relation extractedRelationship) bool {
	if normalizeRelationType(relation.Type) != "RELATED_TO" {
		return false
	}
	if relation.Confidence < 0.9 {
		return false
	}
	text := strings.TrimSpace(relation.Description + " " + relation.Evidence)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	weakPhrases := []string{"同段出现", "同时出现", "一起出现", "相关", "提到了", "mentioned", "co-occur", "cooccur"}
	for _, phrase := range weakPhrases {
		if strings.Contains(lower, phrase) || strings.Contains(text, phrase) {
			return false
		}
	}
	semanticPhrases := []string{"包含", "组成", "属于", "用于说明", "用于解释", "是", "定义", "体现", "支撑", "描述", "面向", "覆盖", "适用于", "组成部分", "核心内容", "关键维度"}
	for _, phrase := range semanticPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return len([]rune(text)) >= 18
}

func capGraphRelations(relations []extractedRelationship, limit int) []extractedRelationship {
	if limit <= 0 || len(relations) <= limit {
		return relations
	}
	sort.SliceStable(relations, func(i, j int) bool {
		leftRelated := normalizeRelationType(relations[i].Type) == "RELATED_TO"
		rightRelated := normalizeRelationType(relations[j].Type) == "RELATED_TO"
		if leftRelated != rightRelated {
			return !leftRelated
		}
		return relations[i].Confidence > relations[j].Confidence
	})
	return relations[:limit]
}

func isUsefulGraphRelation(relation extractedRelationship, evidence string) bool {
	if relation.Source == "" || relation.Target == "" {
		return false
	}
	if relation.Confidence > 0 && relation.Confidence < 0.62 {
		return false
	}
	relationEvidence := strings.TrimSpace(relation.Evidence)
	relationDescription := strings.TrimSpace(relation.Description)
	if relationEvidence == "" && relationDescription == "" {
		return false
	}
	sourceKey := canonicalEntityKey(relation.Source)
	targetKey := canonicalEntityKey(relation.Target)
	evidenceText := strings.TrimSpace(relationEvidence + " " + relationDescription + " " + evidence)
	evidenceLower := strings.ToLower(evidenceText)
	if evidenceLower != "" {
		compactEvidence := canonicalEntityKey(evidenceLower)
		sourceMentioned := strings.Contains(evidenceLower, strings.ToLower(relation.Source)) || strings.Contains(compactEvidence, sourceKey)
		targetMentioned := strings.Contains(evidenceLower, strings.ToLower(relation.Target)) || strings.Contains(compactEvidence, targetKey)
		if !sourceMentioned && !targetMentioned {
			return false
		}
	}
	return true
}

func isUsefulGraphEntity(entity extractedEntity, evidence string, usedInRelation bool) bool {
	name := strings.TrimSpace(entity.Name)
	if name == "" {
		return false
	}
	key := canonicalEntityKey(name)
	if len([]rune(key)) < 2 {
		return false
	}
	lower := strings.ToLower(name)
	noise := []string{
		"product image file", "image file", "screenshot", "unknown", "none", "n/a", "todo",
		"正文", "标题", "内容", "图片", "文件", "截图", "页面", "文本", "示例", "未知实体", "系统", "用户", "数据", "信息", "说明", "文档",
		"在线用户", "当前用户", "普通用户", "触发用户", "用户身份", "文件内容", "文本内容", "页面内容", "图片内容",
		"会话摘要", "会话主题", "聊天摘要", "聊天主题",
		"目录", "章节", "步骤", "材料", "资料", "个人经历", "工作经历", "项目经历", "教育背景", "个人技能", "求职目标", "自我评价", "联系方式",
	}
	for _, item := range noise {
		if lower == item || strings.EqualFold(name, item) || name == item {
			return false
		}
	}
	if isGraphStopEntityName(name) {
		return false
	}
	if isWeakChineseGraphEntity(name) {
		return false
	}
	if isGenericDocumentSectionEntity(name) {
		return false
	}
	if regexp.MustCompile(`(?i)^[a-f0-9]{16,}\.(png|jpg|jpeg|webp|gif|bmp|tif|tiff)$`).MatchString(lower) {
		return false
	}
	if regexp.MustCompile(`(?i)^p?\d{1,4}[-_]\d{1,4}$`).MatchString(lower) {
		return false
	}
	if isGraphFieldOrStatusName(name) {
		return false
	}
	if normalizeEntityType(entity.Type, name) == "Concept" && len([]rune(name)) <= 2 {
		return false
	}
	if !usedInRelation && isRuleExtractedGraphEntity(entity) {
		return false
	}
	if strings.TrimSpace(evidence) != "" && !usedInRelation && !entityMentionedByEvidence(entity, evidence) {
		return false
	}
	return true
}

func isGenericDocumentSectionEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	switch trimmed {
	case "个人经历", "工作经历", "实习经历", "项目经历", "教育背景", "专业技能", "个人技能", "求职目标", "自我评价", "联系方式", "获奖经历", "校园经历", "目录", "章节", "步骤":
		return true
	default:
		return false
	}
}

func isRuleExtractedGraphEntity(entity extractedEntity) bool {
	return strings.Contains(strings.TrimSpace(entity.Description), "从知识分块中识别")
}

func entityMentionedByEvidence(entity extractedEntity, evidence string) bool {
	evidenceLower := strings.ToLower(evidence)
	if strings.Contains(evidenceLower, strings.ToLower(strings.TrimSpace(entity.Name))) {
		return true
	}
	for _, alias := range entity.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" && strings.Contains(evidenceLower, strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func isGraphFieldOrStatusName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	fieldNames := map[string]bool{
		"id": true, "user_id": true, "sender_id": true, "owner_id": true, "group_id": true, "conversation_id": true,
		"message_id": true, "msg_id": true, "reply_to_id": true, "event_id": true, "source_event_id": true,
		"client_msg_id": true, "trace_id": true, "agent_trace_id": true, "agent_user_id": true, "bot_id": true,
		"created_at": true, "updated_at": true, "deleted_at": true, "status": true, "type": true,
		"pending": true, "processing": true, "completed": true, "failed": true, "success": true, "error": true,
	}
	if fieldNames[lower] {
		return true
	}
	if regexp.MustCompile(`(?i)^(.*_)?(id|ids|status|type|time|at)$`).MatchString(lower) && !strings.HasSuffix(lower, "_service") {
		return true
	}
	return false
}

func isGraphStopEntityName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http") || strings.HasPrefix(lower, "data:") {
		return true
	}
	if regexp.MustCompile(`(?i)\.(png|jpg|jpeg|webp|gif|bmp|tif|tiff|pdf|docx?|pptx?|md|markdown|txt|go|js|ts|tsx|jsx|py|java|c|cc|cpp|h|hpp|rs|sql|json|ya?ml|toml|xml|html|css|scss|sh|bat|ps1)$`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`(?i)^[a-z]:[\\/]|[\\/]{2,}`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`^\d+(\.\d+)*$`).MatchString(lower) {
		return true
	}
	if regexp.MustCompile(`^[第]?\d+[章节页课]$`).MatchString(trimmed) {
		return true
	}
	return false
}

func isWeakChineseGraphEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	weakSuffixes := []string{"内容", "文本", "页面", "截图", "图片", "文件", "数据", "信息", "说明", "示例", "用户"}
	for _, suffix := range weakSuffixes {
		if strings.HasSuffix(trimmed, suffix) {
			switch trimmed {
			case "用户服务", "用户系统", "用户模块", "用户画像", "用户记忆":
				return false
			default:
				return true
			}
		}
	}
	return false
}

func extractGraphEntities(text string) []extractedEntity {
	seen := map[string]bool{}
	var out []extractedEntity
	add := func(name string, aliases ...string) {
		name = strings.Trim(strings.TrimSpace(name), "，。,.；;:：()（）[]【】")
		if name == "" || isGraphStopEntityName(name) || hasGraphRelationPrefixNoise(name) {
			return
		}
		if !isStrongRuleGraphEntity(name) {
			return
		}
		canonical := canonicalEntityKey(name)
		if canonical == "" || seen[canonical] {
			if canonical != "" && len(aliases) > 0 {
				for i := range out {
					if canonicalEntityKey(out[i].Name) == canonical {
						out[i].Aliases = mergeAliases(out[i].Aliases, aliases...)
						return
					}
				}
			}
			return
		}
		seen[canonical] = true
		out = append(out, extractedEntity{
			Name:        name,
			Type:        inferEntityType(name),
			Description: "从知识分块中识别的 " + inferEntityType(name) + " 实体",
			Aliases:     mergeAliases([]string{name}, aliases...),
		})
	}
	identifierRe := regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[-_.][A-Za-z0-9]+)+|[a-z][a-z0-9]+(?:_[a-z0-9]+)+`)
	for _, match := range identifierRe.FindAllString(text, -1) {
		add(match)
	}
	if strings.Contains(strings.ToLower(text), "msg core service") {
		add("msg core service", "msg-core-service", "消息核心服务")
	}
	aliasRe := regexp.MustCompile(`([A-Za-z][A-Za-z0-9 _.-]{2,60})\s*(?:也叫|又称|即|alias)\s*([\p{Han}A-Za-z0-9 _.-]{2,40})`)
	for _, match := range aliasRe.FindAllStringSubmatch(text, -1) {
		if len(match) == 3 {
			add(match[1], match[2])
			add(match[2], match[1])
		}
	}
	chineseRe := regexp.MustCompile(`[\p{Han}]{2,18}(?:服务|模块|系统|平台|助手|数据库|数据表|事件|接口|知识库|知识图谱|图谱)`)
	for _, match := range chineseRe.FindAllString(text, -1) {
		add(match)
	}
	return out
}

func isStrongRuleGraphEntity(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if lower == "msg core service" {
		return true
	}
	if strings.Contains(lower, "-service") ||
		strings.Contains(lower, "_service") ||
		strings.Contains(lower, "-gateway") ||
		strings.Contains(lower, "_gateway") ||
		strings.Contains(lower, "-manager") ||
		strings.Contains(lower, "_manager") ||
		strings.Contains(lower, "-runtime") ||
		strings.Contains(lower, "_runtime") ||
		strings.Contains(lower, ".events") ||
		strings.Contains(lower, ".event") ||
		strings.Contains(lower, "_events") ||
		strings.Contains(lower, "_event") ||
		strings.Contains(lower, "_records") ||
		strings.Contains(lower, "_outbox") ||
		strings.Contains(lower, "_table") ||
		strings.Contains(lower, "_tasks") ||
		strings.Contains(lower, "_messages") ||
		strings.Contains(lower, "_settings") ||
		strings.Contains(lower, "_memory") ||
		strings.Contains(lower, "_chunks") ||
		strings.Contains(lower, "_entities") ||
		strings.Contains(lower, "_relationships") {
		return true
	}
	if strings.HasPrefix(lower, "/api/") || strings.HasPrefix(lower, "api/") {
		return true
	}
	if regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(Service|Gateway|Manager|Runtime|Controller|Repository|Client)$`).MatchString(trimmed) {
		return true
	}
	if regexp.MustCompile(`^[a-z][a-z0-9]+(?:_[a-z0-9]+){1,}$`).MatchString(lower) {
		for _, suffix := range []string{"service", "gateway", "manager", "runtime", "events", "event", "records", "outbox", "table", "tasks", "messages", "settings", "memory", "chunks", "entities", "relationships"} {
			if strings.HasSuffix(lower, "_"+suffix) || strings.Contains(lower, "_"+suffix+"_") {
				return true
			}
		}
	}
	if regexp.MustCompile(`[\p{Han}]{2,18}(服务|模块|系统|平台|助手|数据库|数据表|事件|接口|知识库|知识图谱|图谱)$`).MatchString(trimmed) {
		return !isWeakChineseGraphEntity(trimmed)
	}
	return false
}

func hasGraphRelationPrefixNoise(name string) bool {
	trimmed := strings.TrimSpace(name)
	for _, prefix := range []string{"也叫", "又称", "负责把", "用于", "应该成为", "不应该成为"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func extractGraphRelationships(text string, entities []extractedEntity) []extractedRelationship {
	if len(entities) < 2 {
		return nil
	}
	var out []extractedRelationship
	seen := map[string]bool{}
	entityTypeByKey := map[string]string{}
	for _, entity := range entities {
		entityTypeByKey[canonicalEntityKey(entity.Name)] = normalizeEntityType(entity.Type, entity.Name)
	}
	for _, sentence := range splitGraphSentences(text) {
		mentions := entityMentionsInText(sentence, entities)
		if len(mentions) < 2 {
			continue
		}
		triggers := relationTriggersInSentence(sentence)
		if len(triggers) == 0 {
			continue
		}
		defaultSubject := mentions[0].Name
		for _, trigger := range triggers {
			source, target := relationDirectionAt(sentence, mentions, trigger, defaultSubject)
			if source == "" || target == "" || canonicalEntityKey(source) == canonicalEntityKey(target) {
				continue
			}
			sourceKey := canonicalEntityKey(source)
			targetKey := canonicalEntityKey(target)
			if !relationAllowedByEntityTypes(trigger.Type, entityTypeByKey[sourceKey], entityTypeByKey[targetKey]) {
				continue
			}
			key := sourceKey + "->" + targetKey + ":" + trigger.Type
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, extractedRelationship{
				Source:      source,
				Target:      target,
				Type:        trigger.Type,
				Description: relationDescription(source, target, trigger.Type),
				Evidence:    sentence,
				Confidence:  0.84,
			})
		}
	}
	return out
}

func (s *ragServiceImpl) rebuildGraphCommunities(ctx context.Context, ownerID int64) {
	if s == nil || s.repo == nil || ownerID <= 0 {
		return
	}
	entities, relations, err := s.repo.ListOwnerGraph(ctx, ownerID)
	if err != nil || len(entities) == 0 {
		return
	}
	assignments := leidenLikeCommunities(entities, relations)
	communities := s.buildCommunityModels(ctx, ownerID, entities, relations, assignments)
	entityCommunity := map[int64]int64{}
	idx := 0
	for _, community := range communities {
		for _, entityID := range communityEntityIDs(assignments, idx) {
			entityCommunity[entityID] = community.ID
		}
		idx++
	}
	_ = s.repo.ReplaceOwnerCommunities(ctx, ownerID, communities, entityCommunity)
}

func leidenLikeCommunities(entities []model.Entity, relations []model.Relation) map[int64]int {
	if len(entities) == 0 {
		return map[int64]int{}
	}
	nodeSet := map[int64]bool{}
	for idx, entity := range entities {
		nodeSet[entity.ID] = true
		_ = idx
	}
	graph := weightedAdjacency(nodeSet, relations)
	assignments := strongEdgeComponents(entities, relations, graph)
	for iter := 0; iter < 8; iter++ {
		moved := false
		ordered := append([]model.Entity(nil), entities...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Score > ordered[j].Score })
		for _, entity := range ordered {
			current := assignments[entity.ID]
			bestCommunity := current
			bestGain := 0.0
			neighborCommunityWeight := map[int]float64{}
			for neighborID, weight := range graph[entity.ID] {
				neighborCommunityWeight[assignments[neighborID]] += weight
			}
			for community, weight := range neighborCommunityWeight {
				if community == current {
					continue
				}
				gain := weight - 0.05*communitySize(assignments, community)
				if gain > bestGain {
					bestGain = gain
					bestCommunity = community
				}
			}
			if bestCommunity != current && bestGain > 0 {
				assignments[entity.ID] = bestCommunity
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	return compactCommunityIDs(refineDisconnectedCommunities(assignments, graph))
}

func strongEdgeComponents(entities []model.Entity, relations []model.Relation, graph map[int64]map[int64]float64) map[int64]int {
	parent := map[int64]int64{}
	for _, entity := range entities {
		parent[entity.ID] = entity.ID
	}
	var find func(int64) int64
	find = func(id int64) int64 {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	union := func(a, b int64) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	threshold := strongEdgeThreshold(relations)
	for sourceID, neighbors := range graph {
		for targetID, weight := range neighbors {
			if sourceID < targetID && weight >= threshold {
				union(sourceID, targetID)
			}
		}
	}
	roots := map[int64]int{}
	next := 0
	assignments := map[int64]int{}
	for _, entity := range entities {
		root := find(entity.ID)
		if _, ok := roots[root]; !ok {
			roots[root] = next
			next++
		}
		assignments[entity.ID] = roots[root]
	}
	return assignments
}

func strongEdgeThreshold(relations []model.Relation) float64 {
	if len(relations) == 0 {
		return 0.15
	}
	total := 0.0
	count := 0
	for _, relation := range relations {
		weight := relation.Weight
		if weight <= 0 {
			weight = relation.Confidence
		}
		if weight <= 0 {
			weight = 0.5
		}
		total += weight
		count++
	}
	avg := total / float64(count)
	threshold := avg * 0.25
	if threshold < 0.15 {
		return 0.15
	}
	if threshold > 0.6 {
		return 0.6
	}
	return threshold
}

func weightedAdjacency(nodeSet map[int64]bool, relations []model.Relation) map[int64]map[int64]float64 {
	graph := map[int64]map[int64]float64{}
	for id := range nodeSet {
		graph[id] = map[int64]float64{}
	}
	for _, relation := range relations {
		if !nodeSet[relation.SourceID] || !nodeSet[relation.TargetID] || relation.SourceID == relation.TargetID {
			continue
		}
		weight := relation.Weight
		if weight <= 0 {
			weight = relation.Confidence
		}
		if weight <= 0 {
			weight = 0.5
		}
		graph[relation.SourceID][relation.TargetID] += weight
		graph[relation.TargetID][relation.SourceID] += weight
	}
	return graph
}

func communitySize(assignments map[int64]int, community int) float64 {
	count := 0
	for _, value := range assignments {
		if value == community {
			count++
		}
	}
	return float64(count)
}

func refineDisconnectedCommunities(assignments map[int64]int, graph map[int64]map[int64]float64) map[int64]int {
	nextCommunity := 0
	for _, community := range assignments {
		if community >= nextCommunity {
			nextCommunity = community + 1
		}
	}
	refined := map[int64]int{}
	visited := map[int64]bool{}
	communityNodes := map[int][]int64{}
	for nodeID, community := range assignments {
		communityNodes[community] = append(communityNodes[community], nodeID)
	}
	for community, nodes := range communityNodes {
		nodeSet := map[int64]bool{}
		for _, nodeID := range nodes {
			nodeSet[nodeID] = true
		}
		componentIndex := 0
		for _, nodeID := range nodes {
			if visited[nodeID] {
				continue
			}
			targetCommunity := community
			if componentIndex > 0 {
				targetCommunity = nextCommunity
				nextCommunity++
			}
			componentIndex++
			queue := []int64{nodeID}
			visited[nodeID] = true
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				refined[current] = targetCommunity
				for neighborID := range graph[current] {
					if !nodeSet[neighborID] || visited[neighborID] {
						continue
					}
					visited[neighborID] = true
					queue = append(queue, neighborID)
				}
			}
		}
	}
	return refined
}

func compactCommunityIDs(assignments map[int64]int) map[int64]int {
	seen := map[int]int{}
	next := 0
	out := map[int64]int{}
	ids := make([]int64, 0, len(assignments))
	for id := range assignments {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		old := assignments[id]
		if _, ok := seen[old]; !ok {
			seen[old] = next
			next++
		}
		out[id] = seen[old]
	}
	return out
}

type graphCommunitySummaryInput struct {
	CommunityIndex int
	Entities       []model.Entity
	Relations      []model.Relation
}

type graphCommunitySummary struct {
	Title   string
	Summary string
}

func (s *ragServiceImpl) buildCommunityModels(ctx context.Context, ownerID int64, entities []model.Entity, relations []model.Relation, assignments map[int64]int) []model.Community {
	grouped := map[int][]model.Entity{}
	for _, entity := range entities {
		grouped[assignments[entity.ID]] = append(grouped[assignments[entity.ID]], entity)
	}
	indexes := make([]int, 0, len(grouped))
	for index := range grouped {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	summarizer := s.graphSummarizer
	if summarizer == nil {
		summarizer = ruleGraphCommunitySummarizer{}
	}
	out := make([]model.Community, 0, len(indexes))
	for _, index := range indexes {
		items := grouped[index]
		sort.Slice(items, func(i, j int) bool {
			if items[i].Score == items[j].Score {
				return items[i].Name < items[j].Name
			}
			return items[i].Score > items[j].Score
		})
		communityRelations := relationsForCommunity(relations, assignments, index)
		summary, err := summarizer.Summarize(ctx, graphCommunitySummaryInput{CommunityIndex: index, Entities: items, Relations: communityRelations})
		if err != nil || strings.TrimSpace(summary.Summary) == "" {
			summary, _ = ruleGraphCommunitySummarizer{}.Summarize(ctx, graphCommunitySummaryInput{CommunityIndex: index, Entities: items, Relations: communityRelations})
		}
		communityID, _ := idgen.NextID()
		if communityID == 0 {
			communityID = int64(index + 1)
		}
		out = append(out, model.Community{
			ID:              communityID,
			OwnerID:         ownerID,
			Name:            defaultString(summary.Title, communityTitle(items)),
			Summary:         summary.Summary,
			KeyEntitiesJSON: encodeCommunityEntityNames(items),
			Level:           1,
		})
	}
	return out
}

func communityEntityIDs(assignments map[int64]int, community int) []int64 {
	var ids []int64
	for id, value := range assignments {
		if value == community {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func relationsForCommunity(relations []model.Relation, assignments map[int64]int, community int) []model.Relation {
	out := make([]model.Relation, 0)
	for _, relation := range relations {
		if assignments[relation.SourceID] == community && assignments[relation.TargetID] == community {
			out = append(out, relation)
		}
	}
	return out
}

func encodeCommunityEntityNames(entities []model.Entity) string {
	names := make([]string, 0, minInt(len(entities), 12))
	for i, entity := range entities {
		if i >= 12 {
			break
		}
		names = append(names, entity.Name)
	}
	data, _ := json.Marshal(names)
	return string(data)
}

func communityTitle(entities []model.Entity) string {
	if len(entities) == 0 {
		return "知识社区"
	}
	typeCount := map[string]int{}
	for _, entity := range entities {
		typeCount[entity.Type]++
	}
	bestType := "Concept"
	bestCount := -1
	for typ, count := range typeCount {
		if count > bestCount {
			bestType = typ
			bestCount = count
		}
	}
	return fmt.Sprintf("%s 社区", bestType)
}

type ruleGraphCommunitySummarizer struct{}

func (ruleGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	_ = ctx
	names := make([]string, 0, minInt(len(input.Entities), 8))
	for i, entity := range input.Entities {
		if i >= 8 {
			break
		}
		names = append(names, entity.Name)
	}
	title := communityTitle(input.Entities)
	summary := fmt.Sprintf("%s包含 %s 等实体，共 %d 个实体、%d 条内部关系。该社区用于 GraphRAG 查询时提供局部结构和证据入口。", title, strings.Join(names, "、"), len(input.Entities), len(input.Relations))
	return graphCommunitySummary{Title: title, Summary: summary}, nil
}

type fallbackGraphCommunitySummarizer struct {
	primary  graphCommunitySummarizer
	fallback graphCommunitySummarizer
}

func (s fallbackGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	if s.primary != nil {
		result, err := s.primary.Summarize(ctx, input)
		if err == nil && strings.TrimSpace(result.Summary) != "" {
			return result, nil
		}
	}
	if s.fallback == nil {
		return graphCommunitySummary{}, nil
	}
	return s.fallback.Summarize(ctx, input)
}

type LLMGraphCommunitySummarizer struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewLLMGraphCommunitySummarizer(apiKey, baseURL, model string) *LLMGraphCommunitySummarizer {
	return &LLMGraphCommunitySummarizer{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:   defaultString(model, "glm-4-flash"),
		Client:  &http.Client{Timeout: 16 * time.Second},
	}
}

func (s *LLMGraphCommunitySummarizer) Summarize(ctx context.Context, input graphCommunitySummaryInput) (graphCommunitySummary, error) {
	if s == nil || s.APIKey == "" || s.BaseURL == "" {
		return graphCommunitySummary{}, errors.New("llm graph community summarizer未配置")
	}
	payload := map[string]interface{}{
		"model": s.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是GraphRAG community summarizer。只输出JSON，不要解释。根据社区实体和关系生成短标题和摘要。JSON字段: title, summary。摘要要说明该社区表达的系统链路、关键实体和关系用途。"},
			{"role": "user", "content": buildCommunitySummaryPrompt(input)},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return graphCommunitySummary{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return graphCommunitySummary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return graphCommunitySummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return graphCommunitySummary{}, fmt.Errorf("llm graph community summarizer状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return graphCommunitySummary{}, err
	}
	if len(decoded.Choices) == 0 {
		return graphCommunitySummary{}, errors.New("llm graph community summarizer未返回结果")
	}
	return parseCommunitySummary(decoded.Choices[0].Message.Content)
}

func buildCommunitySummaryPrompt(input graphCommunitySummaryInput) string {
	var b strings.Builder
	b.WriteString("实体:\n")
	for _, entity := range input.Entities {
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", entity.Name, entity.Type, truncate(entity.Summary, 180)))
	}
	b.WriteString("\n关系:\n")
	for i, relation := range input.Relations {
		if i >= 30 {
			break
		}
		b.WriteString(fmt.Sprintf("- %d -> %d %s: %s\n", relation.SourceID, relation.TargetID, relation.Relation, truncate(relation.Description, 180)))
	}
	return b.String()
}

func parseCommunitySummary(content string) (graphCommunitySummary, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return graphCommunitySummary{}, err
	}
	if strings.TrimSpace(parsed.Summary) == "" {
		return graphCommunitySummary{}, errors.New("社区摘要为空")
	}
	return graphCommunitySummary{Title: strings.TrimSpace(parsed.Title), Summary: strings.TrimSpace(parsed.Summary)}, nil
}

func splitGraphSentences(text string) []string {
	fields := regexp.MustCompile(`[。！？!?；;\n]+`).Split(text, -1)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func entityNamesInText(sentence string, entities []extractedEntity) []string {
	mentions := entityMentionsInText(sentence, entities)
	names := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		names = append(names, mention.Name)
	}
	return names
}

func entityMentionsInText(sentence string, entities []extractedEntity) []graphEntityMention {
	lowerSentence := strings.ToLower(sentence)
	var mentions []graphEntityMention
	seen := map[string]bool{}
	for _, entity := range entities {
		candidates := append([]string{entity.Name}, entity.Aliases...)
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			index := strings.Index(lowerSentence, strings.ToLower(candidate))
			if index >= 0 {
				key := canonicalEntityKey(entity.Name)
				if !seen[key] {
					seen[key] = true
					mentions = append(mentions, graphEntityMention{Name: entity.Name, Start: index, End: index + len(candidate)})
				}
				break
			}
		}
	}
	sort.SliceStable(mentions, func(i, j int) bool {
		return mentions[i].Start < mentions[j].Start
	})
	return mentions
}

func relationTypeFromSentence(sentence string) string {
	lower := strings.ToLower(sentence)
	switch {
	case strings.Contains(sentence, "写入") || strings.Contains(sentence, "写") || strings.Contains(lower, "write"):
		return "WRITES"
	case strings.Contains(sentence, "读取") || strings.Contains(lower, "read"):
		return "READS"
	case strings.Contains(sentence, "消费") || strings.Contains(lower, "consume"):
		return "CONSUMES"
	case strings.Contains(sentence, "发布") || strings.Contains(lower, "publish"):
		return "PUBLISHES"
	case strings.Contains(sentence, "调用") || strings.Contains(lower, "call"):
		return "CALLS"
	case strings.Contains(sentence, "存储") || strings.Contains(sentence, "保存") || strings.Contains(lower, "store"):
		return "STORES"
	case strings.Contains(sentence, "依赖") || strings.Contains(lower, "depend"):
		return "DEPENDS_ON"
	case strings.Contains(sentence, "配置") || strings.Contains(lower, "config"):
		return "CONFIGURES"
	case strings.Contains(sentence, "触发") || strings.Contains(lower, "trigger"):
		return "TRIGGERS"
	case strings.Contains(sentence, "拥有") || strings.Contains(sentence, "负责") || strings.Contains(lower, "own"):
		return "OWNS"
	default:
		return ""
	}
}

func relationTriggersInSentence(sentence string) []graphRelationTrigger {
	lower := strings.ToLower(sentence)
	relationTypes := []string{"WRITES", "READS", "CONSUMES", "PUBLISHES", "CALLS", "STORES", "DEPENDS_ON", "CONFIGURES", "TRIGGERS", "OWNS"}
	triggers := make([]graphRelationTrigger, 0, 2)
	for _, relationType := range relationTypes {
		for _, term := range relationTriggerTerms(relationType) {
			needle := strings.ToLower(term)
			searchFrom := 0
			for {
				idx := strings.Index(lower[searchFrom:], needle)
				if idx < 0 {
					break
				}
				absolute := searchFrom + idx
				if isASCIIWord(needle) && !isRelationTermBoundary(lower, absolute, absolute+len(needle)) {
					searchFrom = absolute + len(needle)
					continue
				}
				triggers = append(triggers, graphRelationTrigger{Type: relationType, Index: absolute, End: absolute + len(needle)})
				searchFrom = absolute + len(needle)
			}
		}
	}
	sort.SliceStable(triggers, func(i, j int) bool {
		if triggers[i].Index == triggers[j].Index {
			return triggers[i].End > triggers[j].End
		}
		return triggers[i].Index < triggers[j].Index
	})
	return dedupeRelationTriggers(triggers)
}

func dedupeRelationTriggers(triggers []graphRelationTrigger) []graphRelationTrigger {
	out := make([]graphRelationTrigger, 0, len(triggers))
	for _, trigger := range triggers {
		if len(out) > 0 {
			prev := out[len(out)-1]
			if trigger.Index >= prev.Index && trigger.End <= prev.End {
				continue
			}
			if trigger.Index == prev.Index && trigger.End > prev.End {
				out[len(out)-1] = trigger
				continue
			}
		}
		out = append(out, trigger)
	}
	return out
}

func isASCIIWord(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func isRelationTermBoundary(text string, start, end int) bool {
	if start > 0 {
		prev := rune(text[start-1])
		if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') || prev == '_' {
			return false
		}
	}
	if end < len(text) {
		next := rune(text[end])
		if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	return true
}

func relationDirectionAt(sentence string, mentions []graphEntityMention, trigger graphRelationTrigger, defaultSubject string) (string, string) {
	if len(mentions) < 2 {
		return "", ""
	}
	before := -1
	after := -1
	for i, mention := range mentions {
		if mention.End <= trigger.Index {
			before = i
		}
		if mention.Start >= trigger.Index && after == -1 {
			after = i
		}
	}
	if before < 0 || after < 0 || before == after {
		return "", ""
	}
	if isPassiveRelation(sentence, mentions[before], trigger) {
		return mentions[after].Name, mentions[before].Name
	}
	if shouldUseDefaultSubject(sentence, mentions[before], trigger, defaultSubject) {
		return defaultSubject, mentions[after].Name
	}
	return mentions[before].Name, mentions[after].Name
}

func isPassiveRelation(sentence string, target graphEntityMention, trigger graphRelationTrigger) bool {
	if target.End < 0 || trigger.Index <= target.End || trigger.Index > len(sentence) {
		return false
	}
	between := sentence[target.End:trigger.Index]
	return strings.Contains(between, "被") || strings.Contains(strings.ToLower(between), " by ")
}

func shouldUseDefaultSubject(sentence string, before graphEntityMention, trigger graphRelationTrigger, defaultSubject string) bool {
	if defaultSubject == "" || canonicalEntityKey(before.Name) == canonicalEntityKey(defaultSubject) {
		return false
	}
	if before.End < 0 || trigger.Index <= before.End || trigger.Index > len(sentence) {
		return false
	}
	between := strings.TrimSpace(sentence[before.End:trigger.Index])
	if between == "" {
		return false
	}
	return regexp.MustCompile(`^[，,、\s]*(并|并且|然后|同时)?[，,、\s]*$`).MatchString(between)
}

func relationDirection(sentence string, mentions []graphEntityMention, relationType string) (string, string) {
	if len(mentions) < 2 {
		return "", ""
	}
	verbIndex := relationVerbIndex(sentence, relationType)
	if verbIndex >= 0 {
		before := -1
		after := -1
		for i, mention := range mentions {
			if mention.End <= verbIndex {
				before = i
			}
			if mention.Start >= verbIndex && after == -1 {
				after = i
			}
		}
		if before >= 0 && after >= 0 && before != after {
			return mentions[before].Name, mentions[after].Name
		}
	}
	return mentions[0].Name, mentions[1].Name
}

func relationVerbIndex(sentence, relationType string) int {
	terms := relationTriggerTerms(relationType)
	lower := strings.ToLower(sentence)
	best := -1
	for _, term := range terms {
		idx := strings.Index(lower, strings.ToLower(term))
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	return best
}

func relationTriggerTerms(relationType string) []string {
	switch relationType {
	case "WRITES":
		return []string{"写入", "写", "write", "writes"}
	case "READS":
		return []string{"读取", "read", "reads"}
	case "CONSUMES":
		return []string{"消费", "consume", "consumes"}
	case "PUBLISHES":
		return []string{"发布", "publish", "publishes"}
	case "CALLS":
		return []string{"调用", "call", "calls"}
	case "STORES":
		return []string{"存储", "保存", "store", "stores"}
	case "DEPENDS_ON":
		return []string{"依赖", "depend", "depends"}
	case "CONFIGURES":
		return []string{"配置", "config", "configures"}
	case "TRIGGERS":
		return []string{"触发", "trigger", "triggers"}
	case "OWNS":
		return []string{"拥有", "负责", "own", "owns"}
	default:
		return nil
	}
}

func relationDescription(source, target, relationType string) string {
	verbs := map[string]string{
		"CALLS":      "调用",
		"PUBLISHES":  "发布到",
		"CONSUMES":   "消费",
		"STORES":     "存储到",
		"OWNS":       "负责",
		"DEPENDS_ON": "依赖",
		"CONFIGURES": "配置",
		"TRIGGERS":   "触发",
		"READS":      "读取",
		"WRITES":     "写入",
		"RELATED_TO": "关联",
	}
	verb := defaultString(verbs[relationType], "关联")
	return source + " " + verb + " " + target
}

func relationAllowedByEntityTypes(relationType, sourceType, targetType string) bool {
	relationType = normalizeRelationType(relationType)
	sourceType = normalizeGraphEntityTypeForPolicy(sourceType)
	targetType = normalizeGraphEntityTypeForPolicy(targetType)
	if sourceType == "" || targetType == "" {
		return true
	}
	switch relationType {
	case "CALLS":
		return isExecutableGraphEntityType(sourceType) && isCallableGraphEntityType(targetType)
	case "PUBLISHES":
		return isExecutableGraphEntityType(sourceType) && targetType == "EventTopic"
	case "CONSUMES":
		return isExecutableGraphEntityType(sourceType) && targetType == "EventTopic"
	case "WRITES", "READS", "STORES":
		return isExecutableGraphEntityType(sourceType) && isStorageGraphEntityType(targetType)
	case "CONFIGURES":
		return isExecutableGraphEntityType(sourceType) && targetType != "Person"
	case "TRIGGERS":
		return sourceType != "DatabaseTable" && targetType != "DatabaseTable"
	case "DEPENDS_ON":
		return sourceType != "DatabaseTable" || targetType != "Person"
	case "OWNS":
		return sourceType != "DatabaseTable" && targetType != "EventTopic"
	case "RELATED_TO":
		return true
	default:
		return true
	}
}

func normalizeGraphEntityTypeForPolicy(entityType string) string {
	switch strings.TrimSpace(entityType) {
	case "Service", "DatabaseTable", "EventTopic", "API", "Module", "Concept", "Person", "Organization", "Product":
		return strings.TrimSpace(entityType)
	default:
		return ""
	}
}

func isExecutableGraphEntityType(entityType string) bool {
	switch entityType {
	case "Service", "API", "Module", "Product":
		return true
	default:
		return false
	}
}

func isCallableGraphEntityType(entityType string) bool {
	switch entityType {
	case "Service", "API", "Module":
		return true
	default:
		return false
	}
}

func isStorageGraphEntityType(entityType string) bool {
	switch entityType {
	case "DatabaseTable", "Concept", "Product":
		return true
	default:
		return false
	}
}

func canonicalEntityKey(name string) string {
	alias := strings.TrimSpace(strings.ToLower(name))
	switch alias {
	case "消息核心服务", "messageservice", "message service", "msg core service":
		return "msgcoreservice"
	}
	var b strings.Builder
	for _, r := range alias {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeEntityType(entityType, name string) string {
	entityType = strings.TrimSpace(entityType)
	valid := map[string]bool{
		"Service": true, "DatabaseTable": true, "EventTopic": true, "API": true,
		"Module": true, "Concept": true, "Person": true, "Organization": true, "Product": true,
	}
	if valid[entityType] {
		return entityType
	}
	return inferEntityType(name)
}

func normalizeRelationType(relationType string) string {
	relationType = strings.ToUpper(strings.TrimSpace(relationType))
	relationType = strings.ReplaceAll(relationType, "-", "_")
	relationType = strings.ReplaceAll(relationType, " ", "_")
	valid := map[string]bool{
		"CALLS": true, "PUBLISHES": true, "CONSUMES": true, "STORES": true,
		"OWNS": true, "DEPENDS_ON": true, "CONFIGURES": true, "TRIGGERS": true,
		"READS": true, "WRITES": true, "RELATED_TO": true,
	}
	if valid[relationType] {
		return relationType
	}
	return "RELATED_TO"
}

func encodeAliases(aliases []string) string {
	aliases = mergeAliases(nil, aliases...)
	data, err := json.Marshal(aliases)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeAliases(raw string) []string {
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		return nil
	}
	return aliases
}

func mergeAliases(base []string, values ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func communityForEntity(ownerID int64, entity model.Entity) *model.Community {
	name := communityName(entity.Name)
	aliases := decodeAliases(entity.AliasesJSON)
	return &model.Community{
		OwnerID:         ownerID,
		Name:            name,
		Summary:         fmt.Sprintf("%s 社区包含 %s 等实体，用于 GraphRAG 查询时提供领域摘要和子图入口。", name, defaultString(entity.Name, "相关")),
		KeyEntitiesJSON: encodeAliases(append(aliases, entity.Name)),
		Level:           1,
	}
}

func (s *ragServiceImpl) externalRouteHint(query string, decision RouterDecision, self *rag.SelfRAGCheckpoints, route, message string) SearchResult {
	_ = s
	self.Retrieve = decision.Retrieve
	self.IsRel = true
	self.IsSup = false
	self.IsUse = true
	self.Note = self.Note + fmt.Sprintf("；Adaptive route=%s complexity=%s sources=%s strategy=%s", decision.Route, decision.Complexity, strings.Join(decision.Sources, ","), decision.Strategy)
	return SearchResult{
		Success:    true,
		Answer:     message + " 用户问题：" + query,
		Route:      route,
		CragAction: "skip_vector",
		SelfCheck:  self,
	}
}

type rankedChunk struct {
	Chunk    model.Chunk
	Document model.Document
	Score    float64
	Reason   string
}

func (s *ragServiceImpl) rerank(ctx context.Context, query string, chunks []rankedChunk, topN int) []rankedChunk {
	if s != nil && s.reranker != nil && len(chunks) > 0 {
		scores, err := s.reranker.Rerank(ctx, query, chunks, topN)
		if err == nil {
			return applyModelRerank(query, chunks, scores, topN)
		}
	}
	reranked := localRerank(query, chunks)
	if topN > 0 && len(reranked) > topN {
		return reranked[:topN]
	}
	return reranked
}

func (s *ragServiceImpl) evaluateCRAG(ctx context.Context, query string, chunks []rankedChunk) CRAGEvaluation {
	evaluator := CRAGEvaluator(RuleCRAGEvaluator{})
	if s != nil && s.cragEvaluator != nil {
		evaluator = s.cragEvaluator
	}
	evaluation, err := evaluator.Evaluate(ctx, CRAGEvaluateInput{Query: query, Chunks: chunks})
	if err != nil {
		evaluation, _ = RuleCRAGEvaluator{}.Evaluate(ctx, CRAGEvaluateInput{Query: query, Chunks: chunks})
		evaluation.Reason = strings.TrimSpace(evaluation.Reason + "；CRAG evaluator降级: " + err.Error())
	}
	evaluation.Label = normalizeCRAGLabel(evaluation.Label)
	if evaluation.Label == "" {
		evaluation.Label = CRAGLabelAmbiguous
	}
	return evaluation
}

func (s *ragServiceImpl) judgeSelfRAG(ctx context.Context, query, answer string, chunks []rankedChunk, crag CRAGEvaluation, route string) SelfRAGJudgement {
	judge := SelfRAGJudge(RuleSelfRAGJudge{})
	if s != nil && s.selfJudge != nil {
		judge = s.selfJudge
	}
	judgement, err := judge.Judge(ctx, SelfRAGJudgeInput{Query: query, Answer: answer, Chunks: chunks, CRAG: crag, Route: route})
	if err != nil {
		judgement, _ = RuleSelfRAGJudge{}.Judge(ctx, SelfRAGJudgeInput{Query: query, Answer: answer, Chunks: chunks, CRAG: crag, Route: route})
		judgement.Reason = strings.TrimSpace(judgement.Reason + "；Self-RAG judge降级: " + err.Error())
	}
	return judgement
}

func localRerank(query string, chunks []rankedChunk) []rankedChunk {
	terms := extractKeywords(query, 20)
	for i := range chunks {
		exact := keywordScore(terms, chunks[i].Chunk.Content)
		title := keywordScore(terms, chunks[i].Document.Title)
		chunks[i].Score = chunks[i].Score + 0.001*exact + 0.0005*title
		chunks[i].Reason += fmt.Sprintf("; local_rerank exact=%.2f title=%.2f", exact, title)
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].Score > chunks[j].Score })
	return chunks
}

func retrievalQuality(chunks []rankedChunk) float64 {
	if len(chunks) == 0 {
		return 0
	}
	total := 0.0
	for i, chunk := range chunks {
		weight := 1 / float64(i+1)
		total += chunk.Score * weight
	}
	quality := total / float64(len(chunks))
	if quality <= 0.05 {
		return math.Min(quality/0.03, 1)
	}
	return quality
}

func (s *ragServiceImpl) synthesizeDirectAnswer(ctx context.Context, viewerID int64, query string, decision RouterDecision) string {
	router := s.resolveUserRouter(ctx, viewerID)
	llm, ok := unwrapLLMRouter(router)
	if ok && llm != nil && strings.TrimSpace(llm.APIKey) != "" && strings.TrimSpace(llm.BaseURL) != "" {
		systemPrompt := "你是 ClaranAIM 的知识助手。当前 RAG Router 判断这个问题不需要检索内部知识库。请直接回答用户问题，面向业务用户写成清晰 Markdown。不要输出 Router JSON，不要说“我可以帮你”，不要编造内部知识库来源。"
		userPrompt := fmt.Sprintf("用户问题：%s\n\nRouter判断：%s\n\n请生成当前页面要展示的最终回答。", query, defaultString(decision.Reason, "无需检索"))
		if answer, err := llm.chat(ctx, systemPrompt, userPrompt); err == nil && strings.TrimSpace(answer) != "" {
			return answer
		}
	}
	return synthesizeDirectAnswerFallback(query, decision)
}

func synthesizeDirectAnswerFallback(query string, decision RouterDecision) string {
	var b strings.Builder
	b.WriteString("这个问题当前被判断为可以直接回答，不需要检索内部知识库。\n\n")
	b.WriteString("**直接建议**\n")
	b.WriteString("- 请围绕问题「")
	b.WriteString(escapePlainForMarkdown(query))
	b.WriteString("」补充你的目标、素材和约束；如果你想查找已经上传的文档，请切换到“文档检索”模式，系统会强制检索知识库。\n")
	if strings.TrimSpace(decision.Reason) != "" {
		b.WriteString("\n**路由原因**\n")
		b.WriteString(escapePlainForMarkdown(decision.Reason))
	}
	b.WriteString("\n\n> 当前使用本地兜底直答；配置可用的小模型后会在这里生成完整回答。")
	return b.String()
}

func (s *ragServiceImpl) synthesizeAnswer(ctx context.Context, viewerID int64, query, mode, crag string, chunks []rankedChunk, nodes []*rag.RAGGraphNode) string {
	if answer, err := s.synthesizeAnswerWithLLM(ctx, viewerID, query, mode, crag, chunks, nodes); err == nil && strings.TrimSpace(answer) != "" {
		return answer
	}
	return synthesizeAnswerFallback(query, mode, crag, chunks, nodes)
}

func (s *ragServiceImpl) synthesizeAnswerWithLLM(ctx context.Context, viewerID int64, query, mode, crag string, chunks []rankedChunk, nodes []*rag.RAGGraphNode) (string, error) {
	if len(chunks) == 0 {
		return "", errors.New("没有可供生成的内部来源")
	}
	router := s.resolveUserRouter(ctx, viewerID)
	llm, ok := unwrapLLMRouter(router)
	if !ok || llm == nil || strings.TrimSpace(llm.APIKey) == "" || strings.TrimSpace(llm.BaseURL) == "" {
		return "", errors.New("RAG答案生成模型未配置")
	}
	var sourceBuilder strings.Builder
	for i, chunk := range chunks {
		if i >= 6 {
			break
		}
		content := cleanRAGContextText(chunk.Chunk.Content)
		sourceBuilder.WriteString(fmt.Sprintf("[来源%d] 标题：%s\n路径/来源：%s\n相关度：%.4f\n内容：%s\n\n", i+1, chunk.Document.Title, chunk.Document.Source, chunk.Score, truncate(content, 1600)))
	}
	if len(nodes) > 0 {
		sourceBuilder.WriteString("相关图谱实体：")
		for i, node := range nodes {
			if i >= 8 {
				break
			}
			if i > 0 {
				sourceBuilder.WriteString("；")
			}
			sourceBuilder.WriteString(fmt.Sprintf("%s(%s): %s", node.Name, node.Type, truncate(node.Summary, 120)))
		}
		sourceBuilder.WriteString("\n")
	}
	systemPrompt := "你是 ClaranAIM 的知识库问答助手。请只依据给定内部来源回答用户问题，面向普通用户写成清晰 Markdown。不要输出原始 chunk 列表；先给直接答案，再给依据。若资料不足，明确说明缺口。引用来源时用文档标题，不编造不存在的信息。"
	userPrompt := fmt.Sprintf("用户问题：%s\n\n检索模式：%s\nCRAG结果：%s\n\n内部来源：\n%s\n\n请生成最终回答。", query, mode, crag, sourceBuilder.String())
	return llm.chat(ctx, systemPrompt, userPrompt)
}

func unwrapLLMRouter(router RAGRouter) (*LLMRouter, bool) {
	switch typed := router.(type) {
	case *LLMRouter:
		return typed, true
	case HybridAdaptiveRouter:
		if llm, ok := typed.LLM.(*LLMRouter); ok {
			return llm, true
		}
	case fallbackRouter:
		if llm, ok := unwrapLLMRouter(typed.primary); ok {
			return llm, true
		}
		return unwrapLLMRouter(typed.fallback)
	}
	return nil, false
}

func (r *LLMRouter) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if r == nil || r.APIKey == "" || r.BaseURL == "" {
		return "", errors.New("llm未配置")
	}
	payload := map[string]interface{}{
		"model": r.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(r.BaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm调用失败: status=%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("llm未返回答案")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func synthesizeAnswerFallback(query, mode, crag string, chunks []rankedChunk, nodes []*rag.RAGGraphNode) string {
	var b strings.Builder
	if len(chunks) == 0 {
		b.WriteString("没有在内部知识库中找到足够相关的资料。")
		if crag == "web_fallback" {
			b.WriteString("建议继续使用联网增强，或补充更明确的文档标题、关键词和范围。")
		}
		return b.String()
	}
	b.WriteString("根据知识库中命中的资料，问题「")
	b.WriteString(escapePlainForMarkdown(query))
	b.WriteString("」可以这样理解：\n\n")
	b.WriteString("- 主要依据来自 ")
	titles := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		if i >= 5 {
			break
		}
		titles = append(titles, chunk.Document.Title)
	}
	b.WriteString(escapePlainForMarkdown(strings.Join(uniqueStrings(titles), "、")))
	b.WriteString("。\n")
	best := cleanRAGContextText(chunks[0].Chunk.Content)
	b.WriteString("- 最相关内容显示：")
	b.WriteString(escapePlainForMarkdown(truncate(best, 360)))
	b.WriteString("\n")
	if len(chunks) > 1 {
		b.WriteString("- 其他来源提供了补充证据，建议结合下方“来源”逐条核对。\n")
	}
	if len(nodes) > 0 {
		b.WriteString("\n相关图谱实体：")
		for i, node := range nodes {
			if i >= 6 {
				break
			}
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(node.Name)
		}
	}
	b.WriteString("\n\n> 当前回答使用本地兜底生成；如果配置了可用的小模型/大模型，系统会进一步生成更自然的总结。")
	return b.String()
}

func cleanRAGContextText(text string) string {
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">")
	text = replacer.Replace(text)
	text = regexp.MustCompile(`(?is)<script.*?</script>|<style.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)</?(table|thead|tbody|tr|td|th|p|div|span|strong|em|ul|ol|li|h[1-6])[^>]*>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func escapePlainForMarkdown(text string) string {
	return strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func chunksToSources(chunks []rankedChunk) []*rag.RAGSource {
	out := make([]*rag.RAGSource, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, &rag.RAGSource{DocumentId: chunk.Document.ID, ChunkId: chunk.Chunk.ID, Title: chunk.Document.Title, Content: truncate(chunk.Chunk.Content, 500), Source: chunk.Document.Source, Score: chunk.Score, Reason: chunk.Reason})
	}
	return out
}

func splitIntoChunks(text string, size int) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	var chunks []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[start:end])))
	}
	return chunks
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9_\-\p{Han}]{2,}`)

func extractKeywords(text string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for _, match := range wordRe.FindAllString(strings.ToLower(text), -1) {
		match = strings.TrimSpace(match)
		if match == "" || seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func keywordScore(terms []string, text string) float64 {
	if len(terms) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	hits := 0
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			hits++
		}
	}
	return float64(hits) / float64(len(terms))
}

func hashEmbedding(text string, dim int) []float32 {
	vec := make([]float32, dim)
	for _, token := range extractKeywords(text, 0) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32()) % dim
		vec[idx] += 1
	}
	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm == 0 {
		return vec
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func cosine(a, b []float32) float64 {
	n := min(len(a), len(b))
	sum := float64(0)
	for i := 0; i < n; i++ {
		sum += float64(a[i] * b[i])
	}
	return sum
}

func qualityScore(text string) float64 {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}
	score := math.Min(float64(len(runes))/900, 1)
	if strings.Contains(text, "。") || strings.Contains(text, ".") {
		score += 0.1
	}
	if score > 1 {
		return 1
	}
	return score
}

func extractEntities(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range wordRe.FindAllString(text, -1) {
		if len([]rune(token)) < 2 {
			continue
		}
		first := []rune(token)[0]
		if unicode.IsUpper(first) || unicode.Is(unicode.Han, first) {
			if !seen[token] {
				seen[token] = true
				out = append(out, token)
			}
		}
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func inferEntityType(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "-service") || strings.Contains(lower, "-gateway") || strings.Contains(name, "服务") || strings.Contains(name, "网关") {
		return "Service"
	}
	if strings.Contains(lower, "_") || strings.HasSuffix(lower, "table") || strings.Contains(name, "表") {
		return "DatabaseTable"
	}
	if strings.Contains(lower, ".events") || strings.Contains(lower, "topic") || strings.Contains(name, "事件") || strings.Contains(name, "主题") {
		return "EventTopic"
	}
	if strings.Contains(lower, "/api/") || strings.HasPrefix(lower, "api") {
		return "API"
	}
	if strings.Contains(name, "部") || strings.Contains(name, "组") || strings.Contains(name, "团队") {
		return "Organization"
	}
	if strings.Contains(name, "系统") || strings.Contains(name, "平台") || strings.Contains(name, "产品") {
		return "Product"
	}
	return "Concept"
}

func communityName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "agent") || strings.Contains(name, "助手") || strings.Contains(name, "调度"):
		return "Agent 事件链路"
	case strings.Contains(lower, "message") || strings.Contains(lower, "msg") || strings.Contains(lower, "conversation") || strings.Contains(name, "消息") || strings.Contains(name, "会话"):
		return "IM 消息链路"
	case strings.Contains(lower, "rag") || strings.Contains(lower, "memory") || strings.Contains(lower, "milvus") || strings.Contains(name, "记忆") || strings.Contains(name, "知识"):
		return "Memory/RAG"
	case strings.Contains(lower, "file") || strings.Contains(lower, "minio") || strings.Contains(lower, "ocr") || strings.Contains(name, "文件"):
		return "文件服务"
	default:
		return "通用知识图谱"
	}
}

func firstLine(text, fallback string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncate(line, 80)
		}
	}
	return fallback
}

func truncate(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func documentToRPC(doc *model.Document) rag.RAGDocument {
	return rag.RAGDocument{Id: doc.ID, OwnerId: doc.OwnerID, Title: doc.Title, Source: doc.Source, SourceType: doc.SourceType, Visibility: doc.Visibility, GroupId: doc.GroupID, ConversationId: doc.ConversationID, Status: doc.Status, CreatedAt: doc.CreatedAt.Format(time.RFC3339), UpdatedAt: doc.UpdatedAt.Format(time.RFC3339)}
}

func entitiesToRPC(entities []model.Entity) []*rag.RAGGraphNode {
	out := make([]*rag.RAGGraphNode, 0, len(entities))
	for _, e := range entities {
		out = append(out, &rag.RAGGraphNode{Id: e.ID, Name: e.Name, Type: e.Type, Summary: e.Summary, CommunityId: e.CommunityID, Score: e.Score})
	}
	return out
}

func relationsToRPC(relations []model.Relation) []*rag.RAGGraphEdge {
	out := make([]*rag.RAGGraphEdge, 0, len(relations))
	for _, r := range relations {
		out = append(out, &rag.RAGGraphEdge{Id: r.ID, SourceId: r.SourceID, TargetId: r.TargetID, Relation: r.Relation, Weight: r.Weight, Evidence: r.Evidence, DocumentId: r.DocumentID})
	}
	return out
}

func filterGraphForDisplay(entities []model.Entity, relations []model.Relation) ([]model.Entity, []model.Relation) {
	validID := map[int64]bool{}
	entityByID := map[int64]model.Entity{}
	filteredEntities := make([]model.Entity, 0, len(entities))
	for _, entity := range entities {
		extracted := extractedEntity{Name: entity.Name, Type: entity.Type, Description: entity.Summary, Aliases: decodeAliases(entity.AliasesJSON)}
		if !isUsefulGraphEntity(extracted, entity.Summary, true) {
			continue
		}
		entity.Type = normalizeEntityType(entity.Type, entity.Name)
		filteredEntities = append(filteredEntities, entity)
		validID[entity.ID] = true
		entityByID[entity.ID] = entity
	}
	filteredRelations := make([]model.Relation, 0, len(relations))
	seen := map[string]bool{}
	for _, relation := range relations {
		if !validID[relation.SourceID] || !validID[relation.TargetID] || relation.SourceID == relation.TargetID {
			continue
		}
		relation.Relation = normalizeRelationType(relation.Relation)
		source := entityByID[relation.SourceID]
		target := entityByID[relation.TargetID]
		if !relationAllowedByEntityTypes(relation.Relation, source.Type, target.Type) {
			continue
		}
		if relation.Relation == "RELATED_TO" && relation.Weight < 0.82 {
			continue
		}
		key := fmt.Sprintf("%d-%s-%d", relation.SourceID, relation.Relation, relation.TargetID)
		if seen[key] {
			continue
		}
		seen[key] = true
		filteredRelations = append(filteredRelations, relation)
	}
	return filteredEntities, filteredRelations
}

func communitiesToRPC(communities []model.Community) []*rag.RAGGraphCommunity {
	out := make([]*rag.RAGGraphCommunity, 0, len(communities))
	for _, c := range communities {
		out = append(out, &rag.RAGGraphCommunity{Id: c.ID, Name: c.Name, Summary: c.Summary, Level: c.Level})
	}
	return out
}
