package service

import (
	"ClaranAIM/internal/memory-service/model"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	milvus "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// MemoryRAGOptions 控制长期记忆召回时的候选数量、融合权重和过滤策略。
type MemoryRAGOptions struct {
	UseVector          bool
	VectorCandidateK   int
	MinScore           float64
	VectorWeight       float64
	ImportanceWeight   float64
	RecencyWeight      float64
	ScopeWeight        float64
	EnableLLMFiltering bool
	Now                func() time.Time
}

// MemoryVectorMetadata 写入向量库时同步保存的权限过滤元数据。
type MemoryVectorMetadata struct {
	BotID          int64
	UserID         int64
	OwnerUserID    int64
	GroupID        int64
	ConversationID int64
	SessionID      string
	Scope          string
	Type           string
	Visibility     string
}

// MemoryVectorFilter 表示向量检索阶段可用的粗权限过滤条件。
type MemoryVectorFilter struct {
	BotID          int64
	UserID         int64
	GroupID        int64
	ConversationID int64
	SessionID      string
}

// MemoryVectorHit 是向量库返回的候选 ID 和相似度。
type MemoryVectorHit struct {
	MemoryID int64
	Score    float64
}

// MemoryVectorIndex 抽象长期记忆向量索引。
type MemoryVectorIndex interface {
	Upsert(ctx context.Context, memoryID int64, text string, metadata MemoryVectorMetadata) error
	Search(ctx context.Context, query string, filter MemoryVectorFilter, limit int) ([]MemoryVectorHit, error)
}

// ScoredMemory 保存融合打分后的记忆候选。
type ScoredMemory struct {
	Fact        model.MemoryFact
	VectorScore float64
	FinalScore  float64
	Reason      string
}

// MemoryRelevanceFilter 是可选的小模型过滤器，用于剔除语义召回噪声。
type MemoryRelevanceFilter interface {
	Filter(ctx context.Context, query string, candidates []ScoredMemory) ([]ScoredMemory, error)
}

type localMemoryVectorIndex struct {
	mu       sync.RWMutex
	vectors  map[int64][]float32
	metadata map[int64]MemoryVectorMetadata
	embedder MemoryEmbeddingProvider
}

func NewLocalMemoryVectorIndex(embedder MemoryEmbeddingProvider) MemoryVectorIndex {
	if embedder == nil {
		embedder = HashMemoryEmbeddingProvider{Dim: 256}
	}
	return &localMemoryVectorIndex{vectors: map[int64][]float32{}, metadata: map[int64]MemoryVectorMetadata{}, embedder: embedder}
}

func (i *localMemoryVectorIndex) Upsert(ctx context.Context, memoryID int64, text string, metadata MemoryVectorMetadata) error {
	vector, err := i.embedder.Embed(ctx, text)
	if err != nil {
		vector = hashMemoryEmbedding(text, 256)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.vectors[memoryID] = append([]float32(nil), vector...)
	i.metadata[memoryID] = metadata
	return nil
}

func (i *localMemoryVectorIndex) Search(ctx context.Context, query string, filter MemoryVectorFilter, limit int) ([]MemoryVectorHit, error) {
	queryVector, err := i.embedder.Embed(ctx, query)
	if err != nil {
		queryVector = hashMemoryEmbedding(query, 256)
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	hits := make([]MemoryVectorHit, 0, len(i.vectors))
	for id, vector := range i.vectors {
		if !memoryVectorMetadataMatches(i.metadata[id], filter) {
			continue
		}
		hits = append(hits, MemoryVectorHit{MemoryID: id, Score: cosineMemory(queryVector, vector)})
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if limit <= 0 || limit > len(hits) {
		limit = len(hits)
	}
	return hits[:limit], nil
}

// MilvusMemoryVectorIndex 是长期记忆的 Milvus 向量索引。
// 它只做粗过滤和候选召回，最终权限与事实有效性仍由 MySQL 回源校验负责。
type MilvusMemoryVectorIndex struct {
	Address    string
	Collection string
	Dim        int
	embedder   MemoryEmbeddingProvider
	client     milvus.Client
}

func NewMilvusMemoryVectorIndex(ctx context.Context, address, collection string, dim int, embedder MemoryEmbeddingProvider) (*MilvusMemoryVectorIndex, error) {
	if dim <= 0 {
		dim = 256
	}
	if collection == "" {
		collection = "claran_memory_facts"
	}
	if embedder == nil {
		embedder = HashMemoryEmbeddingProvider{Dim: dim}
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cli, err := milvus.NewClient(connectCtx, milvus.Config{Address: address})
	if err != nil {
		return nil, err
	}
	idx := &MilvusMemoryVectorIndex{Address: address, Collection: collection, Dim: dim, embedder: embedder, client: cli}
	if err := idx.ensureCollection(connectCtx); err != nil {
		_ = cli.Close()
		return nil, err
	}
	return idx, nil
}

func (i *MilvusMemoryVectorIndex) Upsert(ctx context.Context, memoryID int64, text string, metadata MemoryVectorMetadata) error {
	if i == nil || i.client == nil {
		return errors.New("milvus memory client未初始化")
	}
	vector, err := i.embedder.Embed(ctx, text)
	if err != nil {
		vector = hashMemoryEmbedding(text, i.Dim)
	}
	_, err = i.client.Upsert(
		ctx,
		i.Collection,
		"",
		entity.NewColumnInt64("memory_id", []int64{memoryID}),
		entity.NewColumnInt64("bot_id", []int64{metadata.BotID}),
		entity.NewColumnInt64("user_id", []int64{metadata.UserID}),
		entity.NewColumnInt64("owner_user_id", []int64{metadata.OwnerUserID}),
		entity.NewColumnInt64("group_id", []int64{metadata.GroupID}),
		entity.NewColumnInt64("conversation_id", []int64{metadata.ConversationID}),
		entity.NewColumnVarChar("scope", []string{metadata.Scope}),
		entity.NewColumnVarChar("visibility", []string{metadata.Visibility}),
		entity.NewColumnFloatVector("vector", i.Dim, [][]float32{normalizeMemoryVector(vector, i.Dim)}),
	)
	if err != nil {
		return err
	}
	return i.client.Flush(ctx, i.Collection, false)
}

func (i *MilvusMemoryVectorIndex) Search(ctx context.Context, query string, filter MemoryVectorFilter, limit int) ([]MemoryVectorHit, error) {
	if i == nil || i.client == nil {
		return nil, errors.New("milvus memory client未初始化")
	}
	if limit <= 0 {
		limit = 20
	}
	vector, err := i.embedder.Embed(ctx, query)
	if err != nil {
		vector = hashMemoryEmbedding(query, i.Dim)
	}
	searchParam, err := entity.NewIndexFlatSearchParam()
	if err != nil {
		return nil, err
	}
	expr := fmt.Sprintf("bot_id == %d and user_id == %d", filter.BotID, filter.UserID)
	results, err := i.client.Search(
		ctx,
		i.Collection,
		nil,
		expr,
		[]string{"memory_id"},
		[]entity.Vector{entity.FloatVector(normalizeMemoryVector(vector, i.Dim))},
		"vector",
		entity.COSINE,
		limit,
		searchParam,
	)
	if err != nil {
		return nil, err
	}
	hits := make([]MemoryVectorHit, 0, limit)
	for _, result := range results {
		column := result.Fields.GetColumn("memory_id")
		if column == nil {
			continue
		}
		for row := 0; row < column.Len(); row++ {
			raw, err := column.Get(row)
			if err != nil {
				continue
			}
			id, ok := raw.(int64)
			if !ok {
				continue
			}
			score := 0.0
			if row < len(result.Scores) {
				score = float64(result.Scores[row])
			}
			hits = append(hits, MemoryVectorHit{MemoryID: id, Score: score})
		}
	}
	return hits, nil
}

func (i *MilvusMemoryVectorIndex) ensureCollection(ctx context.Context) error {
	has, err := i.client.HasCollection(ctx, i.Collection)
	if err != nil {
		return err
	}
	if has {
		return i.client.LoadCollection(ctx, i.Collection, false)
	}
	schema := entity.NewSchema().
		WithName(i.Collection).
		WithDescription("ClaranAIM memory fact vectors").
		WithAutoID(false).
		WithField(entity.NewField().WithName("memory_id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("bot_id").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("owner_user_id").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("group_id").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("conversation_id").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("scope").WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName("visibility").WithDataType(entity.FieldTypeVarChar).WithMaxLength(64)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(i.Dim)))
	if err := i.client.CreateCollection(ctx, schema, 2); err != nil {
		return err
	}
	index, err := entity.NewIndexFlat(entity.COSINE)
	if err != nil {
		return err
	}
	if err := i.client.CreateIndex(ctx, i.Collection, "vector", index, false); err != nil {
		return err
	}
	return i.client.LoadCollection(ctx, i.Collection, false)
}

// MemoryEmbeddingProvider 抽象记忆文本 embedding。
type MemoryEmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type HashMemoryEmbeddingProvider struct {
	Dim int
}

func (p HashMemoryEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	return hashMemoryEmbedding(text, p.Dim), nil
}

type GLMMemoryEmbeddingProvider struct {
	URL        string
	APIKey     string
	Model      string
	Dimensions int
	Client     *http.Client
}

func NewGLMMemoryEmbeddingProvider(url, apiKey, model string, dimensions int) *GLMMemoryEmbeddingProvider {
	return &GLMMemoryEmbeddingProvider{URL: strings.TrimSpace(url), APIKey: strings.TrimSpace(apiKey), Model: defaultString(model, "embedding-3"), Dimensions: dimensions, Client: &http.Client{Timeout: 20 * time.Second}}
}

func (p *GLMMemoryEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p == nil || p.URL == "" || p.APIKey == "" {
		return nil, errors.New("memory embedding provider未配置")
	}
	body := map[string]interface{}{"model": p.Model, "input": text}
	if p.Dimensions > 0 {
		body["dimensions"] = p.Dimensions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("memory embedding接口状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, errors.New("memory embedding接口未返回向量")
	}
	vector := make([]float32, len(decoded.Data[0].Embedding))
	for i, value := range decoded.Data[0].Embedding {
		vector[i] = float32(value)
	}
	return vector, nil
}

type LLMMemoryRelevanceFilter struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewLLMMemoryRelevanceFilter(apiKey, baseURL, model string) *LLMMemoryRelevanceFilter {
	return &LLMMemoryRelevanceFilter{APIKey: strings.TrimSpace(apiKey), BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), Model: defaultString(model, "glm-4-flash"), Client: &http.Client{Timeout: 12 * time.Second}}
}

func (f *LLMMemoryRelevanceFilter) Filter(ctx context.Context, query string, candidates []ScoredMemory) ([]ScoredMemory, error) {
	if f == nil || f.APIKey == "" || f.BaseURL == "" || len(candidates) == 0 {
		return candidates, nil
	}
	payload, err := json.Marshal(map[string]interface{}{
		"model": f.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是长期记忆过滤器。只输出JSON，格式为 {\"keep_ids\":[数字ID]}。只保留对当前用户问题真正有帮助的记忆。"},
			{"role": "user", "content": buildMemoryFilterPrompt(query, candidates)},
		},
		"temperature": 0,
	})
	if err != nil {
		return candidates, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return candidates, err
	}
	req.Header.Set("Authorization", "Bearer "+f.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return candidates, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return candidates, fmt.Errorf("memory relevance filter状态码%d", resp.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return candidates, err
	}
	if len(decoded.Choices) == 0 {
		return candidates, nil
	}
	keep := parseMemoryKeepIDs(decoded.Choices[0].Message.Content)
	if len(keep) == 0 {
		return nil, nil
	}
	out := make([]ScoredMemory, 0, len(candidates))
	for _, candidate := range candidates {
		if keep[candidateID(candidate)] {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func buildMemoryFilterPrompt(query string, candidates []ScoredMemory) string {
	var b strings.Builder
	b.WriteString("当前问题:\n")
	b.WriteString(query)
	b.WriteString("\n\n候选记忆:\n")
	for _, candidate := range candidates {
		b.WriteString(fmt.Sprintf("- id=%d score=%.3f content=%s\n", candidateID(candidate), candidate.FinalScore, candidateContent(candidate)))
	}
	return b.String()
}

func parseMemoryKeepIDs(content string) map[int64]bool {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var parsed struct {
		KeepIDs []int64 `json:"keep_ids"`
	}
	_ = json.Unmarshal([]byte(content), &parsed)
	out := map[int64]bool{}
	for _, id := range parsed.KeepIDs {
		out[id] = true
	}
	return out
}

func memoryVectorMetadataMatches(metadata MemoryVectorMetadata, filter MemoryVectorFilter) bool {
	if filter.BotID > 0 && metadata.BotID != filter.BotID {
		return false
	}
	if filter.UserID > 0 && metadata.UserID != filter.UserID {
		return false
	}
	return true
}

func hashMemoryEmbedding(text string, dim int) []float32 {
	if dim <= 0 {
		dim = 256
	}
	vector := make([]float32, dim)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32() % uint32(dim))
		vector[idx] += 1
	}
	return normalizeMemoryVector(vector, dim)
}

func normalizeMemoryVector(vector []float32, dim int) []float32 {
	if dim <= 0 {
		dim = len(vector)
	}
	out := make([]float32, dim)
	copy(out, vector)
	var norm float64
	for _, value := range out {
		norm += float64(value * value)
	}
	if norm == 0 {
		return out
	}
	norm = math.Sqrt(norm)
	for i := range out {
		out[i] = float32(float64(out[i]) / norm)
	}
	return out
}

func cosineMemory(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var dot float64
	for i := 0; i < n; i++ {
		dot += float64(a[i] * b[i])
	}
	return dot
}

func candidateID(candidate ScoredMemory) int64 {
	return candidate.Fact.ID
}

func candidateContent(candidate ScoredMemory) string {
	return candidate.Fact.Content
}

func parseMemoryInt64s(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}
