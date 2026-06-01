// Package service 实现 Milvus-ready RAG、Hybrid Search 和 GraphRAG MVP。
package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/pkg/idgen"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
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
}

type SearchResult = rag.SearchResp

type GraphInput struct {
	ViewerID int64
	Query    string
	Limit    int
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
	repo          dao.Repository
	vectorIndex   VectorIndex
	embedder      EmbeddingProvider
	router        RAGRouter
	reranker      Reranker
	cragEvaluator CRAGEvaluator
	selfJudge     SelfRAGJudge
	settings      settingsclient.Service
	routerFactory func(settingsclient.ResolvedLLMConfig) RAGRouter
	embeddingDim  int
	defaultMode   string
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
	return &ragServiceImpl{repo: repo, vectorIndex: vectorIndex, router: HybridAdaptiveRouter{}, embeddingDim: embeddingDim, defaultMode: defaultMode}
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
	storedChunks, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: input.OwnerID, Limit: 500})
	if err == nil {
		for _, row := range storedChunks {
			if row.Chunk.DocumentID != doc.ID || normalizedChunkLevel(row.Chunk) != model.ChunkLevelChild {
				continue
			}
			_ = s.vectorIndex.Upsert(ctx, row.Chunk.ID, s.embedding(ctx, row.Chunk.Content), map[string]string{"document_id": fmt.Sprint(doc.ID)})
		}
	}
	entityCount, relationCount := s.buildGraph(ctx, input.OwnerID, doc.ID, chunks)
	dto := documentToRPC(doc)
	return IngestResult{Document: dto, ChunkCount: int64(countChildChunks(chunks)), EntityCount: entityCount, RelationCount: relationCount}, nil
}

// Search 执行 Adaptive RAG：按问题复杂度选择 Hybrid/GraphRAG/Text-to-SQL/简单检索，并附带 CRAG 和 Self-RAG 检查点。
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
	if mode == "" || mode == "adaptive" {
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
		return SearchResult{
			Success:    true,
			Answer:     "这个问题不需要检索知识库，可以直接由对话模型回答。RAG Router 判断原因：" + defaultString(decision.Reason, "无需检索"),
			Route:      "direct",
			CragAction: "skip_vector",
			SelfCheck:  self,
		}, nil
	}
	if decision.Route == AdaptiveRouteWebRAG || mode == "web" {
		return s.externalRouteHint(query, decision, self, "web_rag", "这个问题需要外部实时资料。当前 RAG 服务已路由到 Web RAG，但 Web Search 工具由 agent-runtime 执行；请让 Agent 调用 web_search 或接入 web-search-service 后再合并内部资料。"), nil
	}
	if decision.Route == AdaptiveRouteMemoryRAG || mode == "memory" {
		return s.externalRouteHint(query, decision, self, "memory_rag", "这个问题需要私有记忆。当前 memory-service 仍是 MySQL 事实记忆 MVP，尚未接入 RAG/Milvus 检索；请先通过 memory-service 召回记忆后再回答。"), nil
	}
	if decision.Route == AdaptiveRouteToolAction || mode == "tool_action" {
		return s.externalRouteHint(query, decision, self, "tool_action", "这是动作请求，不应由 RAG 检索直接执行。请交给 Agent 工具审批/执行链路，并保留授权和审计。"), nil
	}
	if mode == "text_to_sql" {
		return s.textToSQLHint(query, self), nil
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
	graph, _ := s.GetGraph(ctx, GraphInput{ViewerID: input.ViewerID, Query: query, Limit: 30})
	answer := synthesizeAnswer(query, mode, cragAction, reranked, graph.Nodes)
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
	nodes, edges, communities, err := s.repo.ListGraph(ctx, input.ViewerID, input.Query, input.Limit)
	if err != nil {
		return GraphResult{}, err
	}
	return GraphResult{Success: true, Nodes: entitiesToRPC(nodes), Edges: relationsToRPC(edges), Communities: communitiesToRPC(communities)}, nil
}

