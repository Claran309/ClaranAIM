// Package handler 实现 rag-service 的 Kitex RPC 入口。
package handler

import (
	ragsvc "ClaranAIM/internal/rag-service/service"
	"ClaranAIM/kitex_gen/rag"
	"context"
)

// RAGServiceImpl 把 Kitex 生成的 RPC 请求适配到业务 service。
// handler 只做 DTO 转换和错误包装；检索策略、权限裁剪、GraphRAG 构建都在 service 层完成。
type RAGServiceImpl struct {
	svc ragsvc.RAGService
}

// NewRAGServiceImpl 创建 rag-service 的 Kitex handler。
func NewRAGServiceImpl(svc ragsvc.RAGService) rag.RAGService {
	return &RAGServiceImpl{svc: svc}
}

// IngestDocument 写入知识文档，并触发分块、向量索引和轻量 GraphRAG 抽取。
func (h *RAGServiceImpl) IngestDocument(ctx context.Context, req *rag.IngestDocumentReq) (*rag.IngestDocumentResp, error) {
	result, err := h.svc.IngestDocument(ctx, ragsvc.IngestInput{
		OwnerID:        req.GetOwnerId(),
		Title:          req.GetTitle(),
		Content:        req.GetContent(),
		Source:         req.GetSource(),
		SourceType:     req.GetSourceType(),
		Visibility:     req.GetVisibility(),
		GroupID:        req.GetGroupId(),
		ConversationID: req.GetConversationId(),
		GraphRelationMode: req.GetGraphRelationMode(),
	})
	if err != nil {
		return &rag.IngestDocumentResp{Success: false, Msg: err.Error()}, nil
	}
	return &rag.IngestDocumentResp{
		Success:       true,
		Document:      &result.Document,
		ChunkCount:    result.ChunkCount,
		EntityCount:   result.EntityCount,
		RelationCount: result.RelationCount,
		Msg:           "写入成功",
	}, nil
}

// Search 执行 Adaptive RAG，返回答案、来源、GraphRAG 子图和 Self-RAG 检查点。
func (h *RAGServiceImpl) Search(ctx context.Context, req *rag.SearchReq) (*rag.SearchResp, error) {
	result, err := h.svc.Search(ctx, ragsvc.SearchInput{
		ViewerID:       req.GetViewerId(),
		Query:          req.GetQuery(),
		Mode:           req.GetMode(),
		Limit:          int(req.GetLimit()),
		GroupID:        req.GetGroupId(),
		ConversationID: req.GetConversationId(),
		DocumentID:     req.GetDocumentId(),
	})
	if err != nil {
		return &rag.SearchResp{Success: false, Msg: err.Error()}, nil
	}
	return &result, nil
}

// GetGraph 返回当前用户可见的知识图谱子图。
func (h *RAGServiceImpl) GetGraph(ctx context.Context, req *rag.GraphReq) (*rag.GraphResp, error) {
	result, err := h.svc.GetGraph(ctx, ragsvc.GraphInput{
		ViewerID:   req.GetViewerId(),
		Query:      req.GetQuery(),
		Limit:      int(req.GetLimit()),
		DocumentID: req.GetDocumentId(),
		Hops:       int(req.GetHops()),
	})
	if err != nil {
		return &rag.GraphResp{Success: false, Msg: err.Error()}, nil
	}
	return &result, nil
}

// ListDocuments 返回当前用户可见的知识库文档列表。
func (h *RAGServiceImpl) ListDocuments(ctx context.Context, req *rag.ListDocumentsReq) (*rag.ListDocumentsResp, error) {
	docs, total, err := h.svc.ListDocuments(ctx, req.GetViewerId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return &rag.ListDocumentsResp{Success: false, Msg: err.Error()}, nil
	}
	out := make([]*rag.RAGDocument, 0, len(docs))
	for i := range docs {
		doc := docs[i]
		out = append(out, &doc)
	}
	return &rag.ListDocumentsResp{Success: true, Documents: out, Total: total}, nil
}

// DeleteDocument 删除当前用户拥有的知识文档及其分块/图谱。
func (h *RAGServiceImpl) DeleteDocument(ctx context.Context, req *rag.DeleteDocumentReq) (*rag.DeleteDocumentResp, error) {
	if err := h.svc.DeleteDocument(ctx, req.GetViewerId(), req.GetDocumentId()); err != nil {
		return &rag.DeleteDocumentResp{Success: false, Msg: err.Error()}, nil
	}
	return &rag.DeleteDocumentResp{Success: true, Msg: "删除成功"}, nil
}

// DeleteDocumentGraph 删除某篇文档贡献的 GraphRAG 关系，保留文档和检索 chunk。
func (h *RAGServiceImpl) DeleteDocumentGraph(ctx context.Context, req *rag.DeleteGraphReq) (*rag.DeleteGraphResp, error) {
	if err := h.svc.DeleteDocumentGraph(ctx, req.GetViewerId(), req.GetDocumentId()); err != nil {
		return &rag.DeleteGraphResp{Success: false, Msg: err.Error()}, nil
	}
	return &rag.DeleteGraphResp{Success: true, Msg: "图谱已删除"}, nil
}
