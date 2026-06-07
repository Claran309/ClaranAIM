package service

import (
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"context"
	"errors"
)

type ragServiceSource struct {
	client ragservice.Client
}

func NewRAGSource(client ragservice.Client) GraphSource {
	return &ragServiceSource{client: client}
}

func (s *ragServiceSource) GetGraph(ctx context.Context, viewerID int64, input GraphInput) (*rag.GraphResp, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("rag-service客户端未配置")
	}
	return s.client.GetGraph(ctx, &rag.GraphReq{
		ViewerId:   viewerID,
		Query:      input.Query,
		Limit:      int64(input.Limit),
		DocumentId: input.DocumentID,
		Hops:       int64(input.Hops),
	})
}
