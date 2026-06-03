package ragclient

import (
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"context"
	"errors"
)

// RPCClient 使用 Kitex 调用 rag-service。
// 调用方只依赖 pkg/ragclient，避免跨微服务 import internal/rag-service。
type RPCClient struct {
	client ragservice.Client
}

// NewRPCClient 包装已经由 api-gateway 启动逻辑创建好的 Kitex 客户端。
func NewRPCClient(client ragservice.Client) *RPCClient {
	return &RPCClient{client: client}
}

// IngestDocument 写入一份知识文档。
func (c *RPCClient) IngestDocument(ctx context.Context, ownerID int64, input IngestInput) (*rag.IngestDocumentResp, error) {
	resp, err := c.client.IngestDocument(ctx, &rag.IngestDocumentReq{
		OwnerId:        ownerID,
		Title:          input.Title,
		Content:        input.Content,
		Source:         input.Source,
		SourceType:     input.SourceType,
		Visibility:     input.Visibility,
		GroupId:        input.GroupID,
		ConversationId: input.ConversationID,
	})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// Search 执行 RAG 检索并返回答案、来源和图谱证据。
func (c *RPCClient) Search(ctx context.Context, viewerID int64, input SearchInput) (*rag.SearchResp, error) {
	resp, err := c.client.Search(ctx, &rag.SearchReq{
		ViewerId:       viewerID,
		Query:          input.Query,
		Mode:           input.Mode,
		Limit:          int64(input.Limit),
		GroupId:        input.GroupID,
		ConversationId: input.ConversationID,
		DocumentId:     input.DocumentID,
	})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// GetGraph 读取当前用户可见的 GraphRAG 子图。
func (c *RPCClient) GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error) {
	resp, err := c.client.GetGraph(ctx, &rag.GraphReq{ViewerId: viewerID, Query: input.Query, Limit: int64(input.Limit), DocumentId: input.DocumentID, Hops: int64(input.Hops)})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// ListDocuments 返回当前用户可见的知识文档。
func (c *RPCClient) ListDocuments(ctx context.Context, viewerID int64, limit, offset int) (*rag.ListDocumentsResp, error) {
	resp, err := c.client.ListDocuments(ctx, &rag.ListDocumentsReq{ViewerId: viewerID, Limit: int64(limit), Offset: int64(offset)})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// DeleteDocument 删除当前用户有权管理的知识文档、分块和该文档图谱。
func (c *RPCClient) DeleteDocument(ctx context.Context, viewerID, documentID int64) (*rag.DeleteDocumentResp, error) {
	resp, err := c.client.DeleteDocument(ctx, &rag.DeleteDocumentReq{ViewerId: viewerID, DocumentId: documentID})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

// DeleteDocumentGraph 删除当前用户有权管理的某篇文档 GraphRAG 关系，并清理孤立实体。
func (c *RPCClient) DeleteDocumentGraph(ctx context.Context, viewerID, documentID int64) (*rag.DeleteGraphResp, error) {
	resp, err := c.client.DeleteDocumentGraph(ctx, &rag.DeleteGraphReq{ViewerId: viewerID, DocumentId: documentID})
	if err != nil {
		return nil, err
	}
	return resp, rpcStatus(resp.GetSuccess(), resp.GetMsg())
}

func rpcStatus(success bool, msg string) error {
	if success {
		return nil
	}
	if msg == "" {
		msg = "rag-service RPC调用失败"
	}
	return errors.New(msg)
}
