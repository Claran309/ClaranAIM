// Package ragclient 定义其他服务调用 rag-service 时使用的稳定客户端契约。
package ragclient

import (
	"ClaranAIM/kitex_gen/rag"
	"context"
)

const (
	// VisibilityPrivate 表示文档只对上传者本人可见。
	VisibilityPrivate = "private"
	// VisibilityPublic 表示文档可作为公共知识被所有登录用户检索。
	VisibilityPublic = "public"
)

// IngestInput 是知识文档入库的跨服务入参。
type IngestInput struct {
	Title          string
	Content        string
	Source         string
	SourceType     string
	Visibility     string
	GroupID        int64
	ConversationID int64
}

// SearchInput 是 RAG 检索的跨服务入参。
type SearchInput struct {
	Query          string
	Mode           string
	Limit          int
	GroupID        int64
	ConversationID int64
}

// GraphInput 是知识图谱读取的跨服务入参。
type GraphInput struct {
	Query string
	Limit int
}

// Service 是 api-gateway、Agent 服务调用 rag-service 的最小接口。
type Service interface {
	IngestDocument(ctx context.Context, ownerID int64, input IngestInput) (*rag.IngestDocumentResp, error)
	Search(ctx context.Context, viewerID int64, input SearchInput) (*rag.SearchResp, error)
	GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error)
	ListDocuments(ctx context.Context, viewerID int64, limit, offset int) (*rag.ListDocumentsResp, error)
}
