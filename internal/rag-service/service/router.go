package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	AdaptiveRouteDirect     = "direct"
	AdaptiveRouteProjectRAG = "project_rag"
	AdaptiveRouteStrictRAG  = "strict_rag"
	AdaptiveRouteWebRAG     = "web_rag"
	AdaptiveRouteMemoryRAG  = "memory_rag"
	AdaptiveRouteToolAction = "tool_action"
)

// RouterInput 是 RAG 路由器判断是否需要检索时看到的最小上下文。
type RouterInput struct {
	Query          string
	DefaultMode    string
	GroupID        int64
	ConversationID int64
}

// RouterDecision 是 LLM router 或规则 router 对检索路线的判断结果。
type RouterDecision struct {
	Route           string
	Mode            string
	Complexity      string
	Retrieve        bool
	RetrievalSource string
	Sources         []string
	Strategy        string
	Query           string
	Reason          string
}

// RAGRouter 抽象“是否需要 RAG、走哪条 RAG 路线”的决策能力。
type RAGRouter interface {
	Route(ctx context.Context, input RouterInput) (RouterDecision, error)
}

// RouterFunc 方便测试用函数直接实现 RAGRouter。
type RouterFunc func(ctx context.Context, input RouterInput) (RouterDecision, error)

func (f RouterFunc) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	return f(ctx, input)
}

// RuleRouter 是 LLM router 不可用时的本地兜底。
type RuleRouter struct{}

func (RuleRouter) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	_ = ctx
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return RouterDecision{}, errors.New("query不能为空")
	}
	decision, _ := classifyByRules(query, input.DefaultMode)
	return decision, nil
}

// HybridAdaptiveRouter 先走规则分类器，规则无法确定时才调用 LLM Router。
type HybridAdaptiveRouter struct {
	LLM RAGRouter
}

func (r HybridAdaptiveRouter) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return RouterDecision{}, errors.New("query不能为空")
	}
	if decision, uncertain := classifyByRules(query, input.DefaultMode); !uncertain {
		return decision, nil
	}
	if r.LLM != nil {
		decision, err := r.LLM.Route(ctx, input)
		if err == nil {
			return normalizeRouterDecision(decision, input.DefaultMode, query), nil
		}
	}
	return defaultProjectDecision(query, input.DefaultMode, "规则不确定且LLM Router不可用"), nil
}