func (s *ragServiceImpl) ListDocuments(ctx context.Context, viewerID int64, limit, offset int) ([]rag.RAGDocument, int64, error) {
	docs, total, err := s.repo.ListDocuments(ctx, dao.SearchFilter{ViewerID: viewerID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]rag.RAGDocument, 0, len(docs))
	for i := range docs {
		out = append(out, documentToRPC(&docs[i]))
	}
	return out, total, nil
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
			out = append(out, part)
		}
	}
	if len(out) == 0 && strings.TrimSpace(content) != "" {
		out = splitIntoChunks(content, 1200)
	}
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
	rows, err := s.repo.ListChunks(ctx, dao.SearchFilter{ViewerID: input.ViewerID, GroupID: input.GroupID, ConversationID: input.ConversationID, Limit: 300})
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

type graphExtractor interface {
	Extract(ctx context.Context, input graphExtractInput) (graphExtractResult, error)
}

type ruleGraphExtractor struct{}

func (s *ragServiceImpl) buildGraph(ctx context.Context, ownerID, documentID int64, chunks []model.Chunk) (int64, int64) {
	extractor := graphExtractor(ruleGraphExtractor{})
	entityByCanonical := map[string]*model.Entity{}
	entityCount := int64(0)
	relationCount := int64(0)
	for _, chunk := range chunks {
		if normalizedChunkLevel(chunk) != model.ChunkLevelChild {
			continue
		}
		result, err := extractor.Extract(ctx, graphExtractInput{DocumentID: documentID, Chunk: chunk})
		if err != nil {
			continue
		}
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
			relation := &model.Relation{
				OwnerID:         ownerID,
				SourceID:        source.ID,
				TargetID:        target.ID,
				Relation:        normalizeRelationType(extracted.Type),
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
	return entityCount, relationCount
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
	community := communityForEntity(ownerID, *entity)
	_ = s.repo.SaveCommunity(ctx, community)
	entity.CommunityID = community.ID
	_ = s.repo.SaveEntity(ctx, entity)
	return entity
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
	if len(out.Relationships) == 0 && len(out.Entities) > 1 {
		for i := 0; i < len(out.Entities)-1 && i < 3; i++ {
			out.Relationships = append(out.Relationships, extractedRelationship{
				Source:      out.Entities[i].Name,
				Target:      out.Entities[i+1].Name,
				Type:        "RELATED_TO",
				Description: "两个实体在同一知识分块中共同出现",
				Evidence:    truncate(text, 240),
				Confidence:  0.45,
			})
		}
	}
	return out, nil
}

func extractGraphEntities(text string) []extractedEntity {
	seen := map[string]bool{}
	var out []extractedEntity
	add := func(name string, aliases ...string) {
		name = strings.Trim(strings.TrimSpace(name), "，。,.；;:：()（）[]【】")
		if name == "" {
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
	camelRe := regexp.MustCompile(`[A-Z][A-Za-z0-9]{2,}(?:\s+[A-Z][A-Za-z0-9]{2,})?`)
	for _, match := range camelRe.FindAllString(text, -1) {
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
	chineseRe := regexp.MustCompile(`[\p{Han}]{2,16}(?:服务|模块|系统|平台|产品|组织|团队|用户|助手|数据库|表|事件|主题|知识库|图谱)`)
	for _, match := range chineseRe.FindAllString(text, -1) {
		add(match)
	}
	return out
}

func extractGraphRelationships(text string, entities []extractedEntity) []extractedRelationship {
	if len(entities) < 2 {
		return nil
	}
	var out []extractedRelationship
	for _, sentence := range splitGraphSentences(text) {
		names := entityNamesInText(sentence, entities)
		if len(names) < 2 {
			continue
		}
		relationType := relationTypeFromSentence(sentence)
		if relationType == "" {
			continue
		}
		source, target := names[0], names[1]
		if relationType == "CONSUMES" || relationType == "READS" || relationType == "WRITES" || relationType == "PUBLISHES" || relationType == "CALLS" {
			source, target = relationDirection(sentence, names, relationType)
		}
		out = append(out, extractedRelationship{
			Source:      source,
			Target:      target,
			Type:        relationType,
			Description: relationDescription(source, target, relationType),
			Evidence:    sentence,
			Confidence:  0.82,
		})
	}
	return out
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
	lowerSentence := strings.ToLower(sentence)
	var names []string
	seen := map[string]bool{}
	for _, entity := range entities {
		candidates := append([]string{entity.Name}, entity.Aliases...)
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if strings.Contains(lowerSentence, strings.ToLower(candidate)) {
				key := canonicalEntityKey(entity.Name)
				if !seen[key] {
					seen[key] = true
					names = append(names, entity.Name)
				}
				break
			}
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.Index(lowerSentence, strings.ToLower(names[i])) < strings.Index(lowerSentence, strings.ToLower(names[j]))
	})
	return names
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

func relationDirection(sentence string, names []string, relationType string) (string, string) {
	if len(names) < 2 {
		return "", ""
	}
	switch relationType {
	case "PUBLISHES", "WRITES", "CALLS", "CONSUMES", "READS", "STORES", "CONFIGURES", "TRIGGERS":
		return names[0], names[1]
	default:
		return names[0], names[1]
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

func (s *ragServiceImpl) textToSQLHint(query string, self *rag.SelfRAGCheckpoints) SearchResult {
	self.Retrieve = false
	self.IsRel = true
	self.IsSup = false
	self.IsUse = true
	self.Note = "该问题更适合 Text-to-SQL；当前未绑定结构化数据源，返回 SQL RAG 计划"
	return SearchResult{
		Success:    true,
		Answer:     "这是结构化数据问题，适合 Text-to-SQL RAG。当前 rag-service 已识别该路线，但还没有绑定可查询的数据源 schema。建议下一步为数据表注册 schema、只读 SQL 执行器和 SQL 安全审计。",
		Route:      "text_to_sql",
		CragAction: "skip_vector",
		SelfCheck:  self,
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

func synthesizeAnswer(query, mode, crag string, chunks []rankedChunk, nodes []*rag.RAGGraphNode) string {
	var b strings.Builder
	b.WriteString("RAG 路线：")
	b.WriteString(mode)
	b.WriteString("；CRAG：")
	b.WriteString(crag)
	b.WriteString("\n\n")
	if len(chunks) == 0 {
		b.WriteString("内部知识库没有找到足够相关资料。")
		if crag == "web_fallback" {
			b.WriteString("建议回退到 Web 搜索或让 Agent 调用 web_search 后合并结果。")
		}
		return b.String()
	}
	b.WriteString("基于内部知识库，和问题「")
	b.WriteString(query)
	b.WriteString("」最相关的信息如下：\n")
	for i, chunk := range chunks {
		if i >= 5 {
			break
		}
		b.WriteString(fmt.Sprintf("%d. %s：%s\n", i+1, chunk.Document.Title, truncate(chunk.Chunk.Content, 160)))
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
	return b.String()
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
	if strings.Contains(lower, "-service") || strings.Contains(name, "服务") {
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
		out = append(out, &rag.RAGGraphEdge{Id: r.ID, SourceId: r.SourceID, TargetId: r.TargetID, Relation: r.Relation, Weight: r.Weight, Evidence: r.Evidence})
	}
	return out
}

func communitiesToRPC(communities []model.Community) []*rag.RAGGraphCommunity {
	out := make([]*rag.RAGGraphCommunity, 0, len(communities))
	for _, c := range communities {
		out = append(out, &rag.RAGGraphCommunity{Id: c.ID, Name: c.Name, Summary: c.Summary, Level: c.Level})
	}
	return out
}
