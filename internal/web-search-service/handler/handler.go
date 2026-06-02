// Package handler 实现 web-search-service 的 Kitex RPC 入口。
package handler

import (
	websearchsvc "ClaranAIM/internal/web-search-service/service"
	"ClaranAIM/kitex_gen/web_search"
	"context"
)

// WebSearchServiceImpl 将 Kitex 请求转给 web-search-service 业务层。
type WebSearchServiceImpl struct {
	svc websearchsvc.WebSearchService
}

// NewWebSearchServiceImpl 创建 web-search-service 的 Kitex handler。
func NewWebSearchServiceImpl(svc websearchsvc.WebSearchService) web_search.WebSearchService {
	return &WebSearchServiceImpl{svc: svc}
}

// Search 返回搜索候选，不抓取正文。
func (h *WebSearchServiceImpl) Search(ctx context.Context, req *web_search.SearchReq) (*web_search.SearchResp, error) {
	result, err := h.svc.Search(ctx, websearchsvc.SearchInput{Query: req.GetQuery(), Limit: int(req.GetLimit())})
	if err != nil {
		return &web_search.SearchResp{Success: false, Msg: err.Error()}, nil
	}
	return &web_search.SearchResp{Success: result.Success, Query: result.Query, Results: toRPCSources(result.Results), Msg: result.Msg}, nil
}

// Augment 返回一次性搜索增强上下文和来源。
func (h *WebSearchServiceImpl) Augment(ctx context.Context, req *web_search.AugmentReq) (*web_search.AugmentResp, error) {
	result, err := h.svc.Augment(ctx, websearchsvc.AugmentInput{
		Query:       req.GetQuery(),
		Limit:       int(req.GetLimit()),
		MaxFetch:    int(req.GetMaxFetch()),
		MaxPassages: int(req.GetMaxPassages()),
	})
	if err != nil {
		return &web_search.AugmentResp{Success: false, Msg: err.Error()}, nil
	}
	return &web_search.AugmentResp{
		Success:       result.Success,
		Query:         result.Query,
		AnswerContext: result.AnswerContext,
		Sources:       toRPCSources(result.Sources),
		SearchTime:    result.SearchTime,
		Msg:           result.Msg,
	}, nil
}

func toRPCSources(sources []websearchsvc.WebSource) []*web_search.WebSearchSource {
	out := make([]*web_search.WebSearchSource, 0, len(sources))
	for _, source := range sources {
		out = append(out, &web_search.WebSearchSource{
			Title:       source.Title,
			Url:         source.URL,
			Snippet:     source.Snippet,
			Source:      source.Source,
			Trusted:     source.Trusted,
			Score:       source.Score,
			FetchStatus: source.FetchStatus,
			Passages:    source.Passages,
		})
	}
	return out
}