func classifyByRules(query, defaultMode string) (RouterDecision, bool) {
	lower := strings.ToLower(query)
	if isGreetingQuery(query) {
		return RouterDecision{Route: AdaptiveRouteDirect, Mode: "direct", Complexity: "simple", Retrieve: false, RetrievalSource: "none", Sources: []string{}, Strategy: "direct_answer", Reason: "规则命中寒暄/简单问题"}, false
	}
	if looksLikeDocumentLookup(query) {
		return RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: "document", Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs"}, Strategy: "document_search", Query: query, Reason: "规则命中文档标题/材料名称式查询，优先检索知识库而不是直接创作"}, false
	}
	if containsAny(lower, "最新", "today", "price", "价格", "版本", "release", "news", "新闻", "实时", "现在") {
		return RouterDecision{Route: AdaptiveRouteWebRAG, Mode: "web", Complexity: "medium", Retrieve: true, RetrievalSource: "web", Sources: []string{"web"}, Strategy: "web_search", Query: query, Reason: "规则命中实时/最新信息问题"}, false
	}
	if containsAny(query, "我的偏好", "我之前", "我的记忆", "根据我的", "个人习惯") || containsAny(lower, "memory") {
		return RouterDecision{Route: AdaptiveRouteMemoryRAG, Mode: "memory", Complexity: "medium", Retrieve: true, RetrievalSource: "memory", Sources: []string{"memory"}, Strategy: "memory_search", Query: query, Reason: "规则命中私有记忆问题"}, false
	}
	if containsAny(query, "帮我创建", "执行", "运行", "删除", "修改文件", "提醒负责人", "发消息", "创建任务") {
		return RouterDecision{Route: AdaptiveRouteToolAction, Mode: "tool_action", Complexity: "medium", Retrieve: false, RetrievalSource: "none", Sources: []string{}, Strategy: "tool_action", Reason: "规则命中动作/工具请求"}, false
	}
	if containsAny(query, "当前项目", "项目里", "代码里", "实现", "模块", "接口", "函数", "数据库表", "agent_dispatch_records") || containsAny(lower, "code", "repo", "function") {
		return RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: "hybrid", Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs", "code_chunks"}, Strategy: "hybrid_rerank", Query: query, Reason: "规则命中当前项目/代码问题"}, false
	}
	if containsAny(query, "高风险", "安全", "权限", "支付", "一致性", "强一致", "审计", "矛盾", "冲突") {
		return RouterDecision{Route: AdaptiveRouteStrictRAG, Mode: "hybrid", Complexity: "high", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs", "code_chunks"}, Strategy: "strict_rag", Query: query, Reason: "规则命中高风险问题"}, false
	}
	if containsAny(query, "关系", "影响", "为什么", "关联") || containsAny(lower, "graph") {
		return RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: "graphrag", Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs", "knowledge_graph"}, Strategy: "graphrag", Query: query, Reason: "规则命中关系/多跳问题"}, false
	}
	if containsAny(lower, "sql") || containsAny(query, "销售额", "最高", "统计", "排名") {
		return RouterDecision{Route: AdaptiveRouteStrictRAG, Mode: "hybrid", Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs"}, Strategy: "hybrid_rerank", Query: query, Reason: "规则命中结构化/统计问题，当前转为知识库混合检索"}, false
	}
	return RouterDecision{}, true
}

func defaultProjectDecision(query, defaultMode, reason string) RouterDecision {
	mode := sanitizeRAGMode(defaultMode, "hybrid")
	if mode == "direct" || mode == "adaptive" {
		mode = "hybrid"
	}
	return RouterDecision{Route: AdaptiveRouteProjectRAG, Mode: mode, Complexity: "medium", Retrieve: true, RetrievalSource: "project_docs", Sources: []string{"project_docs"}, Strategy: "hybrid_rerank", Query: query, Reason: reason}
}

func isGreetingQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	lower := strings.ToLower(trimmed)
	switch lower {
	case "你好", "hello", "hi", "在吗", "在不在", "哈喽", "嗨":
		return true
	}
	if len([]rune(trimmed)) <= 24 &&
		(containsAny(lower, "你好", "hello", "hi", "哈喽", "嗨") || strings.HasPrefix(lower, "hello")) &&
		containsAny(trimmed, "介绍一下自己", "你是谁", "你能做什么", "自我介绍") {
		return true
	}
	return false
}

func looksLikeDocumentLookup(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if containsAny(trimmed, "帮我写", "帮我生成", "生成一份", "写一份", "准备一份", "起草", "润色", "改写") {
		return false
	}
	if containsAny(lower, ".pdf", ".doc", ".docx", ".ppt", ".pptx", ".md", ".txt") {
		return true
	}
	if containsAny(trimmed, "知识库", "文档", "文件", "材料", "资料", "简历", "论文", "课件", "讲义", "报告", "方案", "面试", "第") {
		return true
	}
	runes := []rune(trimmed)
	if len(runes) >= 4 && len(runes) <= 24 &&
		!containsAny(trimmed, "？", "?", "为什么", "怎么", "如何", "请", "帮", "写", "生成", "创建", "删除", "修改", "执行") &&
		containsAny(trimmed, "中心", "项目", "课程", "实验", "架构", "使用", "进阶", "总结") {
		return true
	}
	return false
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// LLMRouter 使用 OpenAI-compatible chat completions 小模型判断是否需要检索。
type LLMRouter struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewLLMRouter(apiKey, baseURL, model string) *LLMRouter {
	return &LLMRouter{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:   defaultString(model, "glm-4-flash"),
		Client:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (r *LLMRouter) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	if r == nil || r.APIKey == "" || r.BaseURL == "" {
		return RouterDecision{}, errors.New("llm router未配置")
	}
	payload := map[string]interface{}{
		"model": r.Model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是Adaptive RAG Router / Classifier。只输出JSON，不要解释。你只做结构化判断，不执行工具、不绕过权限。字段: route(direct|project_rag|strict_rag|web_rag|memory_rag|tool_action), complexity(simple|medium|high), need_retrieve(boolean), sources(array), strategy(string), mode(direct|hybrid|document|graphrag|web|memory|tool_action), retrieval_source(string), query(string), reason(string)。简单寒暄 direct；普通知识库 project_rag；明确查文档/材料/简历/课件/标题时必须 project_rag + mode=document + need_retrieve=true，不要擅自改写成创作任务；复杂/高风险 strict_rag；实时最新 web_rag；私有记忆 memory_rag；执行动作 tool_action。",
			},
			{
				"role":    "user",
				"content": input.Query,
			},
		},
		"temperature": 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return RouterDecision{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return RouterDecision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return RouterDecision{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RouterDecision{}, errors.New("llm router调用失败")
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return RouterDecision{}, err
	}
	if len(decoded.Choices) == 0 {
		return RouterDecision{}, errors.New("llm router未返回结果")
	}
	return parseRouterDecision(decoded.Choices[0].Message.Content, input.DefaultMode)
}

func parseRouterDecision(content, defaultMode string) (RouterDecision, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		Retrieve        *bool    `json:"retrieve"`
		NeedRetrieve    *bool    `json:"need_retrieve"`
		Route           string   `json:"route"`
		Complexity      string   `json:"complexity"`
		Mode            string   `json:"mode"`
		RetrievalSource string   `json:"retrieval_source"`
		Sources         []string `json:"sources"`
		Strategy        string   `json:"strategy"`
		Query           string   `json:"query"`
		Reason          string   `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return RouterDecision{}, err
	}
	retrieve := parsed.Retrieve
	if retrieve == nil {
		retrieve = parsed.NeedRetrieve
	}
	if retrieve == nil {
		return RouterDecision{}, errors.New("llm router缺少retrieve字段")
	}
	mode := sanitizeRAGMode(parsed.Mode, defaultMode)
	if !*retrieve {
		mode = "direct"
	}
	source := strings.TrimSpace(parsed.RetrievalSource)
	if source == "" {
		if *retrieve {
			source = "project_docs"
		} else {
			source = "none"
		}
	}
	decision := RouterDecision{Route: parsed.Route, Mode: mode, Complexity: parsed.Complexity, Retrieve: *retrieve, RetrievalSource: source, Sources: parsed.Sources, Strategy: parsed.Strategy, Query: strings.TrimSpace(parsed.Query), Reason: strings.TrimSpace(parsed.Reason)}
	return normalizeRouterDecision(decision, defaultMode, ""), nil
}

func sanitizeRAGMode(mode, defaultMode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "direct", "hybrid", "document", "graphrag", "web", "memory", "tool_action":
		return strings.ToLower(strings.TrimSpace(mode))
	case "adaptive", "":
		return defaultString(defaultMode, "hybrid")
	default:
		return defaultString(defaultMode, "hybrid")
	}
}

func normalizeRouterDecision(decision RouterDecision, defaultMode, originalQuery string) RouterDecision {
	decision.Route = normalizeAdaptiveRoute(decision.Route, decision.Mode, decision.Retrieve)
	decision.Mode = sanitizeRAGMode(decision.Mode, defaultMode)
	if decision.Complexity == "" {
		decision.Complexity = "medium"
	}
	if decision.Query == "" {
		decision.Query = originalQuery
	}
	if decision.RetrievalSource == "" {
		decision.RetrievalSource = sourceForRoute(decision.Route)
	}
	if len(decision.Sources) == 0 && decision.RetrievalSource != "" && decision.RetrievalSource != "none" {
		decision.Sources = []string{decision.RetrievalSource}
	}
	if decision.Strategy == "" {
		decision.Strategy = strategyForRoute(decision.Route)
	}
	if decision.Reason == "" {
		decision.Reason = fmt.Sprintf("Adaptive Router route=%s strategy=%s", decision.Route, decision.Strategy)
	}
	return decision
}

func normalizeAdaptiveRoute(route, mode string, retrieve bool) string {
	switch strings.ToLower(strings.TrimSpace(route)) {
	case AdaptiveRouteDirect, AdaptiveRouteProjectRAG, AdaptiveRouteStrictRAG, AdaptiveRouteWebRAG, AdaptiveRouteMemoryRAG, AdaptiveRouteToolAction:
		return strings.ToLower(strings.TrimSpace(route))
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "direct":
		return AdaptiveRouteDirect
	case "web":
		return AdaptiveRouteWebRAG
	case "memory":
		return AdaptiveRouteMemoryRAG
	case "tool_action":
		return AdaptiveRouteToolAction
	default:
		if retrieve {
			return AdaptiveRouteProjectRAG
		}
		return AdaptiveRouteDirect
	}
}

func sourceForRoute(route string) string {
	switch route {
	case AdaptiveRouteDirect, AdaptiveRouteToolAction:
		return "none"
	case AdaptiveRouteWebRAG:
		return "web"
	case AdaptiveRouteMemoryRAG:
		return "memory"
	default:
		return "project_docs"
	}
}

func strategyForRoute(route string) string {
	switch route {
	case AdaptiveRouteDirect:
		return "direct_answer"
	case AdaptiveRouteWebRAG:
		return "web_search"
	case AdaptiveRouteMemoryRAG:
		return "memory_search"
	case AdaptiveRouteToolAction:
		return "tool_action"
	case AdaptiveRouteStrictRAG:
		return "strict_rag"
	default:
		return "hybrid_rerank"
	}
}
