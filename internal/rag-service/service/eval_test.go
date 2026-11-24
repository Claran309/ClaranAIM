package service

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
)

type evalRepo struct {
	rows []dao.ChunkWithDocument
}

func (r *evalRepo) CreateDocumentWithChunks(context.Context, *model.Document, []model.Chunk) error {
	return nil
}

func (r *evalRepo) ListDocuments(context.Context, dao.SearchFilter) ([]model.Document, int64, error) {
	return nil, 0, nil
}

func (r *evalRepo) CountChildChunksByDocumentIDs(context.Context, []int64) (map[int64]int64, error) {
	return map[int64]int64{}, nil
}

func (r *evalRepo) ListChunks(_ context.Context, filter dao.SearchFilter) ([]dao.ChunkWithDocument, error) {
	out := make([]dao.ChunkWithDocument, 0, len(r.rows))
	for _, row := range r.rows {
		if row.Document.OwnerID != filter.ViewerID && row.Document.Visibility != "public" {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *evalRepo) GetVisibleDocument(_ context.Context, viewerID, documentID int64) (*model.Document, error) {
	for _, row := range r.rows {
		if row.Document.ID == documentID && (row.Document.OwnerID == viewerID || row.Document.Visibility == "public") {
			doc := row.Document
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("document %d not visible", documentID)
}

func (r *evalRepo) GetOwnedDocument(_ context.Context, ownerID, documentID int64) (*model.Document, error) {
	for _, row := range r.rows {
		if row.Document.ID == documentID && row.Document.OwnerID == ownerID {
			doc := row.Document
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("document %d not owned", documentID)
}

func (r *evalRepo) DeleteDocument(context.Context, int64, int64) error { return nil }

type evalCase struct {
	Query     string
	Gold      int64
	Relevant  []int64
	Negatives []int64
	Denied    []int64
}

type evalCaseMetrics struct {
	Recall5        float64
	MRR            float64
	NDCG5          float64
	NegativeFalse  float64
	PermissionLeak float64
}

type evalReport struct {
	Strategy       string
	Embedding      string
	Cases          int
	Recall5        float64
	MRR            float64
	NDCG5          float64
	NegativeFalse  float64
	PermissionLeak float64
}

func evalTopics() []struct{ Title, Content string } {
	return []struct{ Title, Content string }{
		{"WebSocket同步", "WebSocket 负责实时推送、上线同步游标和 ACK 重试。"},
		{"Outbox事务", "Transactional Outbox 保证消息写入与事件发布同一事务，崩溃后可重试。"},
		{"Kafka幂等", "Kafka 消费使用 event_id 与 consumer group 做幂等，失败进入死信。"},
		{"雪花ID", "雪花 ID 由时间戳、workerID 和序列号组成，保证趋势递增。"},
		{"JWT认证", "JWT access token 和 refresh token 承载用户角色与过期时间。"},
		{"好友分组", "好友分组允许用户组织好友、设置备注和移动分组。"},
		{"群聊权限", "群聊权限覆盖群主、管理员、成员、禁言和邀请。"},
		{"消息撤回", "消息撤回会写入编辑记录并广播事件，客户端同步更新。"},
		{"已读游标", "用户级已读游标记录最后已读消息，用于未读数计算。"},
		{"多端去重", "多端同步使用 client_msg_id 去重，ACK 表示设备收到而不是已读。"},
		{"Agent身份", "Agent 作为真实系统用户拥有 UID、好友关系和群成员身份。"},
		{"Agent审批", "高风险工具通过 checkpoint 中断，用户确认后从同一运行恢复。"},
		{"运行预算", "RunBudget 限制步数、输入输出 token、成本和截止时间。"},
		{"会话摘要", "长会话保留最近 K 条消息，并把更早消息压缩成滚动摘要。"},
		{"MCP工具", "MCP Gateway 统一暴露 web_search、memory、RAG 和知识图谱工具。"},
		{"Skill注入", "Skill 是行为指令包，通过 SKILL.md 注入系统提示词。"},
		{"RAG切片", "RAG 使用父子 chunk、Markdown 标题和代码结构进行分层切片。"},
		{"Hybrid检索", "Hybrid Search 合并 dense 向量和 BM25 稀疏检索。"},
		{"RRF融合", "RRF 用排名倒数融合多路召回，避免不同分数尺度直接相加。"},
		{"Rerank精排", "Rerank 对候选片段做二次排序，提高前几条命中率。"},
		{"CRAG评估", "CRAG 根据相关性、覆盖度、具体性和冲突判断资料是否足够。"},
		{"Self-RAG", "Self-RAG 输出 IsRel、IsSup、IsUse 三个质量检查点。"},
		{"GraphRAG", "GraphRAG 抽取实体和关系，并用社区摘要提供子图上下文。"},
		{"Memory召回", "Memory-RAG 召回用户、群和会话记忆，并按权限过滤。"},
		{"可观测性", "OpenTelemetry、Prometheus、Grafana 和 Jaeger 组成可观测链路。"},
	}
}

func buildEvalCases() []evalCase {
	topics := evalTopics()
	cases := make([]evalCase, 0, len(topics)*2)
	for i, topic := range topics {
		gold := int64(i + 1)
		negatives := []int64{int64((i+1)%len(topics) + 1), int64((i+2)%len(topics) + 1)}
		base := evalCase{Gold: gold, Relevant: []int64{gold}, Negatives: negatives, Denied: []int64{999}}
		first := base
		first.Query = "什么是" + topic.Title
		cases = append(cases, first)
		second := base
		second.Query = topic.Title + " 的核心机制"
		cases = append(cases, second)
	}
	return cases
}

func buildEvalRepo() (*evalRepo, *LocalVectorIndex) {
	topics := evalTopics()
	rows := make([]dao.ChunkWithDocument, 0, len(topics)+1)
	index := NewLocalVectorIndex()
	for i, topic := range topics {
		doc := model.Document{
			ID:         int64(i + 1),
			OwnerID:    1,
			Title:      topic.Title,
			SourceType: "eval",
			Visibility: "private",
			Status:     "ready",
		}
		chunk := model.Chunk{
			ID:         int64(i + 1),
			DocumentID: doc.ID,
			OwnerID:    1,
			ChunkLevel: model.ChunkLevelChild,
			Content:    topic.Content,
			Keywords:   topic.Title,
		}
		rows = append(rows, dao.ChunkWithDocument{Chunk: chunk, Document: doc})
	}
	deniedDoc := model.Document{ID: 999, OwnerID: 2, Title: "私有拒绝样本", SourceType: "eval", Visibility: "private", Status: "ready"}
	deniedChunk := model.Chunk{ID: 999, DocumentID: 999, OwnerID: 2, ChunkLevel: model.ChunkLevelChild, Content: "仅 owner 2 可见的私有资料"}
	rows = append(rows, dao.ChunkWithDocument{Chunk: deniedChunk, Document: deniedDoc})
	return &evalRepo{rows: rows}, index
}

func evalEmbedding() (EmbeddingProvider, string) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RAG_EVAL_EMBEDDING")), "glm") {
		key := strings.TrimSpace(os.Getenv("RAG_EMBEDDING_API_KEY"))
		if key != "" {
			return NewGLMEmbeddingProvider(
				defaultString(os.Getenv("RAG_EMBEDDING_URL"), "https://open.bigmodel.cn/api/paas/v4/embeddings"),
				key,
				defaultString(os.Getenv("RAG_EMBEDDING_MODEL"), "embedding-3"),
				0,
			), "glm"
		}
	}
	return nil, "hash"
}

func topChunkIDs(chunks []rankedChunk, limit int) []int64 {
	if len(chunks) > limit {
		chunks = chunks[:limit]
	}
	out := make([]int64, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.Chunk.ID)
	}
	return out
}

func evalMetrics(c evalCase, ids []int64) evalCaseMetrics {
	relevant := map[int64]bool{}
	for _, id := range c.Relevant {
		relevant[id] = true
	}
	negative := map[int64]bool{}
	for _, id := range c.Negatives {
		negative[id] = true
	}
	denied := map[int64]bool{}
	for _, id := range c.Denied {
		denied[id] = true
	}
	hits, negHits, leak, dcg := 0, 0, 0, 0.0
	mrr := 0.0
	for i, id := range ids {
		if relevant[id] {
			hits++
			dcg += 1 / math.Log2(float64(i)+2)
			if mrr == 0 {
				mrr = 1 / float64(i+1)
			}
		}
		if negative[id] {
			negHits++
		}
		if denied[id] {
			leak++
		}
	}
	recall := 0.0
	if len(relevant) > 0 {
		recall = float64(hits) / float64(len(relevant))
	}
	idcg := 0.0
	for i := 0; i < len(relevant) && i < len(ids); i++ {
		idcg += 1 / math.Log2(float64(i)+2)
	}
	ndcg := 0.0
	if idcg > 0 {
		ndcg = dcg / idcg
	}
	negRate := 0.0
	if len(negative) > 0 {
		negRate = float64(negHits) / float64(len(negative))
	}
	leakRate := 0.0
	if len(denied) > 0 {
		leakRate = float64(leak) / float64(len(denied))
	}
	return evalCaseMetrics{Recall5: recall, MRR: mrr, NDCG5: ndcg, NegativeFalse: negRate, PermissionLeak: leakRate}
}

func aggregateEval(strategy, embedding string, cases []evalCase, results [][]int64) evalReport {
	report := evalReport{Strategy: strategy, Embedding: embedding, Cases: len(cases)}
	for i, c := range cases {
		m := evalMetrics(c, results[i])
		report.Recall5 += m.Recall5
		report.MRR += m.MRR
		report.NDCG5 += m.NDCG5
		report.NegativeFalse += m.NegativeFalse
		report.PermissionLeak += m.PermissionLeak
	}
	if len(cases) > 0 {
		divisor := float64(len(cases))
		report.Recall5 /= divisor
		report.MRR /= divisor
		report.NDCG5 /= divisor
		report.NegativeFalse /= divisor
		report.PermissionLeak /= divisor
	}
	return report
}

func TestRetrievalEvalReport(t *testing.T) {
	repo, index := buildEvalRepo()
	embedder, provider := evalEmbedding()
	svc := &ragServiceImpl{repo: repo, vectorIndex: index, embedder: embedder, embeddingDim: 256, defaultMode: "hybrid"}
	ctx := context.Background()
	chunkVectors := make(map[int64][]float32, len(repo.rows))
	for _, row := range repo.rows {
		if row.Document.OwnerID != 1 {
			continue
		}
		vector := svc.embedding(ctx, row.Chunk.Content)
		chunkVectors[row.Chunk.ID] = vector
		if err := index.Upsert(ctx, row.Chunk.ID, vector, map[string]string{"document_id": fmt.Sprintf("%d", row.Document.ID)}); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}
	cases := buildEvalCases()
	keywordResults := make([][]int64, 0, len(cases))
	vectorResults := make([][]int64, 0, len(cases))
	hybridResults := make([][]int64, 0, len(cases))
	rerankResults := make([][]int64, 0, len(cases))
	for _, c := range cases {
		rows, _ := repo.ListChunks(ctx, dao.SearchFilter{ViewerID: 1, Limit: 300})
		keywordResults = append(keywordResults, topChunkIDs(bm25RankChunks(c.Query, rows), 5))
		queryVector := svc.embedding(ctx, c.Query)
		dense := make([]rankedChunk, 0, len(rows))
		for _, row := range rows {
			dense = append(dense, rankedChunk{Chunk: row.Chunk, Document: row.Document, Score: cosine(queryVector, chunkVectors[row.Chunk.ID])})
		}
		sort.Slice(dense, func(i, j int) bool { return dense[i].Score > dense[j].Score })
		vectorResults = append(vectorResults, topChunkIDs(dense, 5))
		hybrid, err := svc.hybridRetrieve(ctx, SearchInput{ViewerID: 1, Query: c.Query, Limit: 5}, 5)
		if err != nil {
			t.Fatalf("hybridRetrieve() error = %v", err)
		}
		hybridResults = append(hybridResults, topChunkIDs(hybrid, 5))
		reranked := svc.rerank(ctx, c.Query, append([]rankedChunk(nil), hybrid...), 5)
		rerankResults = append(rerankResults, topChunkIDs(reranked, 5))
	}
	reports := []evalReport{
		aggregateEval("keyword_only", provider, cases, keywordResults),
		aggregateEval("vector_only", provider, cases, vectorResults),
		aggregateEval("hybrid_rrf", provider, cases, hybridResults),
		aggregateEval("hybrid_rrf_rerank", provider, cases, rerankResults),
	}
	t.Logf("embedding_provider=%s cases=%d", provider, len(cases))
	t.Logf("| strategy | recall@5 | mrr | ndcg@5 | negative_false_rate | permission_leak_rate |")
	t.Logf("| --- | --- | --- | --- | --- | --- |")
	for _, report := range reports {
		t.Logf("| %s | %.3f | %.3f | %.3f | %.3f | %.3f |",
			report.Strategy, report.Recall5, report.MRR, report.NDCG5, report.NegativeFalse, report.PermissionLeak)
	}
	for _, report := range reports {
		if report.PermissionLeak > 0 {
			t.Fatalf("%s leaked permission-denied chunks: %.3f", report.Strategy, report.PermissionLeak)
		}
	}
}
