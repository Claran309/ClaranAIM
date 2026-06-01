package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Reranker 抽象模型精排能力。
// 输入是 RRF 后的候选 parent context，输出是每个候选和 query 的相关性分数。
type Reranker interface {
	Rerank(ctx context.Context, query string, chunks []rankedChunk, topN int) ([]RerankScore, error)
}

// RerankScore 表示模型对某个候选的相关性评分，Index 对应输入 chunks 的下标。
type RerankScore struct {
	Index int
	Score float64
}

// GLMReranker 调用智谱 rerank 接口。
type GLMReranker struct {
	URL    string
	APIKey string
	Model  string
	Client *http.Client
}

func NewGLMReranker(url, apiKey, model string) *GLMReranker {
	return &GLMReranker{
		URL:    strings.TrimSpace(url),
		APIKey: strings.TrimSpace(apiKey),
		Model:  defaultString(model, "rerank"),
		Client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (r *GLMReranker) Rerank(ctx context.Context, query string, chunks []rankedChunk, topN int) ([]RerankScore, error) {
	if r == nil || r.URL == "" || r.APIKey == "" {
		return nil, errors.New("reranker未配置")
	}
	if topN <= 0 || topN > len(chunks) {
		topN = len(chunks)
	}
	documents := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		documents = append(documents, strings.TrimSpace(chunk.Document.Title+"\n"+chunk.Chunk.Content))
	}
	payload, err := json.Marshal(map[string]interface{}{
		"model":     r.Model,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank接口返回状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Results) == 0 {
		return nil, errors.New("rerank接口未返回结果")
	}
	scores := make([]RerankScore, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		if item.Index < 0 || item.Index >= len(chunks) {
			continue
		}
		scores = append(scores, RerankScore{Index: item.Index, Score: item.RelevanceScore})
	}
	if len(scores) == 0 {
		return nil, errors.New("rerank接口返回的index无效")
	}
	return scores, nil
}

func applyModelRerank(query string, chunks []rankedChunk, scores []RerankScore, topN int) []rankedChunk {
	if len(chunks) == 0 || len(scores) == 0 {
		return localRerank(query, chunks)
	}
	if topN <= 0 || topN > len(chunks) {
		topN = len(chunks)
	}
	reranked := make([]rankedChunk, 0, len(scores))
	used := map[int]bool{}
	for _, score := range scores {
		if score.Index < 0 || score.Index >= len(chunks) || used[score.Index] {
			continue
		}
		chunk := chunks[score.Index]
		chunk.Score = score.Score
		chunk.Reason += fmt.Sprintf("; model_rerank=%.4f", score.Score)
		reranked = append(reranked, chunk)
		used[score.Index] = true
	}
	sort.Slice(reranked, func(i, j int) bool { return reranked[i].Score > reranked[j].Score })
	if len(reranked) > topN {
		reranked = reranked[:topN]
	}
	return reranked
}
