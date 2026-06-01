package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	milvus "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// LocalVectorIndex 是 Milvus 不可用时的本地开发降级索引。
// 它不替代生产 Milvus，只保证 RAG 流水线、测试和前端调试不依赖外部容器状态。
type LocalVectorIndex struct {
	mu      sync.RWMutex
	vectors map[int64][]float32
}

func NewLocalVectorIndex() *LocalVectorIndex {
	return &LocalVectorIndex{vectors: map[int64][]float32{}}
}

func (i *LocalVectorIndex) Upsert(ctx context.Context, chunkID int64, vector []float32, metadata map[string]string) error {
	_ = ctx
	_ = metadata
	i.mu.Lock()
	defer i.mu.Unlock()
	cp := append([]float32(nil), vector...)
	i.vectors[chunkID] = cp
	return nil
}

func (i *LocalVectorIndex) Search(ctx context.Context, queryVector []float32, limit int) ([]VectorHit, error) {
	_ = ctx
	i.mu.RLock()
	defer i.mu.RUnlock()
	hits := make([]VectorHit, 0, len(i.vectors))
	for id, vector := range i.vectors {
		hits = append(hits, VectorHit{ChunkID: id, Score: cosine(queryVector, vector)})
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if limit <= 0 || limit > len(hits) {
		limit = len(hits)
	}
	return hits[:limit], nil
}

// MilvusVectorIndex 是生产向量检索适配器。
// 它负责连接 Milvus、按需创建 collection、upsert chunk 向量并执行 TopK 搜索。
type MilvusVectorIndex struct {
	Address    string
	Collection string
	Dim        int
	client     milvus.Client
}

func NewMilvusVectorIndex(ctx context.Context, address, collection string, dim int) (*MilvusVectorIndex, error) {
	if dim <= 0 {
		dim = 256
	}
	if collection == "" {
		collection = "claran_rag_chunks"
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cli, err := milvus.NewClient(connectCtx, milvus.Config{Address: address})
	if err != nil {
		return nil, err
	}
	idx := &MilvusVectorIndex{Address: address, Collection: collection, Dim: dim, client: cli}
	if err := idx.ensureCollection(connectCtx); err != nil {
		_ = cli.Close()
		return nil, err
	}
	return idx, nil
}

func (i *MilvusVectorIndex) Upsert(ctx context.Context, chunkID int64, vector []float32, metadata map[string]string) error {
	if i == nil || i.client == nil {
		return fmt.Errorf("milvus client未初始化")
	}
	vector = normalizeVectorDim(vector, i.Dim)
	documentID, _ := strconv.ParseInt(metadata["document_id"], 10, 64)
	_, err := i.client.Upsert(
		ctx,
		i.Collection,
		"",
		entity.NewColumnInt64("chunk_id", []int64{chunkID}),
		entity.NewColumnInt64("document_id", []int64{documentID}),
		entity.NewColumnFloatVector("vector", i.Dim, [][]float32{vector}),
	)
	if err != nil {
		return err
	}
	return i.client.Flush(ctx, i.Collection, false)
}

func (i *MilvusVectorIndex) Search(ctx context.Context, queryVector []float32, limit int) ([]VectorHit, error) {
	if i == nil || i.client == nil {
		return nil, fmt.Errorf("milvus client未初始化")
	}
	if limit <= 0 {
		limit = 8
	}
	searchParam, err := entity.NewIndexFlatSearchParam()
	if err != nil {
		return nil, err
	}
	results, err := i.client.Search(
		ctx,
		i.Collection,
		nil,
		"",
		[]string{"chunk_id"},
		[]entity.Vector{entity.FloatVector(normalizeVectorDim(queryVector, i.Dim))},
		"vector",
		entity.COSINE,
		limit,
		searchParam,
	)
	if err != nil {
		return nil, err
	}
	hits := make([]VectorHit, 0, limit)
	for _, result := range results {
		chunkColumn := result.Fields.GetColumn("chunk_id")
		if chunkColumn == nil {
			continue
		}
		for row := 0; row < chunkColumn.Len(); row++ {
			raw, err := chunkColumn.Get(row)
			if err != nil {
				continue
			}
			chunkID, ok := raw.(int64)
			if !ok {
				continue
			}
			score := float64(0)
			if row < len(result.Scores) {
				score = float64(result.Scores[row])
			}
			hits = append(hits, VectorHit{ChunkID: chunkID, Score: score})
		}
	}
	return hits, nil
}

func (i *MilvusVectorIndex) ensureCollection(ctx context.Context) error {
	has, err := i.client.HasCollection(ctx, i.Collection)
	if err != nil {
		return err
	}
	if has {
		return i.client.LoadCollection(ctx, i.Collection, false)
	}
	schema := entity.NewSchema().
		WithName(i.Collection).
		WithDescription("ClaranAIM RAG chunk vectors").
		WithAutoID(false).
		WithField(entity.NewField().WithName("chunk_id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeInt64)).
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
