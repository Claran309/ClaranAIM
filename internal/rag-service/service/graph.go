package service

import (
	"ClaranAIM/internal/rag-service/dao"
	"ClaranAIM/internal/rag-service/model"
	"ClaranAIM/kitex_gen/rag"
	"context"
	"errors"
	"fmt"
	"strings"
)

func looksLikeLegacyDocumentGraph(nodes []model.Entity, edges []model.Relation, documentID int64) bool {
	if documentID <= 0 || len(nodes) == 0 || len(edges) == 0 {
		return false
	}
	hasDocumentNode := false
	allConcept := true
	onlyRelated := true
	for _, node := range nodes {
		if node.CanonicalKey == fmt.Sprintf("document:%d", documentID) {
			hasDocumentNode = true
		}
		if !isGenericEntityType(node.Type) {
			allConcept = false
		}
	}
	for _, edge := range edges {
		relation := normalizeRelationType(edge.Relation)
		if relation == "CONTAINS" {
			hasDocumentNode = true
		}
		if !isGenericRelatedRelation(edge.Relation) {
			onlyRelated = false
		}
	}
	return !hasDocumentNode && (allConcept || onlyRelated)
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
	documentTitle := ""
	for _, row := range rows {
		if strings.TrimSpace(row.Document.Title) != "" {
			documentTitle = row.Document.Title
			break
		}
	}
	s.buildGraph(ctx, ownerID, documentID, documentTitle, sourceType, chunks, "balanced")
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
	docs, total, err := s.repo.ListDocuments(ctx, dao.SearchFilter{ViewerID: viewerID, Limit: limit, Offset: offset, KnowledgeOnly: true})
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
	if s.graphStore != nil {
		if _, err := s.repo.GetOwnedDocument(ctx, viewerID, documentID); err != nil {
			return err
		}
		if err := s.graphStore.DeleteDocumentGraph(ctx, viewerID, documentID); err != nil {
			return err
		}
	}
	return s.repo.DeleteDocument(ctx, viewerID, documentID)
}

func (s *ragServiceImpl) DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) error {
	if s.repo == nil {
		return errors.New("rag repository未配置")
	}
	if s.graphStore == nil {
		return errors.New("graph store未配置")
	}
	if _, err := s.repo.GetOwnedDocument(ctx, viewerID, documentID); err != nil {
		return err
	}
	if err := s.graphStore.DeleteDocumentGraph(ctx, viewerID, documentID); err != nil {
		return err
	}
	s.rebuildGraphCommunities(ctx, viewerID)
	return nil
}

func shouldBuildGraphForSourceType(sourceType string) bool {
	if isHiddenKnowledgeDocumentSourceType(sourceType) {
		return false
	}
	return true
}

func hiddenKnowledgeDocumentSourceTypes() []string {
	return []string{"conversation", "chat", "conversation_summary", "conversation_topic", "conversation_digest"}
}

func isHiddenKnowledgeDocumentSourceType(sourceType string) bool {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	for _, item := range hiddenKnowledgeDocumentSourceTypes() {
		if sourceType == item {
			return true
		}
	}
	return false
}

func normalizeGraphRelationMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "balanced", "default", "", "generic", "general", "loose", "泛化", "precise", "strict", "精细":
		return "balanced"
	default:
		return "balanced"
	}
}

func graphRelationModeLabel(mode string) string {
	return "平衡"
}
