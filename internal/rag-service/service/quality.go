package service

import (
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
)

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
