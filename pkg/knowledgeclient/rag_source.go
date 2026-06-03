package knowledgeclient

import (
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/pkg/ragclient"
	"context"
)

type ragServiceSource struct {
	svc ragclient.Service
}

// NewRAGSource 把 rag-service 客户端适配为知识图谱底层数据源。
func NewRAGSource(svc ragclient.Service) GraphSource {
	return &ragServiceSource{svc: svc}
}

func (s *ragServiceSource) GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error) {
	return s.svc.GetGraph(ctx, viewerID, ragclient.GraphInput{Query: input.Query, Limit: input.Limit, DocumentID: input.DocumentID, Hops: input.Hops})
}
